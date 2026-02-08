package python

import (
	"path/filepath"
	"sync"

	"latex-translator/internal/models"
)

var (
	globalEnv     *Env
	globalEnvOnce sync.Once
	globalEnvErr  error
)

// RequiredPackages lists the Python packages required for PDF translation
var RequiredPackages = []string{
	"pymupdf",       // For PDF manipulation
	"requests",      // For HTTP requests to translation API
	"onnxruntime",   // For ONNX model inference (layout detection)
	"opencv-python", // For image preprocessing
	"numpy",         // For array operations
}

// GetGlobalEnv returns the global Python environment instance.
// It initializes the environment on first call.
func GetGlobalEnv() (*Env, error) {
	globalEnvOnce.Do(func() {
		globalEnv, globalEnvErr = New(Config{})
	})
	return globalEnv, globalEnvErr
}

// EnsureGlobalEnv ensures the global Python environment is set up with required packages.
// This is the main entry point for other packages to use.
func EnsureGlobalEnv(progressFn func(message string)) error {
	env, err := GetGlobalEnv()
	if err != nil {
		return err
	}

	// Ensure environment is set up
	if err := env.EnsureSetup(progressFn); err != nil {
		return err
	}

	// Ensure required packages are installed
	if err := env.EnsurePackages(RequiredPackages, progressFn); err != nil {
		return err
	}

	// Ensure model is extracted
	if progressFn != nil {
		progressFn("Extracting layout detection model...")
	}
	modelsDir := filepath.Join(env.BaseDir, "models")
	if _, err := models.EnsureModelExtracted(modelsDir); err != nil {
		return err
	}

	return nil
}

// GetPythonPath returns the path to the Python executable.
// Returns empty string if environment is not set up.
func GetPythonPath() string {
	env, err := GetGlobalEnv()
	if err != nil || env == nil {
		return ""
	}
	return env.GetPythonPath()
}

// GetModelPath returns the path to the DocLayout-YOLO model.
// Returns empty string if environment is not set up.
func GetModelPath() string {
	env, err := GetGlobalEnv()
	if err != nil || env == nil {
		return ""
	}
	return filepath.Join(env.BaseDir, "models", models.ModelFileName)
}

// RunScript runs a Python script using the global environment.
func RunScript(scriptPath string, args ...string) (string, error) {
	env, err := GetGlobalEnv()
	if err != nil {
		return "", err
	}
	return env.RunScript(scriptPath, args...)
}
