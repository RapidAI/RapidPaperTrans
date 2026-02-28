//go:build cgo

// Package pdf provides ONNX-based layout detection using DocLayout-YOLO model.
// This file requires CGO and the onnxruntime shared library.
package pdf

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	"latex-translator/internal/logger"
	"latex-translator/internal/models"
)

// DocLayout-YOLO DocStructBench class name strings (10 classes)
var docLayoutClassStrings = []string{
	"title",           // 0
	"plain text",      // 1
	"abandon",         // 2
	"figure",          // 3
	"figure_caption",  // 4
	"table",           // 5
	"table_caption",   // 6
	"table_footnote",  // 7
	"isolate_formula", // 8
	"formula_caption", // 9
}

const (
	modelInputSize = 1024 // DocLayout-YOLO expects 1024x1024 input
	maxDetections  = 300  // YOLOv10 max detections
	defaultConfThreshold = 0.25
)

// ONNXEngine manages the ONNX runtime session for layout detection
type ONNXEngine struct {
	mu          sync.Mutex
	initialized bool
	modelPath   string
	session     *ort.DynamicAdvancedSession
}

var (
	onnxOnce   sync.Once
	onnxEngine *ONNXEngine
)

// GetONNXEngine returns the singleton ONNX engine instance
func GetONNXEngine() *ONNXEngine {
	onnxOnce.Do(func() {
		onnxEngine = &ONNXEngine{}
	})
	return onnxEngine
}

// getOnnxRuntimeLibPath returns the path to the onnxruntime shared library
func getOnnxRuntimeLibPath() string {
	if p := os.Getenv("ONNXRUNTIME_LIB"); p != "" {
		return p
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			"onnxruntime.dll",
			filepath.Join("lib", "onnxruntime.dll"),
		}
		if home := os.Getenv("USERPROFILE"); home != "" {
			candidates = append(candidates,
				filepath.Join(home, "onnxruntime", "onnxruntime.dll"),
				filepath.Join(home, ".onnxruntime", "onnxruntime.dll"),
			)
		}
	case "darwin":
		candidates = []string{
			"libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
		}
	default:
		candidates = []string{
			"libonnxruntime.so",
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/libonnxruntime.so",
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

// Initialize sets up the ONNX runtime and loads the model
func (e *ONNXEngine) Initialize(workDir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.initialized {
		return nil
	}

	modelDir := filepath.Join(workDir, "models")
	modelPath, err := models.EnsureModelExtracted(modelDir)
	if err != nil {
		return fmt.Errorf("failed to extract model: %w", err)
	}
	e.modelPath = modelPath
	logger.Info("model extracted", logger.String("path", modelPath))

	libPath := getOnnxRuntimeLibPath()
	ort.SetSharedLibraryPath(libPath)
	logger.Info("onnxruntime library", logger.String("path", libPath))

	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("failed to initialize onnxruntime: %w", err)
	}

	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("failed to get model info: %w", err)
	}

	for i, info := range inputs {
		logger.Info("model input",
			logger.Int("index", i),
			logger.String("name", info.Name),
			logger.String("shape", info.Dimensions.String()))
	}
	for i, info := range outputs {
		logger.Info("model output",
			logger.Int("index", i),
			logger.String("name", info.Name),
			logger.String("shape", info.Dimensions.String()))
	}

	inputNames := make([]string, len(inputs))
	outputNames := make([]string, len(outputs))
	for i, info := range inputs {
		inputNames[i] = info.Name
	}
	for i, info := range outputs {
		outputNames[i] = info.Name
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("failed to create session options: %w", err)
	}
	defer opts.Destroy()
	opts.SetIntraOpNumThreads(4)

	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, opts)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("failed to create session: %w", err)
	}

	e.session = session
	e.initialized = true
	logger.Info("ONNX engine initialized successfully")
	return nil
}

// Destroy cleans up the ONNX runtime resources
func (e *ONNXEngine) Destroy() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	if e.initialized {
		ort.DestroyEnvironment()
		e.initialized = false
	}
}

// IsInitialized returns whether the engine is ready
func (e *ONNXEngine) IsInitialized() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initialized
}

// DetectLayoutFromImage runs DocLayout-YOLO inference on an image.
// Returns detections using the Detection type from postprocessing.go.
func (e *ONNXEngine) DetectLayoutFromImage(img image.Image, origW, origH int) ([]Detection, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.initialized || e.session == nil {
		return nil, fmt.Errorf("ONNX engine not initialized")
	}

	inputData, scaleX, scaleY, padX, padY := preprocessLetterbox(img, modelInputSize)

	inputShape := ort.NewShape(1, 3, int64(modelInputSize), int64(modelInputSize))
	inputTensor, err := ort.NewTensor(inputShape, inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	outputShape := ort.NewShape(1, int64(maxDetections), 6)
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	inputs := []ort.Value{inputTensor}
	outputs := []ort.Value{outputTensor}
	if err := e.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	outputData := outputTensor.GetData()
	classNames := getDocLayoutClassNames()
	var detections []Detection

	for i := 0; i < maxDetections; i++ {
		offset := i * 6
		if offset+5 >= len(outputData) {
			break
		}

		x1 := float64(outputData[offset+0])
		y1 := float64(outputData[offset+1])
		x2 := float64(outputData[offset+2])
		y2 := float64(outputData[offset+3])
		conf := float64(outputData[offset+4])
		classID := int(outputData[offset+5])

		if conf < defaultConfThreshold {
			continue
		}
		if classID < 0 || classID >= len(classNames) {
			continue
		}

		// Convert from letterbox coordinates back to original image coordinates
		x1 = (x1 - padX) / scaleX
		y1 = (y1 - padY) / scaleY
		x2 = (x2 - padX) / scaleX
		y2 = (y2 - padY) / scaleY

		x1 = math.Max(0, math.Min(x1, float64(origW)))
		y1 = math.Max(0, math.Min(y1, float64(origH)))
		x2 = math.Max(0, math.Min(x2, float64(origW)))
		y2 = math.Max(0, math.Min(y2, float64(origH)))

		w := x2 - x1
		h := y2 - y1
		if w < 1 || h < 1 {
			continue
		}

		detections = append(detections, Detection{
			Box: BoundingBox{
				X: x1, Y: y1, Width: w, Height: h,
			},
			ClassID:    classID,
			Confidence: conf,
		})
	}

	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Confidence > detections[j].Confidence
	})

	return detections, nil
}

// preprocessLetterbox resizes image to target size with letterbox padding (YOLO convention).
func preprocessLetterbox(img image.Image, targetSize int) ([]float32, float64, float64, float64, float64) {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scale := math.Min(float64(targetSize)/float64(srcW), float64(targetSize)/float64(srcH))
	newW := int(math.Round(float64(srcW) * scale))
	newH := int(math.Round(float64(srcH) * scale))

	padX := float64(targetSize-newW) / 2.0
	padY := float64(targetSize-newH) / 2.0

	padded := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	gray := color.RGBA{114, 114, 114, 255}
	draw.Draw(padded, padded.Bounds(), &image.Uniform{gray}, image.Point{}, draw.Src)

	offsetX := int(padX)
	offsetY := int(padY)
	for y := 0; y < newH; y++ {
		srcY := y * srcH / newH
		for x := 0; x < newW; x++ {
			srcX := x * srcW / newW
			c := img.At(srcX+bounds.Min.X, srcY+bounds.Min.Y)
			padded.Set(x+offsetX, y+offsetY, c)
		}
	}

	data := make([]float32, 3*targetSize*targetSize)
	for y := 0; y < targetSize; y++ {
		for x := 0; x < targetSize; x++ {
			r, g, b, _ := padded.At(x, y).RGBA()
			idx := y*targetSize + x
			data[idx] = float32(r>>8) / 255.0
			data[targetSize*targetSize+idx] = float32(g>>8) / 255.0
			data[2*targetSize*targetSize+idx] = float32(b>>8) / 255.0
		}
	}

	return data, scale, scale, padX, padY
}
