package pdf

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayoutDetector_RuleBased(t *testing.T) {
	// Skip if no test PDF available
	testPDF := "../../testdata/test.pdf"
	if _, err := os.Stat(testPDF); os.IsNotExist(err) {
		t.Skip("test PDF not found")
	}

	detector, err := NewLayoutDetector(LayoutDetectorConfig{
		ModelPath: "",
		Enabled:   false, // Use rule-based
	})
	require.NoError(t, err)

	elements, err := detector.DetectLayout(testPDF, 1)
	require.NoError(t, err)

	assert.Greater(t, len(elements), 0, "should detect at least one element")

	// Check element properties
	for _, elem := range elements {
		assert.NotEmpty(t, elem.Type)
		assert.Greater(t, elem.Confidence, 0.0)
		assert.Equal(t, 1, elem.Page)
		assert.Greater(t, elem.BoundingBox.Width, 0.0)
		assert.Greater(t, elem.BoundingBox.Height, 0.0)
	}
}

func TestElementType_IsTranslatable(t *testing.T) {
	tests := []struct {
		elemType     ElementType
		translatable bool
	}{
		{ElementText, true},
		{ElementTitle, true},
		{ElementCaption, true},
		{ElementFormula, false},
		{ElementPicture, false},
		{ElementTable, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.elemType), func(t *testing.T) {
			assert.Equal(t, tt.translatable, tt.elemType.IsTranslatable())
		})
	}
}

func TestElementType_Priority(t *testing.T) {
	// Title should have higher priority than text
	assert.Greater(t, ElementTitle.Priority(), ElementText.Priority())

	// Section header should have higher priority than caption
	assert.Greater(t, ElementSectionHeader.Priority(), ElementCaption.Priority())

	// Formula should have lowest priority (0)
	assert.Equal(t, 0, ElementFormula.Priority())
}

func TestConvertToTextBlock(t *testing.T) {
	elem := LayoutElement{
		Type: ElementTitle,
		BoundingBox: BoundingBox{
			X:      10,
			Y:      20,
			Width:  100,
			Height: 30,
		},
		Confidence: 0.95,
		Page:       1,
		Content:    "Test Title",
	}

	block := elem.ConvertToTextBlock("block_1")

	assert.Equal(t, "block_1", block.ID)
	assert.Equal(t, 1, block.Page)
	assert.Equal(t, "Test Title", block.Text)
	assert.Equal(t, 10.0, block.X)
	assert.Equal(t, 20.0, block.Y)
	assert.Equal(t, 100.0, block.Width)
	assert.Equal(t, 30.0, block.Height)
	assert.Equal(t, "title", block.BlockType)
}
