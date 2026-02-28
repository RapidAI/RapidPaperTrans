// Package pdf provides PDF translation functionality with AI-powered layout detection
package pdf

import (
	"fmt"
	_ "image/png" // register PNG decoder
	"path/filepath"

	"latex-translator/internal/logger"
)

// LayoutElement represents a detected element in the PDF layout
type LayoutElement struct {
	Type        ElementType `json:"type"`
	BoundingBox BoundingBox `json:"bounding_box"`
	Confidence  float64     `json:"confidence"`
	Page        int         `json:"page"`
	Content     string      `json:"content,omitempty"`
}

// ElementType defines the type of layout element
type ElementType string

const (
	ElementText          ElementType = "text"
	ElementTitle         ElementType = "title"
	ElementPicture       ElementType = "picture"
	ElementCaption       ElementType = "caption"
	ElementSectionHeader ElementType = "section_header"
	ElementFootnote      ElementType = "footnote"
	ElementFormula       ElementType = "formula"
	ElementTable         ElementType = "table"
	ElementListItem      ElementType = "list_item"
	ElementPageHeader    ElementType = "page_header"
	ElementPageFooter    ElementType = "page_footer"
)

// BoundingBox represents the position and size of an element
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// LayoutDetector handles AI-powered layout detection using DocLayout-YOLO ONNX model
type LayoutDetector struct {
	engine  *ONNXEngine
	enabled bool
	workDir string
}

// LayoutDetectorConfig holds configuration for layout detector
type LayoutDetectorConfig struct {
	ModelPath string // unused now (model is embedded)
	Enabled   bool
	WorkDir   string
}

// NewLayoutDetector creates a new layout detector.
// When Enabled=true, it initializes the ONNX runtime with the embedded DocLayout-YOLO model.
func NewLayoutDetector(config LayoutDetectorConfig) (*LayoutDetector, error) {
	detector := &LayoutDetector{
		enabled: config.Enabled,
		workDir: config.WorkDir,
	}

	if config.Enabled {
		if err := detector.init(); err != nil {
			logger.Warn("failed to load layout detection model, falling back to rule-based detection",
				logger.Err(err))
			detector.enabled = false
		}
	}

	return detector, nil
}

// init initializes the ONNX engine
func (d *LayoutDetector) init() error {
	workDir := d.workDir
	if workDir == "" {
		workDir = "."
	}

	engine := GetONNXEngine()
	if err := engine.Initialize(workDir); err != nil {
		return fmt.Errorf("ONNX engine init failed: %w", err)
	}
	d.engine = engine

	logger.Info("layout detection model loaded via ONNX runtime")
	return nil
}

// Close releases resources
func (d *LayoutDetector) Close() {
	// Don't destroy the singleton engine here; it may be shared
}

// DetectLayout detects layout elements in a PDF page.
// Uses ONNX model if enabled, otherwise falls back to rule-based heuristics.
func (d *LayoutDetector) DetectLayout(pdfPath string, pageNum int) ([]LayoutElement, error) {
	if !d.enabled || d.engine == nil {
		return d.detectLayoutRuleBased(pdfPath, pageNum)
	}
	return d.detectLayoutAI(pdfPath, pageNum)
}

// detectLayoutAI uses the DocLayout-YOLO ONNX model
func (d *LayoutDetector) detectLayoutAI(pdfPath string, pageNum int) ([]LayoutElement, error) {
	logger.Info("detecting layout with AI model",
		logger.String("pdf", filepath.Base(pdfPath)),
		logger.Int("page", pageNum))

	// Convert PDF page to image using GoPDF2 (pure Go, no external deps)
	img, err := ConvertPDFPageToImage(pdfPath, pageNum, 150)
	if err != nil {
		logger.Warn("PDF to image conversion failed, falling back to rule-based",
			logger.Err(err))
		return d.detectLayoutRuleBased(pdfPath, pageNum)
	}

	bounds := img.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()

	// Run ONNX inference
	detections, err := d.engine.DetectLayoutFromImage(img, imgW, imgH)
	if err != nil {
		logger.Warn("ONNX inference failed, falling back to rule-based",
			logger.Err(err))
		return d.detectLayoutRuleBased(pdfPath, pageNum)
	}

	// Convert pixel-space detections to LayoutElements
	var elements []LayoutElement
	classNames := getDocLayoutClassNames()
	for _, det := range detections {
		elemType := ElementText
		if det.ClassID >= 0 && det.ClassID < len(classNames) {
			elemType = classNames[det.ClassID]
		}
		elements = append(elements, LayoutElement{
			Type:       elemType,
			BoundingBox: det.Box,
			Confidence: det.Confidence,
			Page:       pageNum,
		})
	}

	logger.Info("AI layout detection complete",
		logger.Int("detections", len(elements)),
		logger.Int("page", pageNum))

	return elements, nil
}

// detectLayoutRuleBased uses rule-based heuristics (fallback)
func (d *LayoutDetector) detectLayoutRuleBased(pdfPath string, pageNum int) ([]LayoutElement, error) {
	logger.Info("detecting layout with rule-based method",
		logger.String("pdf", filepath.Base(pdfPath)),
		logger.Int("page", pageNum))

	parser := NewPDFParser("")
	textBlocks, err := parser.ExtractText(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	var elements []LayoutElement
	for _, block := range textBlocks {
		if block.Page != pageNum {
			continue
		}
		elements = append(elements, LayoutElement{
			Type: ElementType(block.BlockType),
			BoundingBox: BoundingBox{
				X:      block.X,
				Y:      block.Y,
				Width:  block.Width,
				Height: block.Height,
			},
			Confidence: 1.0,
			Page:       block.Page,
			Content:    block.Text,
		})
	}

	logger.Info("rule-based detection complete",
		logger.Int("elements", len(elements)))
	return elements, nil
}

// IsTranslatable returns whether an element type should be translated
func (e ElementType) IsTranslatable() bool {
	switch e {
	case ElementText, ElementTitle, ElementCaption,
		ElementSectionHeader, ElementFootnote, ElementListItem:
		return true
	case ElementFormula, ElementPicture, ElementTable,
		ElementPageHeader, ElementPageFooter:
		return false
	default:
		return true
	}
}

// Priority returns the translation priority (higher = translate first)
func (e ElementType) Priority() int {
	switch e {
	case ElementTitle:
		return 100
	case ElementSectionHeader:
		return 90
	case ElementText:
		return 80
	case ElementCaption:
		return 70
	case ElementListItem:
		return 60
	case ElementFootnote:
		return 50
	default:
		return 0
	}
}

// ConvertToTextBlock converts a LayoutElement to TextBlock
func (e *LayoutElement) ConvertToTextBlock(blockID string) TextBlock {
	return TextBlock{
		ID:        blockID,
		Page:      e.Page,
		Text:      e.Content,
		X:         e.BoundingBox.X,
		Y:         e.BoundingBox.Y,
		Width:     e.BoundingBox.Width,
		Height:    e.BoundingBox.Height,
		BlockType: string(e.Type),
	}
}

// DownloadModel is a no-op since the model is embedded in the binary.
func DownloadModel(modelPath string) error {
	logger.Info("model is embedded in binary, no download needed")
	return nil
}
