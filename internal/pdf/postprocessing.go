// Package pdf provides YOLO output postprocessing
package pdf

import (
	"fmt"
	"math"
	"sort"

	"latex-translator/internal/logger"
)

// Detection represents a raw detection from YOLO
type Detection struct {
	Box        BoundingBox
	ClassID    int
	Confidence float64
}

// PostProcessor handles YOLO output postprocessing
type PostProcessor struct {
	confThreshold float64
	nmsThreshold  float64
	classNames    []ElementType
}

// NewPostProcessor creates a new postprocessor
func NewPostProcessor(confThreshold, nmsThreshold float64) *PostProcessor {
	return &PostProcessor{
		confThreshold: confThreshold,
		nmsThreshold:  nmsThreshold,
		classNames:    getDocLayoutClassNames(),
	}
}

// getDocLayoutClassNames returns the class names for DocLayout-YOLO
func getDocLayoutClassNames() []ElementType {
	return []ElementType{
		ElementText,          // 0
		ElementTitle,         // 1
		ElementPicture,       // 2
		ElementCaption,       // 3
		ElementSectionHeader, // 4
		ElementFootnote,      // 5
		ElementFormula,       // 6
		ElementTable,         // 7
		ElementListItem,      // 8
		ElementPageHeader,    // 9
		ElementPageFooter,    // 10
	}
}

// ProcessYOLOOutput processes raw YOLO output to layout elements
func (p *PostProcessor) ProcessYOLOOutput(output []float32, imgWidth, imgHeight int, pageNum int) ([]LayoutElement, error) {
	logger.Debug("processing YOLO output",
		logger.Int("outputSize", len(output)),
		logger.Int("imgWidth", imgWidth),
		logger.Int("imgHeight", imgHeight))

	// 1. Parse detections from raw output
	detections := p.parseDetections(output, imgWidth, imgHeight)
	logger.Debug("parsed detections", logger.Int("count", len(detections)))

	// 2. Filter by confidence threshold
	filtered := p.filterByConfidence(detections)
	logger.Debug("after confidence filter", logger.Int("count", len(filtered)))

	// 3. Apply NMS per class
	nmsResults := p.applyNMSPerClass(filtered)
	logger.Debug("after NMS", logger.Int("count", len(nmsResults)))

	// 4. Convert to LayoutElements
	elements := p.detectionsToElements(nmsResults, pageNum)

	logger.Info("postprocessing complete",
		logger.Int("finalElements", len(elements)),
		logger.Int("page", pageNum))

	return elements, nil
}

// parseDetections parses YOLO output format
// YOLO output format: [num_detections, 6] where each detection is [x, y, w, h, confidence, class_id]
func (p *PostProcessor) parseDetections(output []float32, imgWidth, imgHeight int) []Detection {
	var detections []Detection

	// YOLO output is typically in format: [batch, num_boxes, 5+num_classes]
	// For simplicity, assuming format: [num_detections * 6]
	numDetections := len(output) / 6

	for i := 0; i < numDetections; i++ {
		offset := i * 6

		// Extract values
		x := float64(output[offset])
		y := float64(output[offset+1])
		w := float64(output[offset+2])
		h := float64(output[offset+3])
		conf := float64(output[offset+4])
		classID := int(output[offset+5])

		// Convert from normalized [0, 1] to pixel coordinates
		detection := Detection{
			Box: BoundingBox{
				X:      x * float64(imgWidth),
				Y:      y * float64(imgHeight),
				Width:  w * float64(imgWidth),
				Height: h * float64(imgHeight),
			},
			ClassID:    classID,
			Confidence: conf,
		}

		detections = append(detections, detection)
	}

	return detections
}

// filterByConfidence filters detections by confidence threshold
func (p *PostProcessor) filterByConfidence(detections []Detection) []Detection {
	var filtered []Detection

	for _, det := range detections {
		if det.Confidence >= p.confThreshold {
			filtered = append(filtered, det)
		}
	}

	return filtered
}

// applyNMSPerClass applies Non-Maximum Suppression per class
func (p *PostProcessor) applyNMSPerClass(detections []Detection) []Detection {
	// Group detections by class
	detectionsByClass := make(map[int][]Detection)
	for _, det := range detections {
		detectionsByClass[det.ClassID] = append(detectionsByClass[det.ClassID], det)
	}

	// Apply NMS to each class
	var results []Detection
	for _, classDetections := range detectionsByClass {
		nmsResults := p.applyNMS(classDetections)
		results = append(results, nmsResults...)
	}

	return results
}

// applyNMS applies Non-Maximum Suppression
func (p *PostProcessor) applyNMS(detections []Detection) []Detection {
	if len(detections) == 0 {
		return nil
	}

	// Sort by confidence (descending)
	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Confidence > detections[j].Confidence
	})

	var keep []Detection

	for len(detections) > 0 {
		// Keep the detection with highest confidence
		best := detections[0]
		keep = append(keep, best)

		// Filter out detections with high IoU
		var remaining []Detection
		for _, det := range detections[1:] {
			if iou(best.Box, det.Box) < p.nmsThreshold {
				remaining = append(remaining, det)
			}
		}

		detections = remaining
	}

	return keep
}

// iou calculates Intersection over Union
func iou(box1, box2 BoundingBox) float64 {
	// Calculate intersection
	x1 := math.Max(box1.X, box2.X)
	y1 := math.Max(box1.Y, box2.Y)
	x2 := math.Min(box1.X+box1.Width, box2.X+box2.Width)
	y2 := math.Min(box1.Y+box1.Height, box2.Y+box2.Height)

	// No intersection
	if x2 < x1 || y2 < y1 {
		return 0
	}

	intersection := (x2 - x1) * (y2 - y1)

	// Calculate union
	area1 := box1.Width * box1.Height
	area2 := box2.Width * box2.Height
	union := area1 + area2 - intersection

	if union == 0 {
		return 0
	}

	return intersection / union
}

// detectionsToElements converts detections to LayoutElements
func (p *PostProcessor) detectionsToElements(detections []Detection, pageNum int) []LayoutElement {
	elements := make([]LayoutElement, len(detections))

	for i, det := range detections {
		// Map class ID to element type
		var elementType ElementType
		if det.ClassID >= 0 && det.ClassID < len(p.classNames) {
			elementType = p.classNames[det.ClassID]
		} else {
			elementType = ElementText // Default
		}

		elements[i] = LayoutElement{
			Type:        elementType,
			BoundingBox: det.Box,
			Confidence:  det.Confidence,
			Page:        pageNum,
		}
	}

	return elements
}

// MergeLayoutWithText merges AI-detected layout with extracted text
func MergeLayoutWithText(layoutElements []LayoutElement, textBlocks []TextBlock) []TextBlock {
	logger.Info("merging layout with text",
		logger.Int("layoutElements", len(layoutElements)),
		logger.Int("textBlocks", len(textBlocks)))

	// For each text block, find the best matching layout element
	for i := range textBlocks {
		bestMatch := findBestLayoutMatch(&textBlocks[i], layoutElements)
		if bestMatch != nil {
			// Update block type based on layout detection
			textBlocks[i].BlockType = string(bestMatch.Type)
			logger.Debug("matched text block to layout",
				logger.String("blockID", textBlocks[i].ID),
				logger.String("type", string(bestMatch.Type)),
				logger.Float64("confidence", bestMatch.Confidence))
		}
	}

	return textBlocks
}

// findBestLayoutMatch finds the layout element that best matches a text block
func findBestLayoutMatch(textBlock *TextBlock, layoutElements []LayoutElement) *LayoutElement {
	var bestMatch *LayoutElement
	maxOverlap := 0.0

	textBox := BoundingBox{
		X:      textBlock.X,
		Y:      textBlock.Y,
		Width:  textBlock.Width,
		Height: textBlock.Height,
	}

	for i := range layoutElements {
		// Only consider elements on the same page
		if layoutElements[i].Page != textBlock.Page {
			continue
		}

		// Calculate overlap (IoU)
		overlap := iou(textBox, layoutElements[i].BoundingBox)

		if overlap > maxOverlap {
			maxOverlap = overlap
			bestMatch = &layoutElements[i]
		}
	}

	// Only return match if overlap is significant (>30%)
	if maxOverlap < 0.3 {
		return nil
	}

	return bestMatch
}

// FilterTranslatableElements filters elements that should be translated
func FilterTranslatableElements(elements []LayoutElement) []LayoutElement {
	var translatable []LayoutElement

	for _, elem := range elements {
		if elem.Type.IsTranslatable() {
			translatable = append(translatable, elem)
		}
	}

	// Sort by priority (higher priority first)
	sort.Slice(translatable, func(i, j int) bool {
		return translatable[i].Type.Priority() > translatable[j].Type.Priority()
	})

	logger.Info("filtered translatable elements",
		logger.Int("total", len(elements)),
		logger.Int("translatable", len(translatable)))

	return translatable
}

// GroupElementsByType groups elements by their type
func GroupElementsByType(elements []LayoutElement) map[ElementType][]LayoutElement {
	groups := make(map[ElementType][]LayoutElement)

	for _, elem := range elements {
		groups[elem.Type] = append(groups[elem.Type], elem)
	}

	return groups
}

// GetElementStatistics returns statistics about detected elements
func GetElementStatistics(elements []LayoutElement) map[ElementType]int {
	stats := make(map[ElementType]int)

	for _, elem := range elements {
		stats[elem.Type]++
	}

	return stats
}

// ValidateDetections validates detection results
func ValidateDetections(detections []Detection, imgWidth, imgHeight int) error {
	for i, det := range detections {
		// Check if bounding box is within image bounds
		if det.Box.X < 0 || det.Box.Y < 0 ||
			det.Box.X+det.Box.Width > float64(imgWidth) ||
			det.Box.Y+det.Box.Height > float64(imgHeight) {
			return fmt.Errorf("detection %d has invalid bounding box: %+v", i, det.Box)
		}

		// Check confidence range
		if det.Confidence < 0 || det.Confidence > 1 {
			return fmt.Errorf("detection %d has invalid confidence: %f", i, det.Confidence)
		}

		// Check class ID
		if det.ClassID < 0 {
			return fmt.Errorf("detection %d has invalid class ID: %d", i, det.ClassID)
		}
	}

	return nil
}
