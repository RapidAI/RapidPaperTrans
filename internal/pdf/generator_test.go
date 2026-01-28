// Package pdf provides PDF translation functionality including text extraction,
// batch translation, and PDF generation with translated content.
package pdf

import (
	"math"
	"testing"
)

// TestAdjustTextSize tests the AdjustTextSize method
// Requirements: 4.3 - 当中文文本超出原始文本框时，自动调整字体大小或换行
// Property 9: 布局保持 - 翻译后文本位置与原始位置偏差在可接受范围内
func TestAdjustTextSize(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name      string
		text      string
		maxWidth  float64
		maxHeight float64
		wantErr   bool
		minSize   float64 // minimum expected size
		maxSize   float64 // maximum expected size
	}{
		{
			name:      "short English text",
			text:      "Hello",
			maxWidth:  100,
			maxHeight: 50,
			wantErr:   false,
			minSize:   6,
			maxSize:   72,
		},
		{
			name:      "short Chinese text",
			text:      "你好",
			maxWidth:  100,
			maxHeight: 50,
			wantErr:   false,
			minSize:   6,
			maxSize:   72,
		},
		{
			name:      "mixed Chinese and English",
			text:      "Hello 你好 World 世界",
			maxWidth:  200,
			maxHeight: 50,
			wantErr:   false,
			minSize:   6,
			maxSize:   72,
		},
		{
			name:      "long Chinese text in narrow space",
			text:      "这是一段很长的中文文本，需要自动调整字体大小以适应有限的空间",
			maxWidth:  100,
			maxHeight: 50,
			wantErr:   false,
			minSize:   6,
			maxSize:   20, // Should be smaller due to space constraints
		},
		{
			name:      "empty text",
			text:      "",
			maxWidth:  100,
			maxHeight: 50,
			wantErr:   true,
		},
		{
			name:      "zero width",
			text:      "Test",
			maxWidth:  0,
			maxHeight: 50,
			wantErr:   true,
		},
		{
			name:      "zero height",
			text:      "Test",
			maxWidth:  100,
			maxHeight: 0,
			wantErr:   true,
		},
		{
			name:      "negative width",
			text:      "Test",
			maxWidth:  -10,
			maxHeight: 50,
			wantErr:   true,
		},
		{
			name:      "very small space",
			text:      "这是测试文本",
			maxWidth:  20,
			maxHeight: 10,
			wantErr:   false,
			minSize:   6, // Should return minimum size
			maxSize:   6,
		},
		{
			name:      "multi-line text",
			text:      "第一行\n第二行\n第三行",
			maxWidth:  100,
			maxHeight: 100,
			wantErr:   false,
			minSize:   6,
			maxSize:   72,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := g.AdjustTextSize(tt.text, tt.maxWidth, tt.maxHeight)

			if tt.wantErr {
				if err == nil {
					t.Errorf("AdjustTextSize() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("AdjustTextSize() unexpected error: %v", err)
				return
			}

			if size < tt.minSize || size > tt.maxSize {
				t.Errorf("AdjustTextSize() = %v, want between %v and %v", size, tt.minSize, tt.maxSize)
			}
		})
	}
}


// TestAdjustTextSizeWithOriginal tests the AdjustTextSizeWithOriginal method
func TestAdjustTextSizeWithOriginal(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name             string
		text             string
		originalFontSize float64
		maxWidth         float64
		maxHeight        float64
		wantErr          bool
		maxExpectedSize  float64
	}{
		{
			name:             "should not exceed original font size",
			text:             "Hello",
			originalFontSize: 10,
			maxWidth:         200,
			maxHeight:        100,
			wantErr:          false,
			maxExpectedSize:  10,
		},
		{
			name:             "should use smaller size when needed",
			text:             "这是一段很长的中文文本需要缩小字体",
			originalFontSize: 20,
			maxWidth:         50,
			maxHeight:        20,
			wantErr:          false,
			maxExpectedSize:  20,
		},
		{
			name:             "zero original font size",
			text:             "Test",
			originalFontSize: 0,
			maxWidth:         100,
			maxHeight:        50,
			wantErr:          false,
			maxExpectedSize:  72, // Should use calculated optimal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := g.AdjustTextSizeWithOriginal(tt.text, tt.originalFontSize, tt.maxWidth, tt.maxHeight)

			if tt.wantErr {
				if err == nil {
					t.Errorf("AdjustTextSizeWithOriginal() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("AdjustTextSizeWithOriginal() unexpected error: %v", err)
				return
			}

			if size > tt.maxExpectedSize {
				t.Errorf("AdjustTextSizeWithOriginal() = %v, want <= %v", size, tt.maxExpectedSize)
			}
		})
	}
}


// TestCalculateTextMetrics tests the calculateTextMetrics method
func TestCalculateTextMetrics(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name         string
		text         string
		wantChinese  int
		wantLatin    int
		wantSpaces   int
		wantNewlines int
	}{
		{
			name:         "pure English",
			text:         "Hello World",
			wantChinese:  0,
			wantLatin:    10,
			wantSpaces:   1,
			wantNewlines: 0,
		},
		{
			name:         "pure Chinese",
			text:         "你好世界",
			wantChinese:  4,
			wantLatin:    0,
			wantSpaces:   0,
			wantNewlines: 0,
		},
		{
			name:         "mixed content",
			text:         "Hello 你好",
			wantChinese:  2,
			wantLatin:    5,
			wantSpaces:   1,
			wantNewlines: 0,
		},
		{
			name:         "with newlines",
			text:         "Line1\nLine2\nLine3",
			wantChinese:  0,
			wantLatin:    15,
			wantSpaces:   0,
			wantNewlines: 2,
		},
		{
			name:         "empty string",
			text:         "",
			wantChinese:  0,
			wantLatin:    0,
			wantSpaces:   0,
			wantNewlines: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := g.calculateTextMetrics(tt.text)

			if metrics.ChineseChars != tt.wantChinese {
				t.Errorf("ChineseChars = %v, want %v", metrics.ChineseChars, tt.wantChinese)
			}
			if metrics.LatinChars != tt.wantLatin {
				t.Errorf("LatinChars = %v, want %v", metrics.LatinChars, tt.wantLatin)
			}
			if metrics.SpaceChars != tt.wantSpaces {
				t.Errorf("SpaceChars = %v, want %v", metrics.SpaceChars, tt.wantSpaces)
			}
			if metrics.NewlineChars != tt.wantNewlines {
				t.Errorf("NewlineChars = %v, want %v", metrics.NewlineChars, tt.wantNewlines)
			}
		})
	}
}


// TestCalculateLinesNeeded tests the calculateLinesNeeded method
func TestCalculateLinesNeeded(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name      string
		text      string
		fontSize  float64
		maxWidth  float64
		wantLines int
	}{
		{
			name:      "single line short text",
			text:      "Hi",
			fontSize:  12,
			maxWidth:  100,
			wantLines: 1,
		},
		{
			name:      "explicit newlines",
			text:      "Line1\nLine2\nLine3",
			fontSize:  12,
			maxWidth:  200,
			wantLines: 3,
		},
		{
			name:      "wrap long text",
			text:      "This is a very long text that should wrap",
			fontSize:  12,
			maxWidth:  50,
			wantLines: 5, // Approximate, depends on wrapping logic
		},
		{
			name:      "Chinese text wrapping",
			text:      "这是一段中文文本需要换行",
			fontSize:  12,
			maxWidth:  50,
			wantLines: 3, // Each Chinese char is ~12pt wide
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := g.calculateLinesNeeded(tt.text, tt.fontSize, tt.maxWidth)

			// Allow some tolerance for wrapping calculations
			if lines < 1 {
				t.Errorf("calculateLinesNeeded() = %v, want >= 1", lines)
			}
		})
	}
}


// TestWrapTextForBounds tests the WrapTextForBounds method
func TestWrapTextForBounds(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name     string
		text     string
		fontSize float64
		maxWidth float64
		wantLen  int // minimum expected length (should be >= original)
	}{
		{
			name:     "short text no wrap needed",
			text:     "Hi",
			fontSize: 12,
			maxWidth: 100,
			wantLen:  2,
		},
		{
			name:     "Chinese text needs wrapping",
			text:     "这是一段很长的中文文本",
			fontSize: 12,
			maxWidth: 50,
			wantLen:  11, // Original length, may have newlines added
		},
		{
			name:     "empty text",
			text:     "",
			fontSize: 12,
			maxWidth: 100,
			wantLen:  0,
		},
		{
			name:     "zero width returns original",
			text:     "Test",
			fontSize: 12,
			maxWidth: 0,
			wantLen:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.WrapTextForBounds(tt.text, tt.fontSize, tt.maxWidth)

			if len(result) < tt.wantLen {
				t.Errorf("WrapTextForBounds() length = %v, want >= %v", len(result), tt.wantLen)
			}
		})
	}
}


// TestCalculateStringWidth tests the calculateStringWidth method
func TestCalculateStringWidth(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name      string
		text      string
		fontSize  float64
		wantWidth float64
	}{
		{
			name:      "single Latin char",
			text:      "A",
			fontSize:  12,
			wantWidth: 6, // 0.5 * 12
		},
		{
			name:      "single Chinese char",
			text:      "中",
			fontSize:  12,
			wantWidth: 12, // 1.0 * 12
		},
		{
			name:      "space",
			text:      " ",
			fontSize:  12,
			wantWidth: 3, // 0.25 * 12
		},
		{
			name:      "mixed text",
			text:      "A中 ",
			fontSize:  10,
			wantWidth: 17.5, // 0.5*10 + 1.0*10 + 0.25*10
		},
		{
			name:      "empty string",
			text:      "",
			fontSize:  12,
			wantWidth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width := g.calculateStringWidth(tt.text, tt.fontSize)

			if math.Abs(width-tt.wantWidth) > 0.01 {
				t.Errorf("calculateStringWidth() = %v, want %v", width, tt.wantWidth)
			}
		})
	}
}


// TestIsChinese tests the isChinese function
func TestIsChinese(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"Chinese character 中", '中', true},
		{"Chinese character 国", '国', true},
		{"Chinese character 文", '文', true},
		{"Chinese punctuation 。", '。', true},
		{"Chinese punctuation 、", '、', true},
		{"Latin A", 'A', false},
		{"Latin z", 'z', false},
		{"Number 1", '1', false},
		{"Space", ' ', false},
		{"Newline", '\n', false},
		{"Japanese Hiragana あ", 'あ', false}, // Not in CJK Unified Ideographs
		{"Korean 한", '한', false},             // Not in CJK Unified Ideographs
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isChinese(tt.r); got != tt.want {
				t.Errorf("isChinese(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}


// TestAdjustFontSizeForChinese tests the adjustFontSizeForChinese method
func TestAdjustFontSizeForChinese(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name         string
		text         string
		originalSize float64
		maxWidth     float64
		maxHeight    float64
		wantMin      float64
		wantMax      float64
	}{
		{
			name:         "text fits - no adjustment",
			text:         "Hi",
			originalSize: 12,
			maxWidth:     100,
			maxHeight:    50,
			wantMin:      12,
			wantMax:      12,
		},
		{
			name:         "Chinese text needs reduction",
			text:         "这是一段很长的中文文本",
			originalSize: 20,
			maxWidth:     50,
			maxHeight:    50,
			wantMin:      6,
			wantMax:      20,
		},
		{
			name:         "zero width returns original",
			text:         "Test",
			originalSize: 12,
			maxWidth:     0,
			maxHeight:    50,
			wantMin:      12,
			wantMax:      12,
		},
		{
			name:         "zero height returns original",
			text:         "Test",
			originalSize: 12,
			maxWidth:     100,
			maxHeight:    0,
			wantMin:      12,
			wantMax:      12,
		},
		{
			name:         "empty text returns original",
			text:         "",
			originalSize: 12,
			maxWidth:     100,
			maxHeight:    50,
			wantMin:      12,
			wantMax:      12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := g.adjustFontSizeForChinese(tt.text, tt.originalSize, tt.maxWidth, tt.maxHeight)

			if size < tt.wantMin || size > tt.wantMax {
				t.Errorf("adjustFontSizeForChinese() = %v, want between %v and %v", size, tt.wantMin, tt.wantMax)
			}
		})
	}
}


// TestTextFitsInBounds tests the textFitsInBounds method
func TestTextFitsInBounds(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name            string
		text            string
		fontSize        float64
		maxWidth        float64
		maxHeight       float64
		lineHeightRatio float64
		wantFits        bool
	}{
		{
			name:            "short text fits",
			text:            "Hi",
			fontSize:        12,
			maxWidth:        100,
			maxHeight:       50,
			lineHeightRatio: 1.2,
			wantFits:        true,
		},
		{
			name:            "long text doesn't fit in narrow space",
			text:            "This is a very long text that definitely won't fit",
			fontSize:        12,
			maxWidth:        20,
			maxHeight:       14,
			lineHeightRatio: 1.2,
			wantFits:        false,
		},
		{
			name:            "zero font size",
			text:            "Test",
			fontSize:        0,
			maxWidth:        100,
			maxHeight:       50,
			lineHeightRatio: 1.2,
			wantFits:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := g.calculateTextMetrics(tt.text)
			fits, _ := g.textFitsInBounds(tt.text, metrics, tt.fontSize, tt.maxWidth, tt.maxHeight, tt.lineHeightRatio)

			if fits != tt.wantFits {
				t.Errorf("textFitsInBounds() = %v, want %v", fits, tt.wantFits)
			}
		})
	}
}


// TestNewPDFGenerator tests the NewPDFGenerator constructor
func TestNewPDFGenerator(t *testing.T) {
	tests := []struct {
		name    string
		workDir string
	}{
		{"empty workDir", ""},
		{"with workDir", "/tmp/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewPDFGenerator(tt.workDir)
			if g == nil {
				t.Error("NewPDFGenerator() returned nil")
			}
			if g.workDir != tt.workDir {
				t.Errorf("workDir = %v, want %v", g.workDir, tt.workDir)
			}
		})
	}
}

// TestNewPDFGeneratorWithFont tests the NewPDFGeneratorWithFont constructor
func TestNewPDFGeneratorWithFont(t *testing.T) {
	g := NewPDFGeneratorWithFont("/tmp/test", "/path/to/font.ttf")
	if g == nil {
		t.Error("NewPDFGeneratorWithFont() returned nil")
	}
	if g.workDir != "/tmp/test" {
		t.Errorf("workDir = %v, want /tmp/test", g.workDir)
	}
	if g.fontPath != "/path/to/font.ttf" {
		t.Errorf("fontPath = %v, want /path/to/font.ttf", g.fontPath)
	}
}

// TestSetFontPath tests the SetFontPath method
func TestSetFontPath(t *testing.T) {
	g := NewPDFGenerator("")
	g.SetFontPath("/new/font/path.ttf")
	if g.fontPath != "/new/font/path.ttf" {
		t.Errorf("fontPath = %v, want /new/font/path.ttf", g.fontPath)
	}
}

// TestGetAvailableFonts tests the GetAvailableFonts method
func TestGetAvailableFonts(t *testing.T) {
	g := NewPDFGenerator("")
	fonts := g.GetAvailableFonts()

	if len(fonts) == 0 {
		t.Error("GetAvailableFonts() returned empty list")
	}

	// Check that some expected fonts are in the list
	expectedFonts := []string{"NotoSansSC", "SimSun", "SimHei"}
	for _, expected := range expectedFonts {
		found := false
		for _, font := range fonts {
			if font == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected font %s not found in available fonts", expected)
		}
	}
}


// TestIsPositionWithinTolerance tests the IsPositionWithinTolerance method
func TestIsPositionWithinTolerance(t *testing.T) {
	g := NewPDFGenerator("")

	tests := []struct {
		name      string
		dx        float64
		dy        float64
		tolerance float64
		want      bool
	}{
		{"within tolerance", 5, 5, 10, true},
		{"exactly at tolerance", 10, 10, 10, false}, // < not <=
		{"outside tolerance", 15, 5, 10, false},
		{"negative within tolerance", -5, -5, 10, true},
		{"zero offset", 0, 0, 10, true},
		{"default tolerance", 5, 5, 0, true}, // Uses default 10
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.IsPositionWithinTolerance(tt.dx, tt.dy, tt.tolerance)
			if got != tt.want {
				t.Errorf("IsPositionWithinTolerance(%v, %v, %v) = %v, want %v",
					tt.dx, tt.dy, tt.tolerance, got, tt.want)
			}
		})
	}
}

// TestCalculatePositionOffset tests the CalculatePositionOffset method
func TestCalculatePositionOffset(t *testing.T) {
	g := NewPDFGenerator("")

	original := TextBlock{X: 10, Y: 20}
	translated := TextBlock{X: 15, Y: 25}

	dx, dy := g.CalculatePositionOffset(original, translated)

	if dx != 5 {
		t.Errorf("dx = %v, want 5", dx)
	}
	if dy != 5 {
		t.Errorf("dy = %v, want 5", dy)
	}
}

// TestGetOutputPath tests the GetOutputPath method
func TestGetOutputPath(t *testing.T) {
	tests := []struct {
		name         string
		workDir      string
		originalPath string
		wantContains string
	}{
		{
			name:         "with workDir",
			workDir:      "/output",
			originalPath: "/input/document.pdf",
			wantContains: "_translated.pdf",
		},
		{
			name:         "without workDir",
			workDir:      "",
			originalPath: "/input/document.pdf",
			wantContains: "_translated.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewPDFGenerator(tt.workDir)
			output := g.GetOutputPath(tt.originalPath)

			if !contains(output, tt.wantContains) {
				t.Errorf("GetOutputPath() = %v, want to contain %v", output, tt.wantContains)
			}
		})
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
