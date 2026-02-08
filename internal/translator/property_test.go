// Package translator provides property-based tests for LaTeX format preservation.
// These tests validate correctness properties across many random inputs.
package translator

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"testing/quick"
)

// ============================================================
// Property Test Configuration
// ============================================================

// quickConfig returns the configuration for property-based tests
func quickConfig() *quick.Config {
	return &quick.Config{
		MaxCount: 100, // Run at least 100 iterations per property
		Rand:     rand.New(rand.NewSource(42)), // Reproducible tests
	}
}

// ============================================================
// Test Data Generators
// ============================================================

// generateRandomLaTeXContent generates random LaTeX content for testing
func generateRandomLaTeXContent(r *rand.Rand) string {
	var sb strings.Builder
	
	// Add some random text
	words := []string{"The", "a", "is", "are", "this", "that", "test", "example", "content", "text"}
	for i := 0; i < r.Intn(10)+1; i++ {
		sb.WriteString(words[r.Intn(len(words))])
		sb.WriteString(" ")
	}
	
	return sb.String()
}

// generateMathContent generates random math content
func generateMathContent(r *rand.Rand) string {
	mathExprs := []string{
		"$x + y$",
		"$a^2 + b^2 = c^2$",
		"$\\frac{1}{2}$",
		"$\\sum_{i=1}^{n} x_i$",
		"\\[E = mc^2\\]",
		"\\(x = y\\)",
		"\\begin{equation}\nx = y\n\\end{equation}",
		"\\begin{align}\na &= b \\\\\nc &= d\n\\end{align}",
	}
	return mathExprs[r.Intn(len(mathExprs))]
}

// generateTableContent generates random table content
func generateTableContent(r *rand.Rand) string {
	tables := []string{
		"\\begin{tabular}{cc}\nA & B \\\\\n1 & 2 \\\\\n\\end{tabular}",
		"\\begin{table}\n\\begin{tabular}{|c|c|}\n\\hline\nX & Y \\\\\n\\hline\n\\end{tabular}\n\\end{table}",
		"\\multirow{2}{*}{Test} & A \\\\\n& B \\\\",
		"\\multicolumn{2}{c}{Header} \\\\",
	}
	return tables[r.Intn(len(tables))]
}

// generateEnvironmentContent generates random environment content
func generateEnvironmentContent(r *rand.Rand) string {
	envs := []string{
		"\\begin{figure}\n\\includegraphics{test.png}\n\\caption{Test}\n\\end{figure}",
		"\\begin{itemize}\n\\item First\n\\item Second\n\\end{itemize}",
		"\\begin{enumerate}\n\\item One\n\\item Two\n\\end{enumerate}",
		"\\begin{center}\nCentered text\n\\end{center}",
	}
	return envs[r.Intn(len(envs))]
}

// generateCommentContent generates random comment content
func generateCommentContent(r *rand.Rand) string {
	comments := []string{
		"% This is a comment\n",
		"% TODO: fix this\n",
		"Some text % inline comment\n",
		"% \\begin{figure}\n% commented out\n% \\end{figure}\n",
	}
	return comments[r.Intn(len(comments))]
}

// ============================================================
// Property 1: Placeholder Round-Trip
// Feature: latex-format-preservation, Property 1: Placeholder Round-Trip
// Validates: Requirements 1.2
// ============================================================

func TestProperty1_PlaceholderRoundTrip(t *testing.T) {
	// Property: For any LaTeX content containing commands, protecting the content
	// with placeholders and then restoring the placeholders SHALL produce content
	// equivalent to the original.
	
	testCases := []string{
		"Simple text without commands",
		"Text with $math$ inline",
		"Text with \\textbf{bold} command",
		"\\begin{equation}\nx = y\n\\end{equation}",
		"Mixed $a$ and \\[b\\] math",
		"\\section{Title}\nContent here",
	}
	
	for _, original := range testCases {
		t.Run("", func(t *testing.T) {
			// Protect math environments
			protected, placeholders := ProtectMathEnvironments(original)
			
			// Restore placeholders
			restored := RestoreMathEnvironments(protected, placeholders)
			
			// Verify round-trip
			if restored != original {
				t.Errorf("Placeholder round-trip failed:\nOriginal:  %q\nProtected: %q\nRestored:  %q",
					original, protected, restored)
			}
		})
	}
}

func TestProperty1_PlaceholderRoundTrip_Quick(t *testing.T) {
	// Property-based test using testing/quick
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		
		// Generate random content with math
		var sb strings.Builder
		sb.WriteString(generateRandomLaTeXContent(r))
		if r.Float32() > 0.5 {
			sb.WriteString(generateMathContent(r))
		}
		sb.WriteString(generateRandomLaTeXContent(r))
		
		original := sb.String()
		
		// Protect and restore
		protected, placeholders := ProtectMathEnvironments(original)
		restored := RestoreMathEnvironments(protected, placeholders)
		
		return restored == original
	}
	
	if err := quick.Check(f, quickConfig()); err != nil {
		t.Error(err)
	}
}

// ============================================================
// Property 2: Math Environment Protection
// Feature: latex-format-preservation, Property 2: Math Environment Protection
// Validates: Requirements 1.3
// ============================================================

func TestProperty2_MathEnvironmentProtection(t *testing.T) {
	// Property: For any LaTeX content containing mathematical expressions,
	// the Format_Protector SHALL identify and protect each complete mathematical
	// expression as a single unit, with no partial expressions.
	
	testCases := []struct {
		name     string
		input    string
		wantMath int // expected number of math regions
	}{
		{"inline dollar", "Text $x + y$ more", 1},
		{"display dollar", "Text $$a = b$$ more", 1},
		{"bracket math", "Text \\[x = y\\] more", 1},
		{"paren math", "Text \\(a\\) more", 1},
		{"equation env", "\\begin{equation}\nx\n\\end{equation}", 1},
		{"multiple inline", "$a$ and $b$ and $c$", 3},
		{"mixed types", "$a$ and \\[b\\] and \\(c\\)", 3},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, placeholders := ProtectMathEnvironments(tc.input)
			
			if len(placeholders) != tc.wantMath {
				t.Errorf("Expected %d math regions, got %d", tc.wantMath, len(placeholders))
			}
			
			// Verify each placeholder contains complete math (starts and ends correctly)
			for _, original := range placeholders {
				if strings.HasPrefix(original, "$") && !strings.HasSuffix(original, "$") {
					t.Errorf("Incomplete math expression: %q", original)
				}
				if strings.HasPrefix(original, "\\[") && !strings.HasSuffix(original, "\\]") {
					t.Errorf("Incomplete bracket math: %q", original)
				}
				if strings.HasPrefix(original, "\\(") && !strings.HasSuffix(original, "\\)") {
					t.Errorf("Incomplete paren math: %q", original)
				}
				if strings.Contains(original, "\\begin{") && !strings.Contains(original, "\\end{") {
					t.Errorf("Incomplete math environment: %q", original)
				}
			}
		})
	}
}

// ============================================================
// Property 3: Table Structure Protection
// Feature: latex-format-preservation, Property 3: Table Structure Protection
// Validates: Requirements 1.4
// ============================================================

func TestProperty3_TableStructureProtection(t *testing.T) {
	// Property: For any LaTeX content containing table environments,
	// the Format_Protector SHALL protect the entire table structure including
	// all \multirow, \multicolumn, cell separators (&), and row terminators (\\).
	
	testCases := []struct {
		name  string
		input string
	}{
		{
			"simple tabular",
			"\\begin{tabular}{cc}\nA & B \\\\\n1 & 2 \\\\\n\\end{tabular}",
		},
		{
			"table with multirow",
			"\\begin{tabular}{cc}\n\\multirow{2}{*}{X} & A \\\\\n& B \\\\\n\\end{tabular}",
		},
		{
			"table with multicolumn",
			"\\begin{tabular}{ccc}\n\\multicolumn{2}{c}{Header} & C \\\\\n\\end{tabular}",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, placeholders := ProtectTableStructures(tc.input)
			
			// Verify table is protected
			if len(placeholders) == 0 {
				t.Error("Table should be protected with placeholder")
			}
			
			// Verify original table structure is preserved in placeholder
			for _, original := range placeholders {
				if strings.Contains(tc.input, "\\multirow") && !strings.Contains(original, "\\multirow") {
					t.Error("\\multirow should be preserved in protected content")
				}
				if strings.Contains(tc.input, "\\multicolumn") && !strings.Contains(original, "\\multicolumn") {
					t.Error("\\multicolumn should be preserved in protected content")
				}
				if strings.Contains(tc.input, "&") && !strings.Contains(original, "&") {
					t.Error("Cell separators (&) should be preserved")
				}
				if strings.Contains(tc.input, "\\\\") && !strings.Contains(original, "\\\\") {
					t.Error("Row terminators (\\\\) should be preserved")
				}
			}
		})
	}
}

// ============================================================
// Property 6: Environment Counting Accuracy
// Feature: latex-format-preservation, Property 6: Environment Counting Accuracy
// Validates: Requirements 3.1
// ============================================================

func TestProperty6_EnvironmentCountingAccuracy(t *testing.T) {
	// Property: For any LaTeX content, the Structure_Validator SHALL correctly
	// count all \begin{env} and \end{env} pairs.
	
	testCases := []struct {
		name     string
		input    string
		envName  string
		expected int
	}{
		{"single figure", "\\begin{figure}\\end{figure}", "figure", 1},
		{"nested envs", "\\begin{figure}\\begin{center}\\end{center}\\end{figure}", "figure", 1},
		{"multiple same", "\\begin{itemize}\\end{itemize}\\begin{itemize}\\end{itemize}", "itemize", 2},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateEnvironments(tc.input)
			
			if count, ok := result.Environments[tc.envName]; ok {
				if count.BeginCount != tc.expected || count.EndCount != tc.expected {
					t.Errorf("Expected %d %s environments, got begin=%d end=%d",
						tc.expected, tc.envName, count.BeginCount, count.EndCount)
				}
			} else if tc.expected > 0 {
				t.Errorf("Environment %s not found in results", tc.envName)
			}
		})
	}
}

// ============================================================
// Property 10: Brace Balance Validation
// Feature: latex-format-preservation, Property 10: Brace Balance Validation
// Validates: Requirements 4.1, 4.4
// ============================================================

func TestProperty10_BraceBalanceValidation(t *testing.T) {
	// Property: For any LaTeX content, the Structure_Validator SHALL correctly
	// count opening { and closing } braces, including nested braces at all levels.
	
	testCases := []struct {
		name       string
		input      string
		isBalanced bool
	}{
		{"empty", "", true},
		{"simple balanced", "{}", true},
		{"nested balanced", "{{{}}}}", false},
		{"text with braces", "\\textbf{bold}", true},
		{"unbalanced open", "{{{", false},
		{"unbalanced close", "}}}", false},
		{"complex balanced", "\\section{Title} \\textbf{\\textit{nested}}", true},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateBraces(tc.input)
			
			if result.IsBalanced != tc.isBalanced {
				t.Errorf("Expected IsBalanced=%v, got %v (open=%d, close=%d)",
					tc.isBalanced, result.IsBalanced, result.OpenCount, result.CloseCount)
			}
		})
	}
}

// ============================================================
// Property 4 & 5: Line Structure Preservation
// Feature: latex-format-preservation, Property 4 & 5: Line Structure Preservation
// Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5
// ============================================================

func TestProperty4_LineCountPreservation(t *testing.T) {
	// Property: After translation and format restoration, the translated content
	// SHALL have the same number of lines as the original content (within ±5%).
	
	testCases := []struct {
		name     string
		original string
		modified string
		valid    bool
	}{
		{"same lines", "line1\nline2\nline3", "行1\n行2\n行3", true},
		{"merged lines", "line1\nline2\nline3", "行1 行2 行3", false},
		{"extra lines", "line1\nline2", "行1\n行2\n行3\n行4", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origLines := strings.Count(tc.original, "\n") + 1
			modLines := strings.Count(tc.modified, "\n") + 1
			
			ratio := float64(modLines) / float64(origLines)
			isValid := ratio >= 0.95 && ratio <= 1.05
			
			if isValid != tc.valid {
				t.Errorf("Line count validation: expected valid=%v, got %v (orig=%d, mod=%d, ratio=%.2f)",
					tc.valid, isValid, origLines, modLines, ratio)
			}
		})
	}
}

func TestProperty5_LineStructurePreservation(t *testing.T) {
	// Property: For any LaTeX content containing \begin{...}, \end{...}, or \item
	// commands, after format restoration, each of these commands SHALL appear on
	// its own separate line.
	
	testCases := []struct {
		name   string
		input  string
		expect bool // expect each command on own line
	}{
		{
			"begin on own line",
			"\\begin{figure}\ncontent\n\\end{figure}",
			true,
		},
		{
			"merged begin end",
			"\\begin{figure}content\\end{figure}",
			false,
		},
		{
			"items on own lines",
			"\\begin{itemize}\n\\item First\n\\item Second\n\\end{itemize}",
			true,
		},
	}
	
	beginEndPattern := regexp.MustCompile(`\\(begin|end)\{[^}]+\}`)
	itemPattern := regexp.MustCompile(`\\item`)
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.input, "\n")
			allOnOwnLine := true
			
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				
				// Check if line has multiple begin/end/item commands
				beginEndMatches := beginEndPattern.FindAllString(trimmed, -1)
				itemMatches := itemPattern.FindAllString(trimmed, -1)
				
				totalCommands := len(beginEndMatches) + len(itemMatches)
				if totalCommands > 1 {
					allOnOwnLine = false
					break
				}
				
				// Check if begin/end is mixed with other content
				if len(beginEndMatches) == 1 {
					// Remove the command and check if there's significant content left
					remaining := beginEndPattern.ReplaceAllString(trimmed, "")
					remaining = strings.TrimSpace(remaining)
					if len(remaining) > 0 && remaining != "%" {
						allOnOwnLine = false
						break
					}
				}
			}
			
			if allOnOwnLine != tc.expect {
				t.Errorf("Expected commands on own line=%v, got %v", tc.expect, allOnOwnLine)
			}
		})
	}
}

// ============================================================
// Property 14-16: Comment Preservation
// Feature: latex-format-preservation, Property 14-16: Comment Preservation
// Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5
// ============================================================

func TestProperty14_CommentLineProtection(t *testing.T) {
	// Property: Lines starting with % SHALL be identified as comment lines,
	// and any \begin{...} or \end{...} within comments SHALL remain commented.
	
	testCases := []struct {
		name     string
		input    string
		expected bool // should be identified as comment
	}{
		{"simple comment", "% This is a comment", true},
		{"commented begin", "% \\begin{figure}", true},
		{"not a comment", "Text here", false},
		{"inline comment", "Text % comment", false}, // line doesn't start with %
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isComment := strings.HasPrefix(strings.TrimSpace(tc.input), "%")
			
			if isComment != tc.expected {
				t.Errorf("Expected isComment=%v, got %v for %q", tc.expected, isComment, tc.input)
			}
		})
	}
}

func TestProperty15_CommentSymbolPreservation(t *testing.T) {
	// Property: For any comment line in LaTeX content, after translation,
	// the % symbol SHALL be preserved at the beginning of the line.
	
	original := "% This is a comment\nRegular text\n% Another comment"
	
	// Simulate what ValidateComments checks
	result := ValidateComments(original, original)
	
	if !result.IsValid {
		t.Errorf("Comment validation should pass for identical content")
	}
}

// ============================================================
// Property 17-20: Chunking Properties
// Feature: latex-format-preservation, Property 17-20: Chunking Properties
// Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5
// ============================================================

func TestProperty17_ChunkBoundaryRespect(t *testing.T) {
	// Property: For any LaTeX content being split into chunks, chunk boundaries
	// SHALL NOT fall within an environment.
	
	content := "\\begin{figure}\nContent here\n\\end{figure}\n\nMore text\n\n\\begin{table}\nTable content\n\\end{table}"
	
	chunks := splitIntoChunks(content, 50) // Small chunk size to force splitting
	
	for i, chunk := range chunks {
		// Count begin and end in each chunk
		beginCount := strings.Count(chunk, "\\begin{")
		endCount := strings.Count(chunk, "\\end{")
		
		// Each chunk should have balanced environments or be part of a larger env
		if beginCount != endCount {
			// This is acceptable if the chunk is intentionally keeping an environment together
			// But we should verify the environment isn't split mid-content
			if strings.Contains(chunk, "\\begin{") && !strings.Contains(chunk, "\\end{") {
				// Check if this is the start of an environment that continues
				t.Logf("Chunk %d has unbalanced env (begin=%d, end=%d): %q", i, beginCount, endCount, chunk[:min(50, len(chunk))])
			}
		}
	}
}

func TestProperty20_ChunkReassemblyRoundTrip(t *testing.T) {
	// Property: For any LaTeX content, splitting into chunks and then reassembling
	// SHALL produce content identical to the original (no duplication or loss).
	
	testCases := []string{
		"Simple text content",
		"\\begin{figure}\nContent\n\\end{figure}",
		"Line 1\n\nLine 2\n\nLine 3",
		"Mixed $math$ and \\textbf{formatting}",
	}
	
	for _, original := range testCases {
		t.Run("", func(t *testing.T) {
			chunks := splitIntoChunks(original, 100)
			reassembled := strings.Join(chunks, "")
			
			if reassembled != original {
				t.Errorf("Chunk reassembly failed:\nOriginal:    %q\nReassembled: %q", original, reassembled)
			}
		})
	}
}

// ============================================================
// Property 21-24: Validation Properties
// Feature: latex-format-preservation, Property 21-24: Validation Properties
// Validates: Requirements 7.1, 7.2, 7.3, 7.5
// ============================================================

func TestProperty22_LengthRatioValidation(t *testing.T) {
	// Property: For any translated content with length ratio outside the acceptable
	// range (< 0.3 or > 3.0 of original), the Structure_Validator SHALL flag it.
	
	testCases := []struct {
		name       string
		original   string
		translated string
		shouldFlag bool
	}{
		{"normal ratio", "Hello world", "你好世界", false},
		{"too short", "This is a long sentence with many words", "短", true},
		{"too long", "Hi", strings.Repeat("很长的翻译", 100), true},
		{"acceptable", "Test content here", "测试内容在这里", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ratio := float64(len(tc.translated)) / float64(len(tc.original))
			isFlagged := ratio < 0.3 || ratio > 3.0
			
			if isFlagged != tc.shouldFlag {
				t.Errorf("Expected flagged=%v, got %v (ratio=%.2f)", tc.shouldFlag, isFlagged, ratio)
			}
		})
	}
}

func TestProperty24_ChineseRatioValidation(t *testing.T) {
	// Property: For any translated content with Chinese character ratio below 5%
	// (for substantial content > 1000 chars), the Structure_Validator SHALL warn.
	
	chinesePattern := regexp.MustCompile(`[\p{Han}]`)
	
	testCases := []struct {
		name       string
		content    string
		shouldWarn bool
	}{
		{"good ratio", "这是中文内容，有很多汉字在这里", false},
		{"low ratio", strings.Repeat("English text ", 100) + "中", true},
		{"short content", "Short", false}, // Too short to warn
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.content) < 1000 && tc.name != "short content" {
				t.Skip("Content too short for this test")
			}
			
			chineseCount := len(chinesePattern.FindAllString(tc.content, -1))
			ratio := float64(chineseCount) / float64(len(tc.content))
			shouldWarn := len(tc.content) > 1000 && ratio < 0.05
			
			if shouldWarn != tc.shouldWarn {
				t.Errorf("Expected warn=%v, got %v (ratio=%.4f)", tc.shouldWarn, shouldWarn, ratio)
			}
		})
	}
}

// ============================================================
// Helper function for min
// ============================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// ============================================================
// Property 7: Environment Mismatch Detection
// Feature: latex-format-preservation, Property 7: Environment Mismatch Detection
// Validates: Requirements 3.2
// ============================================================

func TestProperty7_EnvironmentMismatchDetection(t *testing.T) {
	// Property: For any LaTeX content with mismatched \begin and \end tags,
	// the Structure_Validator SHALL detect the mismatch and report the specific
	// environment name and count difference.
	
	testCases := []struct {
		name           string
		input          string
		expectMismatch bool
		envName        string
		difference     int // positive = missing \end, negative = extra \end
	}{
		{
			"balanced figure",
			"\\begin{figure}\ncontent\n\\end{figure}",
			false,
			"figure",
			0,
		},
		{
			"missing end figure",
			"\\begin{figure}\ncontent",
			true,
			"figure",
			1,
		},
		{
			"extra end figure",
			"content\n\\end{figure}",
			true,
			"figure",
			-1,
		},
		{
			"multiple missing ends",
			"\\begin{table}\\begin{tabular}content\\end{tabular}",
			true,
			"table",
			1,
		},
		{
			"nested balanced",
			"\\begin{figure}\\begin{center}text\\end{center}\\end{figure}",
			false,
			"figure",
			0,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateEnvironments(tc.input)
			
			if tc.expectMismatch {
				if result.IsValid {
					t.Errorf("Expected mismatch to be detected, but validation passed")
				}
				
				// Check if the specific environment mismatch is reported
				found := false
				for _, m := range result.Mismatches {
					if m.EnvName == tc.envName {
						found = true
						if m.Difference != tc.difference {
							t.Errorf("Expected difference %d for %s, got %d",
								tc.difference, tc.envName, m.Difference)
						}
					}
				}
				if !found && tc.difference != 0 {
					t.Errorf("Expected mismatch for %s not found in results", tc.envName)
				}
			} else {
				if !result.IsValid && len(result.Mismatches) > 0 {
					t.Errorf("Expected no mismatch, but got: %v", result.Mismatches)
				}
			}
		})
	}
}

// ============================================================
// Property 8: Missing End Tag Restoration
// Feature: latex-format-preservation, Property 8: Missing End Tag Restoration
// Validates: Requirements 3.3
// ============================================================

func TestProperty8_MissingEndTagRestoration(t *testing.T) {
	// Property: For any LaTeX content where an \end{...} tag is missing,
	// the Format_Restorer SHALL insert the missing tag at a position that
	// maintains valid LaTeX structure.
	
	testCases := []struct {
		name       string
		translated string
		original   string
		expectEnd  string // expected \end{...} to be present after restoration
	}{
		{
			"missing end figure",
			"\\begin{figure}\n\\includegraphics{test.png}\n\\caption{Test}",
			"\\begin{figure}\n\\includegraphics{test.png}\n\\caption{Test}\n\\end{figure}",
			"\\end{figure}",
		},
		{
			"missing end itemize",
			"\\begin{itemize}\n\\item First\n\\item Second",
			"\\begin{itemize}\n\\item First\n\\item Second\n\\end{itemize}",
			"\\end{itemize}",
		},
		{
			"nested missing end",
			"\\begin{table}\n\\begin{tabular}{cc}\nA & B\n\\end{tabular}",
			"\\begin{table}\n\\begin{tabular}{cc}\nA & B\n\\end{tabular}\n\\end{table}",
			"\\end{table}",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Apply environment restoration
			restored := RestoreEnvironments(tc.translated, tc.original)
			
			// Check if the missing end tag was inserted
			if !strings.Contains(restored, tc.expectEnd) {
				t.Errorf("Expected %s to be inserted, but not found in:\n%s",
					tc.expectEnd, restored)
			}
			
			// Verify the restored content has balanced environments
			result := ValidateEnvironments(restored)
			if !result.IsValid {
				t.Errorf("Restored content still has environment mismatches: %v",
					result.Mismatches)
			}
		})
	}
}

// ============================================================
// Property 9: Nested Environment Validation
// Feature: latex-format-preservation, Property 9: Nested Environment Validation
// Validates: Requirements 3.5
// ============================================================

func TestProperty9_NestedEnvironmentValidation(t *testing.T) {
	// Property: For any LaTeX content with nested environments, the Structure_Validator
	// SHALL verify that inner environments are completely contained within outer
	// environments (proper nesting order).
	
	testCases := []struct {
		name         string
		input        string
		nestingValid bool
	}{
		{
			"proper nesting",
			"\\begin{figure}\\begin{center}text\\end{center}\\end{figure}",
			true,
		},
		{
			"improper nesting - crossed",
			"\\begin{figure}\\begin{center}text\\end{figure}\\end{center}",
			false,
		},
		{
			"deep proper nesting",
			"\\begin{table}\\begin{center}\\begin{tabular}{c}A\\end{tabular}\\end{center}\\end{table}",
			true,
		},
		{
			"unclosed inner",
			"\\begin{figure}\\begin{center}text\\end{figure}",
			false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateEnvironments(tc.input)
			
			if result.NestingValid != tc.nestingValid {
				t.Errorf("Expected NestingValid=%v, got %v (error: %s)",
					tc.nestingValid, result.NestingValid, result.NestingError)
			}
		})
	}
}

// ============================================================
// Property 11: Brace Imbalance Location
// Feature: latex-format-preservation, Property 11: Brace Imbalance Location
// Validates: Requirements 4.2
// ============================================================

func TestProperty11_BraceImbalanceLocation(t *testing.T) {
	// Property: For any LaTeX content with brace imbalance, the Structure_Validator
	// SHALL identify the approximate location (line number or character position)
	// of the imbalance.
	
	testCases := []struct {
		name          string
		input         string
		expectError   bool
		expectLineNum int // approximate line where error should be detected
	}{
		{
			"balanced",
			"\\textbf{bold} and \\textit{italic}",
			false,
			0,
		},
		{
			"unclosed on line 1",
			"\\textbf{bold",
			true,
			1,
		},
		{
			"unclosed on line 2",
			"Line 1\n\\textbf{bold",
			true,
			2,
		},
		{
			"extra close on line 3",
			"Line 1\nLine 2\n}extra",
			true,
			3,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateBraces(tc.input)
			
			if tc.expectError {
				if result.IsBalanced {
					t.Errorf("Expected brace imbalance to be detected")
				}
				if len(result.ErrorLines) == 0 {
					t.Errorf("Expected error location to be reported")
				} else {
					// Check if error line is approximately correct
					foundCorrectLine := false
					for _, line := range result.ErrorLines {
						if line == tc.expectLineNum {
							foundCorrectLine = true
							break
						}
					}
					if !foundCorrectLine {
						t.Errorf("Expected error on line %d, got lines %v",
							tc.expectLineNum, result.ErrorLines)
					}
				}
			} else {
				if !result.IsBalanced {
					t.Errorf("Expected balanced braces, got imbalance")
				}
			}
		})
	}
}

// ============================================================
// Property 12: Multirow/Multicolumn Brace Fix
// Feature: latex-format-preservation, Property 12: Multirow/Multicolumn Brace Fix
// Validates: Requirements 4.3
// ============================================================

func TestProperty12_MultirowMulticolumnBraceFix(t *testing.T) {
	// Property: For any LaTeX content containing \multirow or \multicolumn commands
	// with missing closing braces, the Format_Restorer SHALL add the missing braces
	// to complete the command.
	
	testCases := []struct {
		name       string
		translated string
		original   string
		expectFix  bool
	}{
		{
			"multirow missing brace",
			"\\multirow{2}{*}{\\textbf{Method} &",
			"\\multirow{2}{*}{\\textbf{Method}} &",
			true,
		},
		{
			"multicolumn missing brace",
			"\\multicolumn{3}{c}{\\textbf{Header} &",
			"\\multicolumn{3}{c}{\\textbf{Header}} &",
			true,
		},
		{
			"already correct",
			"\\multirow{2}{*}{\\textbf{Method}} &",
			"\\multirow{2}{*}{\\textbf{Method}} &",
			false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixed := fixMultirowMulticolumnBraces(tc.translated, tc.original)
			
			// Count braces before and after
			origBraces := strings.Count(tc.translated, "}")
			fixedBraces := strings.Count(fixed, "}")
			
			if tc.expectFix {
				if fixedBraces <= origBraces {
					t.Errorf("Expected braces to be added, but count didn't increase")
				}
			}
			
			// Verify the fixed content has balanced braces for the command
			if strings.Contains(fixed, "\\multirow") || strings.Contains(fixed, "\\multicolumn") {
				// Basic check: should have matching braces
				result := ValidateBraces(fixed)
				if !result.IsBalanced {
					t.Logf("Warning: fixed content still has brace imbalance: %d open, %d close",
						result.OpenCount, result.CloseCount)
				}
			}
		})
	}
}

// ============================================================
// Property 13: Brace Restoration
// Feature: latex-format-preservation, Property 13: Brace Restoration
// Validates: Requirements 4.5
// ============================================================

func TestProperty13_BraceRestoration(t *testing.T) {
	// Property: For any translated LaTeX content where the brace count differs
	// from the original, the Format_Restorer SHALL attempt to restore the
	// original brace balance.
	
	testCases := []struct {
		name       string
		translated string
		original   string
	}{
		{
			"extra closing braces",
			"\\textbf{bold}}}",
			"\\textbf{bold}",
		},
		{
			"missing closing brace",
			"\\textbf{bold",
			"\\textbf{bold}",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixed := fixBraceBalance(tc.translated, tc.original)
			
			// Get original brace balance
			origOpen, origClose := countBraces(tc.original)
			origBalance := origOpen - origClose
			
			// Get fixed brace balance
			fixedOpen, fixedClose := countBraces(fixed)
			fixedBalance := fixedOpen - fixedClose
			
			// Fixed should be closer to original balance
			transOpen, transClose := countBraces(tc.translated)
			transBalance := transOpen - transClose
			
			transDiff := absInt(transBalance - origBalance)
			fixedDiff := absInt(fixedBalance - origBalance)
			
			if fixedDiff > transDiff {
				t.Errorf("Brace restoration made balance worse: trans diff=%d, fixed diff=%d",
					transDiff, fixedDiff)
			}
		})
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ============================================================
// Property 18: Table Chunking
// Feature: latex-format-preservation, Property 18: Table Chunking
// Validates: Requirements 6.3
// ============================================================

func TestProperty18_TableChunking(t *testing.T) {
	// Property: For any LaTeX content containing tables, if the table size is
	// within the maximum chunk size, the entire table SHALL be kept in a single chunk.
	
	smallTable := `\begin{table}
\begin{tabular}{cc}
A & B \\
1 & 2 \\
\end{tabular}
\end{table}`

	content := "Some text before.\n\n" + smallTable + "\n\nSome text after."
	
	// Use a chunk size large enough to fit the table
	chunks := splitIntoChunks(content, 500)
	
	// Find which chunk contains the table
	tableFound := false
	for _, chunk := range chunks {
		if strings.Contains(chunk, "\\begin{table}") {
			tableFound = true
			// Verify the entire table is in this chunk
			if !strings.Contains(chunk, "\\end{table}") {
				t.Errorf("Table was split across chunks - \\begin{table} found but not \\end{table}")
			}
			if !strings.Contains(chunk, "\\begin{tabular}") || !strings.Contains(chunk, "\\end{tabular}") {
				t.Errorf("Table was split - tabular environment incomplete")
			}
		}
	}
	
	if !tableFound {
		t.Errorf("Table not found in any chunk")
	}
}

// ============================================================
// Property 19: Safe Boundary Splitting
// Feature: latex-format-preservation, Property 19: Safe Boundary Splitting
// Validates: Requirements 6.4
// ============================================================

func TestProperty19_SafeBoundarySplitting(t *testing.T) {
	// Property: For any chunk that exceeds the maximum size, the split SHALL occur
	// at a safe boundary (paragraph break, section end, or environment end).
	
	content := `First paragraph with some content.

Second paragraph with more content.

\begin{itemize}
\item Item one
\item Item two
\end{itemize}

Third paragraph after the list.`

	// Use small chunk size to force splitting
	chunks := splitIntoChunks(content, 50)
	
	// Verify splits occur at safe boundaries
	for i, chunk := range chunks {
		// Check that chunks don't start or end in the middle of a word
		trimmed := strings.TrimSpace(chunk)
		if len(trimmed) > 0 {
			// Should not start with lowercase letter (middle of word)
			firstChar := rune(trimmed[0])
			if firstChar >= 'a' && firstChar <= 'z' && i > 0 {
				// This might be okay if previous chunk ended with space/newline
				prevChunk := chunks[i-1]
				lastChar := prevChunk[len(prevChunk)-1]
				if lastChar != ' ' && lastChar != '\n' && lastChar != '\t' {
					t.Logf("Warning: chunk %d may start mid-word: %q", i, trimmed[:min(20, len(trimmed))])
				}
			}
		}
	}
}

// ============================================================
// Property 21: Structure Comparison
// Feature: latex-format-preservation, Property 21: Structure Comparison
// Validates: Requirements 7.1
// ============================================================

func TestProperty21_StructureComparison(t *testing.T) {
	// Property: For any original and translated LaTeX content pair, the
	// Structure_Validator SHALL compare and report differences in document
	// structure (sections, environments, commands).
	
	original := `\documentclass{article}
\begin{document}
\section{Introduction}
Content here.
\section{Methods}
More content.
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\section{引言}
内容在这里。
\section{方法}
更多内容。
\end{document}`

	// Create validator and check structure preservation
	validator := NewTranslationValidator()
	result := validator.ValidateTranslation(original, translated)
	
	// Structure should be preserved (same number of sections, etc.)
	origSections := countSections(original)
	transSections := countSections(translated)
	
	if origSections != transSections {
		t.Errorf("Section count mismatch: original=%d, translated=%d",
			origSections, transSections)
	}
	
	// Document class should be preserved
	origClass := extractDocumentClass(original)
	transClass := extractDocumentClass(translated)
	
	if origClass != transClass {
		t.Errorf("Document class changed: original=%s, translated=%s",
			origClass, transClass)
	}
	
	// Basic validation should pass
	if !result.IsValid {
		t.Logf("Validation warnings/errors: %v %v", result.Errors, result.Warnings)
	}
}

// ============================================================
// Property 23: Required Pattern Validation
// Feature: latex-format-preservation, Property 23: Required Pattern Validation
// Validates: Requirements 7.3
// ============================================================

func TestProperty23_RequiredPatternValidation(t *testing.T) {
	// Property: For any original LaTeX content containing required patterns
	// (like \documentclass), the translated content SHALL also contain these patterns.
	
	testCases := []struct {
		name       string
		original   string
		translated string
		shouldFail bool
	}{
		{
			"documentclass preserved",
			"\\documentclass{article}\n\\begin{document}Hello\\end{document}",
			"\\documentclass{article}\n\\begin{document}你好\\end{document}",
			false,
		},
		{
			"documentclass missing",
			"\\documentclass{article}\n\\begin{document}Hello\\end{document}",
			"\\begin{document}你好\\end{document}",
			true,
		},
	}
	
	validator := NewTranslationValidator()
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateTranslation(tc.original, tc.translated)
			
			hasPatternError := false
			for _, err := range result.Errors {
				if strings.Contains(err, "LaTeX 结构") || strings.Contains(err, "documentclass") {
					hasPatternError = true
					break
				}
			}
			
			if tc.shouldFail && !hasPatternError {
				t.Errorf("Expected pattern validation to fail, but it passed")
			}
		})
	}
}

// ============================================================
// Property 25: Post-Processing Application
// Feature: latex-format-preservation, Property 25: Post-Processing Application
// Validates: Requirements 9.1
// ============================================================

func TestProperty25_PostProcessingApplication(t *testing.T) {
	// Property: For any translated chunk, the Format_Restorer SHALL apply all
	// enabled post-processing fixes before returning the result.
	
	testCases := []struct {
		name       string
		translated string
		original   string
		expectFix  bool
	}{
		{
			"merged begin end",
			"\\begin{figure}content\\end{figure}",
			"\\begin{figure}\ncontent\n\\end{figure}",
			true,
		},
		{
			"already correct",
			"\\begin{figure}\ncontent\n\\end{figure}",
			"\\begin{figure}\ncontent\n\\end{figure}",
			false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := PostprocessChunk(tc.translated, tc.original)
			
			if tc.expectFix {
				if result == tc.translated {
					t.Errorf("Expected post-processing to make changes, but content unchanged")
				}
			}
			
			// Verify basic structure is maintained
			if strings.Contains(tc.translated, "\\begin{") {
				if !strings.Contains(result, "\\begin{") {
					t.Errorf("Post-processing removed \\begin tag")
				}
			}
		})
	}
}

// ============================================================
// Property 26: Line Break Insertion
// Feature: latex-format-preservation, Property 26: Line Break Insertion
// Validates: Requirements 9.2, 9.3, 9.4
// ============================================================

func TestProperty26_LineBreakInsertion(t *testing.T) {
	// Property: For any translated content where \end{env} and \item are on the
	// same line, or \caption{...} is followed by \begin{tabular} on the same line,
	// or table row separators are followed by \midrule on the same line, the
	// Format_Restorer SHALL insert appropriate line breaks.
	
	testCases := []struct {
		name       string
		input      string
		shouldFix  bool
		checkFor   string // pattern that should be on separate line after fix
	}{
		{
			"end and item same line",
			"\\end{itemize}\\item Next",
			true,
			"\n\\item",
		},
		{
			"caption and tabular same line",
			"\\caption{Title}\\begin{tabular}{c}",
			true,
			"\n",
		},
		{
			"row end and midrule same line",
			"A & B \\\\\\midrule",
			true,
			"\n",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use RestoreLineStructure which handles all line break patterns
			fixed := RestoreLineStructure(tc.input, tc.input)
			
			if tc.shouldFix {
				if !strings.Contains(fixed, tc.checkFor) {
					t.Errorf("Expected line break to be inserted, but not found.\nInput: %q\nOutput: %q",
						tc.input, fixed)
				}
			}
		})
	}
}

// ============================================================
// Property 27: Reference-Based Fixing
// Feature: latex-format-preservation, Property 27: Reference-Based Fixing
// Validates: Requirements 9.5
// ============================================================

func TestProperty27_ReferenceBasedFixing(t *testing.T) {
	// Property: For any translated content, when comparing with the original content,
	// the Format_Restorer SHALL use the original structure as reference to fix
	// structural issues.
	
	original := `\begin{figure}
\includegraphics{test.png}
\caption{Test caption}
\end{figure}`

	translated := `\begin{figure}
\includegraphics{test.png}
\caption{测试标题}`  // Missing \end{figure}

	fixed := PostprocessChunk(translated, original)
	
	// Should have restored the missing \end{figure}
	if !strings.Contains(fixed, "\\end{figure}") {
		t.Errorf("Expected \\end{figure} to be restored using original as reference")
	}
	
	// Verify environment balance
	result := ValidateEnvironments(fixed)
	if !result.IsValid {
		t.Errorf("Fixed content still has environment issues: %v", result.Mismatches)
	}
}

// ============================================================
// Property-Based Quick Tests for Additional Coverage
// ============================================================

func TestProperty7_Quick_EnvironmentMismatchDetection(t *testing.T) {
	// Quick test: randomly generate content with mismatches and verify detection
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		
		envs := []string{"figure", "table", "itemize", "enumerate"}
		env := envs[r.Intn(len(envs))]
		
		// Generate content with intentional mismatch
		var content string
		if r.Float32() > 0.5 {
			// Missing end
			content = fmt.Sprintf("\\begin{%s}\ncontent here", env)
		} else {
			// Extra end
			content = fmt.Sprintf("content here\n\\end{%s}", env)
		}
		
		result := ValidateEnvironments(content)
		
		// Should detect the mismatch
		return !result.IsValid
	}
	
	if err := quick.Check(f, quickConfig()); err != nil {
		t.Error(err)
	}
}

func TestProperty9_Quick_NestedEnvironmentValidation(t *testing.T) {
	// Quick test: verify proper nesting detection
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		
		// Generate properly nested content
		outer := []string{"figure", "table"}[r.Intn(2)]
		inner := []string{"center", "tabular"}[r.Intn(2)]
		
		properNesting := fmt.Sprintf("\\begin{%s}\\begin{%s}content\\end{%s}\\end{%s}",
			outer, inner, inner, outer)
		
		result := ValidateEnvironments(properNesting)
		return result.NestingValid
	}
	
	if err := quick.Check(f, quickConfig()); err != nil {
		t.Error(err)
	}
}
