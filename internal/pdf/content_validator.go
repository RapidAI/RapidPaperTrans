// Package pdf provides content validation for translated PDFs.
// This module compares the structure of original and translated PDFs
// to detect missing content like sections, appendices, etc.
package pdf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"latex-translator/internal/logger"
)

// ContentValidationResult holds the result of content validation
type ContentValidationResult struct {
	IsComplete         bool          `json:"is_complete"`
	OriginalSections   []SectionInfo `json:"original_sections"`
	TranslatedSections []SectionInfo `json:"translated_sections"`
	MissingSections    []SectionInfo `json:"missing_sections"`
	Warnings           []string      `json:"warnings"`
	PageCountResult    *PageCountResult `json:"page_count_result,omitempty"`
	Score              float64       `json:"score"` // 0-100, completeness score
}

// SectionInfo represents a section found in the PDF
type SectionInfo struct {
	Type       string `json:"type"`        // section, subsection, appendix, abstract, etc.
	Number     string `json:"number"`      // e.g., "1", "1.1", "A", "B.1"
	Title      string `json:"title"`       // Section title
	Page       int    `json:"page"`        // Page number where found
	Level      int    `json:"level"`       // Nesting level (1=section, 2=subsection, etc.)
	IsAppendix bool   `json:"is_appendix"` // Whether this is in appendix
}

// ContentValidator validates content completeness between original and translated PDFs
type ContentValidator struct {
	translator *BabelDocTranslator
}

// NewContentValidator creates a new content validator
func NewContentValidator(workDir string) *ContentValidator {
	return &ContentValidator{
		translator: NewBabelDocTranslator(BabelDocConfig{WorkDir: workDir}),
	}
}

// ValidateContent compares original and translated PDFs for content completeness
func (v *ContentValidator) ValidateContent(originalPath, translatedPath string) (*ContentValidationResult, error) {
	logger.Info("validating content completeness",
		logger.String("original", originalPath),
		logger.String("translated", translatedPath))

	result := &ContentValidationResult{
		IsComplete: true,
		Score:      100.0,
	}

	// Step 1: Check page count difference
	pageResult, err := v.translator.CheckPageCountDifference(originalPath, translatedPath)
	if err != nil {
		logger.Warn("failed to check page count", logger.Err(err))
	} else {
		result.PageCountResult = pageResult
		if pageResult.IsSuspicious {
			result.Warnings = append(result.Warnings, FormatPageCountError(pageResult))
			result.Score -= 20.0 // Deduct 20 points for suspicious page count
		}
	}

	// Step 2: Extract sections from both PDFs
	originalBlocks, err := v.translator.ExtractBlocks(originalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract original PDF blocks: %w", err)
	}

	translatedBlocks, err := v.translator.ExtractBlocks(translatedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract translated PDF blocks: %w", err)
	}

	// Step 3: Extract section structure from blocks
	result.OriginalSections = v.extractSections(originalBlocks)
	result.TranslatedSections = v.extractSections(translatedBlocks)

	// Step 4: Compare sections
	result.MissingSections = v.findMissingSections(result.OriginalSections, result.TranslatedSections)

	// Step 5: Calculate completeness score
	if len(result.OriginalSections) > 0 {
		matchedCount := len(result.OriginalSections) - len(result.MissingSections)
		sectionScore := float64(matchedCount) / float64(len(result.OriginalSections)) * 80.0
		result.Score = minFloat64(result.Score, sectionScore+20.0)
	} else {
		// If no sections detected, rely more heavily on page count
		// This can happen with PDFs that have poor text extraction
		if result.PageCountResult != nil && result.PageCountResult.IsSuspicious {
			// Page count is suspicious and we couldn't detect sections
			// Lower the score based on page difference
			pageDiffPenalty := result.PageCountResult.DiffPercent * 100 // 0-100
			result.Score = 100.0 - pageDiffPenalty
			if result.Score < 0 {
				result.Score = 0
			}
		}
	}

	// Step 6: Generate warnings for missing sections
	for _, section := range result.MissingSections {
		warning := v.formatMissingSectionWarning(section)
		result.Warnings = append(result.Warnings, warning)
	}

	// Step 7: Check for missing appendices specifically
	originalAppendices := v.filterAppendices(result.OriginalSections)
	translatedAppendices := v.filterAppendices(result.TranslatedSections)

	if len(originalAppendices) > 0 && len(translatedAppendices) == 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("原始PDF有%d个附录，但翻译后PDF中未检测到附录", len(originalAppendices)))
		result.Score -= 15.0
	} else if len(originalAppendices) > len(translatedAppendices) {
		missing := len(originalAppendices) - len(translatedAppendices)
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("翻译后PDF缺少%d个附录", missing))
		result.Score -= float64(missing) * 5.0
	}

	// Determine if content is complete
	result.IsComplete = len(result.MissingSections) == 0 && result.Score >= 70.0

	logger.Info("content validation completed",
		logger.Bool("isComplete", result.IsComplete),
		logger.Float64("score", result.Score),
		logger.Int("missingSections", len(result.MissingSections)))

	return result, nil
}


// extractSections extracts section information from PDF blocks
func (v *ContentValidator) extractSections(blocks []BabelDocBlock) []SectionInfo {
	var sections []SectionInfo

	// Patterns for detecting sections - order matters! More specific patterns first
	sectionPatterns := []struct {
		pattern *regexp.Regexp
		sType   string
		level   int
	}{
		// Sub-subsections first (most specific): "1.1.1 Details"
		{regexp.MustCompile(`^(\d+\.\d+\.\d+)\s*\.?\s+(.+)$`), "subsubsection", 3},

		// Subsections: "1.1 Background", "1.1. Background"
		{regexp.MustCompile(`^(\d+\.\d+)\s*\.?\s+(.+)$`), "subsection", 2},

		// Numbered sections: "1 Introduction", "1. Introduction"
		{regexp.MustCompile(`^(\d+)\s*\.?\s+(.+)$`), "section", 1},
		{regexp.MustCompile(`^Section\s+(\d+)[:\s]*(.*)$`), "section", 1},

		// Appendix subsections: "A.1 Details", "B.2 More"
		{regexp.MustCompile(`^([A-Z]\.\d+)\s*\.?\s+(.+)$`), "appendix_subsection", 2},

		// Appendix patterns: "Appendix A", "A. Appendix"
		{regexp.MustCompile(`^Appendix\s+([A-Z])[:\s]*(.*)$`), "appendix", 1},
		{regexp.MustCompile(`^([A-Z])\s*\.\s*Appendix[:\s]*(.*)$`), "appendix", 1},
		{regexp.MustCompile(`^附录\s*([A-Z]?)[:\s]*(.*)$`), "appendix", 1},

		// Special sections (exact match)
		{regexp.MustCompile(`^Abstract$`), "abstract", 0},
		{regexp.MustCompile(`^摘要$`), "abstract", 0},
		{regexp.MustCompile(`^Introduction$`), "introduction", 1},
		{regexp.MustCompile(`^引言$`), "introduction", 1},
		{regexp.MustCompile(`^Conclusions?$`), "conclusion", 1},
		{regexp.MustCompile(`^结论$`), "conclusion", 1},
		{regexp.MustCompile(`^References?$`), "references", 1},
		{regexp.MustCompile(`^参考文献$`), "references", 1},
		{regexp.MustCompile(`^Acknowledgments?$`), "acknowledgments", 1},
		{regexp.MustCompile(`^致谢$`), "acknowledgments", 1},
	}

	// Track if we're in appendix section
	inAppendix := false

	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if len(text) < 2 || len(text) > 150 {
			continue
		}

		// Check if this could be a section heading based on:
		// 1. BlockType is "heading"
		// 2. Font size is larger than normal (>= 11)
		// 3. Text matches section pattern (even if not marked as heading)
		isLikelyHeading := block.BlockType == "heading" || block.FontSize >= 11

		// Check if entering appendix section
		textLower := strings.ToLower(text)
		if strings.Contains(textLower, "appendix") || strings.Contains(text, "附录") {
			inAppendix = true
		}

		// Try to match section patterns
		for _, sp := range sectionPatterns {
			matches := sp.pattern.FindStringSubmatch(text)
			if matches != nil {
				// For numbered sections, we can be more lenient about heading detection
				// because the pattern itself is strong evidence
				isNumberedSection := sp.sType == "section" || sp.sType == "subsection" || 
					sp.sType == "subsubsection" || strings.Contains(sp.sType, "appendix")
				
				if !isLikelyHeading && !isNumberedSection {
					continue
				}

				section := SectionInfo{
					Type:       sp.sType,
					Level:      sp.level,
					Page:       block.Page,
					IsAppendix: inAppendix || strings.Contains(sp.sType, "appendix"),
				}

				// Extract number and title based on pattern
				if len(matches) >= 2 {
					section.Number = matches[1]
				}
				if len(matches) >= 3 {
					section.Title = strings.TrimSpace(matches[2])
				}
				if section.Title == "" {
					section.Title = text
				}

				// Avoid duplicates
				if !v.sectionExists(sections, section) {
					sections = append(sections, section)
				}
				break
			}
		}
	}

	// Sort sections by page and then by number
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Page != sections[j].Page {
			return sections[i].Page < sections[j].Page
		}
		return sections[i].Number < sections[j].Number
	})

	return sections
}

// sectionExists checks if a similar section already exists
func (v *ContentValidator) sectionExists(sections []SectionInfo, newSection SectionInfo) bool {
	for _, s := range sections {
		if s.Type == newSection.Type && s.Number == newSection.Number {
			return true
		}
		// Also check by title similarity for unnumbered sections
		if s.Type == newSection.Type && s.Number == "" && newSection.Number == "" {
			if strings.EqualFold(s.Title, newSection.Title) {
				return true
			}
		}
	}
	return false
}

// findMissingSections finds sections in original that are missing in translated
func (v *ContentValidator) findMissingSections(original, translated []SectionInfo) []SectionInfo {
	var missing []SectionInfo

	// Create a map of translated sections for quick lookup
	translatedMap := make(map[string]bool)
	for _, s := range translated {
		key := v.sectionKey(s)
		translatedMap[key] = true
	}

	// Find missing sections
	for _, s := range original {
		key := v.sectionKey(s)
		if !translatedMap[key] {
			// Also try to find by similar title (for translated titles)
			found := false
			for _, ts := range translated {
				if v.sectionsMatch(s, ts) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, s)
			}
		}
	}

	return missing
}

// sectionKey generates a unique key for a section
func (v *ContentValidator) sectionKey(s SectionInfo) string {
	if s.Number != "" {
		return fmt.Sprintf("%s:%s", s.Type, s.Number)
	}
	return fmt.Sprintf("%s:%s", s.Type, strings.ToLower(s.Title))
}

// sectionsMatch checks if two sections match (considering translation)
func (v *ContentValidator) sectionsMatch(original, translated SectionInfo) bool {
	// Same type and number
	if original.Type == translated.Type && original.Number == translated.Number && original.Number != "" {
		return true
	}

	// Same type and similar page location (within 2 pages)
	if original.Type == translated.Type {
		pageDiff := abs(float64(original.Page - translated.Page))
		if pageDiff <= 2 {
			if v.titlesSimilar(original.Title, translated.Title) {
				return true
			}
		}
	}

	return false
}


// titlesSimilar checks if two titles are similar (one might be translated)
func (v *ContentValidator) titlesSimilar(title1, title2 string) bool {
	// Exact match
	if strings.EqualFold(title1, title2) {
		return true
	}

	// Common section title mappings (English -> Chinese)
	titleMappings := map[string][]string{
		"introduction":    {"引言", "介绍", "简介"},
		"background":      {"背景", "研究背景"},
		"related work":    {"相关工作", "相关研究"},
		"method":          {"方法", "方法论"},
		"methodology":     {"方法论", "研究方法"},
		"experiment":      {"实验", "实验设置"},
		"experiments":     {"实验", "实验结果"},
		"results":         {"结果", "实验结果"},
		"discussion":      {"讨论", "分析讨论"},
		"conclusion":      {"结论", "总结"},
		"conclusions":     {"结论", "总结"},
		"abstract":        {"摘要"},
		"references":      {"参考文献", "引用"},
		"acknowledgments": {"致谢", "鸣谢"},
		"appendix":        {"附录"},
		"evaluation":      {"评估", "评价"},
		"analysis":        {"分析"},
		"implementation":  {"实现", "实施"},
		"limitations":     {"局限性", "限制"},
		"future work":     {"未来工作", "展望"},
	}

	t1Lower := strings.ToLower(title1)
	t2Lower := strings.ToLower(title2)

	// Check if title1 maps to title2
	for eng, chnList := range titleMappings {
		if strings.Contains(t1Lower, eng) {
			for _, chn := range chnList {
				if strings.Contains(title2, chn) {
					return true
				}
			}
		}
		// Also check reverse (Chinese -> English)
		for _, chn := range chnList {
			if strings.Contains(title1, chn) && strings.Contains(t2Lower, eng) {
				return true
			}
		}
	}

	return false
}

// filterAppendices returns only appendix sections
func (v *ContentValidator) filterAppendices(sections []SectionInfo) []SectionInfo {
	var appendices []SectionInfo
	for _, s := range sections {
		if s.IsAppendix || strings.Contains(s.Type, "appendix") {
			appendices = append(appendices, s)
		}
	}
	return appendices
}

// formatMissingSectionWarning formats a warning message for a missing section
func (v *ContentValidator) formatMissingSectionWarning(section SectionInfo) string {
	var typeStr string
	switch section.Type {
	case "section":
		typeStr = "章节"
	case "subsection":
		typeStr = "小节"
	case "subsubsection":
		typeStr = "子小节"
	case "appendix":
		typeStr = "附录"
	case "appendix_subsection":
		typeStr = "附录小节"
	case "abstract":
		typeStr = "摘要"
	case "references":
		typeStr = "参考文献"
	default:
		typeStr = section.Type
	}

	if section.Number != "" {
		return fmt.Sprintf("缺少%s %s: %s (原始第%d页)", typeStr, section.Number, section.Title, section.Page)
	}
	return fmt.Sprintf("缺少%s: %s (原始第%d页)", typeStr, section.Title, section.Page)
}

// FormatValidationResult formats the validation result as a human-readable string
func FormatValidationResult(result *ContentValidationResult) string {
	var sb strings.Builder

	sb.WriteString("=== 内容完整性检测结果 ===\n\n")

	// Overall status
	if result.IsComplete {
		sb.WriteString("✓ 内容完整性检测通过\n")
	} else {
		sb.WriteString("✗ 内容完整性检测未通过\n")
	}
	sb.WriteString(fmt.Sprintf("完整性评分: %.1f/100\n\n", result.Score))

	// Page count info
	if result.PageCountResult != nil {
		sb.WriteString(fmt.Sprintf("页数对比: 原始 %d 页, 翻译后 %d 页",
			result.PageCountResult.OriginalPages,
			result.PageCountResult.TranslatedPages))
		if result.PageCountResult.IsSuspicious {
			sb.WriteString(fmt.Sprintf(" (差异 %.1f%%, 超过阈值!)", result.PageCountResult.DiffPercent*100))
		}
		sb.WriteString("\n\n")
	}

	// Section comparison
	sb.WriteString(fmt.Sprintf("检测到的章节: 原始 %d 个, 翻译后 %d 个\n",
		len(result.OriginalSections), len(result.TranslatedSections)))

	// Missing sections
	if len(result.MissingSections) > 0 {
		sb.WriteString(fmt.Sprintf("\n缺失的章节 (%d 个):\n", len(result.MissingSections)))
		for _, s := range result.MissingSections {
			if s.Number != "" {
				sb.WriteString(fmt.Sprintf("  - %s %s: %s (第%d页)\n", s.Type, s.Number, s.Title, s.Page))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s: %s (第%d页)\n", s.Type, s.Title, s.Page))
			}
		}
	}

	// Warnings
	if len(result.Warnings) > 0 {
		sb.WriteString("\n警告:\n")
		for _, w := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  ⚠ %s\n", w))
		}
	}

	return sb.String()
}

// minFloat64 returns the minimum of two float64 values
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
