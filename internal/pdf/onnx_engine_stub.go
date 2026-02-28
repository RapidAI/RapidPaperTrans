//go:build !cgo

// Stub implementation when CGO is not available.
// ONNX inference requires CGO and the onnxruntime shared library.
package pdf

import (
	"fmt"
	"image"
	"sync"
)

// ONNXEngine stub - ONNX not available without CGO
type ONNXEngine struct {
	mu sync.Mutex
}

var (
	onnxOnce   sync.Once
	onnxEngine *ONNXEngine
)

func GetONNXEngine() *ONNXEngine {
	onnxOnce.Do(func() {
		onnxEngine = &ONNXEngine{}
	})
	return onnxEngine
}

func (e *ONNXEngine) Initialize(workDir string) error {
	return fmt.Errorf("ONNX runtime not available: build with CGO_ENABLED=1 and install a C compiler (MinGW-w64 on Windows)")
}

func (e *ONNXEngine) Destroy() {}

func (e *ONNXEngine) IsInitialized() bool { return false }

func (e *ONNXEngine) DetectLayoutFromImage(img image.Image, origW, origH int) ([]Detection, error) {
	return nil, fmt.Errorf("ONNX runtime not available: build with CGO_ENABLED=1")
}
