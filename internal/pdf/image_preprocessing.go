// Package pdf provides image preprocessing for AI model input
package pdf

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"latex-translator/internal/logger"
)

// ImagePreprocessor handles image preprocessing for ONNX model
type ImagePreprocessor struct {
	targetSize int
	mean       []float32
	std        []float32
}

// NewImagePreprocessor creates a new preprocessor
func NewImagePreprocessor(targetSize int) *ImagePreprocessor {
	return &ImagePreprocessor{
		targetSize: targetSize,
		// ImageNet normalization values
		mean: []float32{0.485, 0.456, 0.406},
		std:  []float32{0.229, 0.224, 0.225},
	}
}

// Preprocess converts an image to model input format
// Returns: float32 array in CHW format, shape [1, 3, H, W]
func (p *ImagePreprocessor) Preprocess(img image.Image) ([]float32, []int64, error) {
	logger.Debug("preprocessing image",
		logger.Int("originalWidth", img.Bounds().Dx()),
		logger.Int("originalHeight", img.Bounds().Dy()),
		logger.Int("targetSize", p.targetSize))

	// 1. Resize image to target size
	resized := p.resizeImage(img, p.targetSize, p.targetSize)

	// 2. Convert to float32 array in CHW format
	data, err := p.imageToTensor(resized)
	if err != nil {
		return nil, nil, err
	}

	// 3. Create shape [batch, channels, height, width]
	shape := []int64{1, 3, int64(p.targetSize), int64(p.targetSize)}

	logger.Debug("preprocessing complete",
		logger.Int("tensorSize", len(data)),
		logger.String("shape", fmt.Sprintf("%v", shape)))

	return data, shape, nil
}

// resizeImage resizes an image to target dimensions
func (p *ImagePreprocessor) resizeImage(img image.Image, width, height int) image.Image {
	// Create new RGBA image
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Simple nearest-neighbor scaling
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Map destination coordinates to source coordinates
			srcX := x * srcWidth / width
			srcY := y * srcHeight / height

			// Get source pixel
			c := img.At(srcX+bounds.Min.X, srcY+bounds.Min.Y)
			dst.Set(x, y, c)
		}
	}

	return dst
}

// imageToTensor converts image to float32 tensor in CHW format
func (p *ImagePreprocessor) imageToTensor(img image.Image) ([]float32, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Allocate tensor: [C, H, W]
	data := make([]float32, 3*height*width)

	// Convert image to tensor
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Get pixel color
			r, g, b, _ := img.At(x, y).RGBA()

			// Convert to [0, 1] range
			rNorm := float32(r>>8) / 255.0
			gNorm := float32(g>>8) / 255.0
			bNorm := float32(b>>8) / 255.0

			// Apply normalization (mean and std)
			rNorm = (rNorm - p.mean[0]) / p.std[0]
			gNorm = (gNorm - p.mean[1]) / p.std[1]
			bNorm = (bNorm - p.mean[2]) / p.std[2]

			// Store in CHW format
			idx := y*width + x
			data[idx] = rNorm                    // R channel
			data[height*width+idx] = gNorm       // G channel
			data[2*height*width+idx] = bNorm     // B channel
		}
	}

	return data, nil
}

// PreprocessBatch preprocesses multiple images in batch
func (p *ImagePreprocessor) PreprocessBatch(images []image.Image) ([]float32, []int64, error) {
	if len(images) == 0 {
		return nil, nil, fmt.Errorf("no images to preprocess")
	}

	batchSize := len(images)
	singleSize := 3 * p.targetSize * p.targetSize
	data := make([]float32, batchSize*singleSize)

	for i, img := range images {
		imgData, _, err := p.Preprocess(img)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to preprocess image %d: %w", i, err)
		}

		// Copy to batch tensor
		copy(data[i*singleSize:(i+1)*singleSize], imgData)
	}

	shape := []int64{int64(batchSize), 3, int64(p.targetSize), int64(p.targetSize)}
	return data, shape, nil
}

// NormalizeCoordinates normalizes bounding box coordinates from pixel space to [0, 1]
func NormalizeCoordinates(box BoundingBox, imgWidth, imgHeight int) BoundingBox {
	return BoundingBox{
		X:      box.X / float64(imgWidth),
		Y:      box.Y / float64(imgHeight),
		Width:  box.Width / float64(imgWidth),
		Height: box.Height / float64(imgHeight),
	}
}

// DenormalizeCoordinates converts normalized coordinates back to pixel space
func DenormalizeCoordinates(box BoundingBox, imgWidth, imgHeight int) BoundingBox {
	return BoundingBox{
		X:      box.X * float64(imgWidth),
		Y:      box.Y * float64(imgHeight),
		Width:  box.Width * float64(imgWidth),
		Height: box.Height * float64(imgHeight),
	}
}

// PadImage pads an image to square with given size
func PadImage(img image.Image, targetSize int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// If already square and correct size, return as is
	if width == targetSize && height == targetSize {
		return img
	}

	// Create new square image with padding
	padded := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))

	// Fill with gray background
	gray := color.RGBA{128, 128, 128, 255}
	draw.Draw(padded, padded.Bounds(), &image.Uniform{gray}, image.Point{}, draw.Src)

	// Calculate padding to center the image
	offsetX := (targetSize - width) / 2
	offsetY := (targetSize - height) / 2

	// Draw original image in center
	draw.Draw(padded, image.Rect(offsetX, offsetY, offsetX+width, offsetY+height),
		img, bounds.Min, draw.Src)

	return padded
}
