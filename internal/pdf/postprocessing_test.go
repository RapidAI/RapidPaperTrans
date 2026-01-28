package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIOU(t *testing.T) {
	tests := []struct {
		name     string
		box1     BoundingBox
		box2     BoundingBox
		expected float64
	}{
		{
			name:     "identical boxes",
			box1:     BoundingBox{X: 0, Y: 0, Width: 100, Height: 100},
			box2:     BoundingBox{X: 0, Y: 0, Width: 100, Height: 100},
			expected: 1.0,
		},
		{
			name:     "no overlap",
			box1:     BoundingBox{X: 0, Y: 0, Width: 50, Height: 50},
			box2:     BoundingBox{X: 100, Y: 100, Width: 50, Height: 50},
			expected: 0.0,
		},
		{
			name:     "partial overlap",
			box1:     BoundingBox{X: 0, Y: 0, Width: 100, Height: 100},
			box2:     BoundingBox{X: 50, Y: 50, Width: 100, Height: 100},
			expected: 0.143, // intersection: 2500, union: 10000 + 10000 - 2500 = 17500, iou: 2500/17500 ≈ 0.143
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := iou(tt.box1, tt.box2)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestPostProcessor_FilterByConfidence(t *testing.T) {
	processor := NewPostProcessor(0.5, 0.5)

	detections := []Detection{
		{Confidence: 0.9, ClassID: 0},
		{Confidence: 0.3, ClassID: 1},
		{Confidence: 0.7, ClassID: 2},
		{Confidence: 0.4, ClassID: 3},
	}

	filtered := processor.filterByConfidence(detections)

	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, 0.9, filtered[0].Confidence)
	assert.Equal(t, 0.7, filtered[1].Confidence)
}

func TestPostProcessor_ApplyNMS(t *testing.T) {
	processor := NewPostProcessor(0.5, 0.5)

	detections := []Detection{
		{
			Box:        BoundingBox{X: 0, Y: 0, Width: 100, Height: 100},
			Confidence: 0.9,
			ClassID:    0,
		},
		{
			Box:        BoundingBox{X: 10, Y: 10, Width: 100, Height: 100},
			Confidence: 0.8,
			ClassID:    0,
		},
		{
			Box:        BoundingBox{X: 200, Y: 200, Width: 100, Height: 100},
			Confidence: 0.7,
			ClassID:    0,
		},
	}

	result := processor.applyNMS(detections)

	// Should keep the first (highest confidence) and third (no overlap)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, 0.9, result[0].Confidence)
	assert.Equal(t, 0.7, result[1].Confidence)
}

func TestMergeLayoutWithText(t *testing.T) {
	layoutElements := []LayoutElement{
		{
			Type: ElementTitle,
			BoundingBox: BoundingBox{
				X:      10,
				Y:      10,
				Width:  200,
				Height: 50,
			},
			Page: 1,
		},
		{
			Type: ElementFormula,
			BoundingBox: BoundingBox{
				X:      10,
				Y:      100,
				Width:  200,
				Height: 30,
			},
			Page: 1,
		},
	}

	textBlocks := []TextBlock{
		{
			ID:        "block_1",
			Page:      1,
			Text:      "Introduction",
			X:         15,
			Y:         15,
			Width:     190,
			Height:    45,
			BlockType: "paragraph",
		},
		{
			ID:        "block_2",
			Page:      1,
			Text:      "E = mc^2",
			X:         15,
			Y:         105,
			Width:     190,
			Height:    25,
			BlockType: "paragraph",
		},
	}

	merged := MergeLayoutWithText(layoutElements, textBlocks)

	assert.Equal(t, 2, len(merged))
	assert.Equal(t, "title", merged[0].BlockType)
	assert.Equal(t, "formula", merged[1].BlockType)
}

func TestFilterTranslatableElements(t *testing.T) {
	elements := []LayoutElement{
		{Type: ElementText},
		{Type: ElementTitle},
		{Type: ElementFormula},
		{Type: ElementPicture},
		{Type: ElementCaption},
	}

	translatable := FilterTranslatableElements(elements)

	assert.Equal(t, 3, len(translatable))

	// Check that formulas and pictures are filtered out
	for _, elem := range translatable {
		assert.NotEqual(t, ElementFormula, elem.Type)
		assert.NotEqual(t, ElementPicture, elem.Type)
	}

	// Check priority sorting (Title should be first)
	assert.Equal(t, ElementTitle, translatable[0].Type)
}

func TestGetElementStatistics(t *testing.T) {
	elements := []LayoutElement{
		{Type: ElementText},
		{Type: ElementText},
		{Type: ElementTitle},
		{Type: ElementFormula},
		{Type: ElementFormula},
		{Type: ElementFormula},
	}

	stats := GetElementStatistics(elements)

	assert.Equal(t, 2, stats[ElementText])
	assert.Equal(t, 1, stats[ElementTitle])
	assert.Equal(t, 3, stats[ElementFormula])
}
