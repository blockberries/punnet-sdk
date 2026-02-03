package crypto

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCrossImplementationValidation runs external validation scripts to verify
// our cryptographic implementations against independent libraries.
//
// This test is skipped by default and can be enabled with:
//
//	go test -tags=crossimpl ./crypto/...
//
// Or run directly with:
//
//	go test -run TestCrossImplementationValidation ./crypto/...
//
// Requirements:
//   - Python 3.8+ with 'cryptography' library installed
//   - pip install cryptography
func TestCrossImplementationValidation(t *testing.T) {
	// Find repository root
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get current file path")
	}
	repoRoot := filepath.Dir(filepath.Dir(filename))

	// Check for Python
	pythonCmd := findPython()
	if pythonCmd == "" {
		t.Skip("Python not found - skipping cross-implementation validation")
	}

	// Check for cryptography library
	if !hasPythonCryptography(pythonCmd) {
		t.Skip("Python cryptography library not installed - pip install cryptography")
	}

	// Check for vectors file
	vectorsFile := filepath.Join(repoRoot, "testdata", "signing_vectors.json")
	if _, err := os.Stat(vectorsFile); os.IsNotExist(err) {
		t.Skipf("Vectors file not found: %s", vectorsFile)
	}

	// Run Python validation script
	scriptPath := filepath.Join(repoRoot, "scripts", "validate_rfc6979.py")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Skipf("Validation script not found: %s", scriptPath)
	}

	t.Run("Python_RFC6979_Validation", func(t *testing.T) {
		cmd := exec.Command(pythonCmd, scriptPath, "--vectors-file", vectorsFile)
		output, err := cmd.CombinedOutput()

		t.Logf("Python validation output:\n%s", string(output))

		if err != nil {
			// Check if it's an exit code error
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Errorf("Python validation failed with exit code %d", exitErr.ExitCode())
			} else {
				t.Errorf("Python validation failed: %v", err)
			}
		}
	})
}

// TestCrossImplementationValidationWithResults runs validation and parses results.
func TestCrossImplementationValidationWithResults(t *testing.T) {
	// Find repository root
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get current file path")
	}
	repoRoot := filepath.Dir(filepath.Dir(filename))

	pythonCmd := findPython()
	if pythonCmd == "" {
		t.Skip("Python not found")
	}

	if !hasPythonCryptography(pythonCmd) {
		t.Skip("Python cryptography library not installed")
	}

	vectorsFile := filepath.Join(repoRoot, "testdata", "signing_vectors.json")
	scriptPath := filepath.Join(repoRoot, "scripts", "validate_rfc6979.py")

	if _, err := os.Stat(vectorsFile); os.IsNotExist(err) {
		t.Skipf("Vectors file not found: %s", vectorsFile)
	}
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Skipf("Script not found: %s", scriptPath)
	}

	// Create temp file for JSON results
	tmpFile, err := os.CreateTemp("", "validation_results_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cmd := exec.Command(pythonCmd, scriptPath,
		"--vectors-file", vectorsFile,
		"--json-output", tmpFile.Name())
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Validation output:\n%s", string(output))
		t.Fatalf("Validation failed: %v", err)
	}

	// Parse results
	resultData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read results: %v", err)
	}

	var results struct {
		Passed  []string `json:"passed"`
		Failed  []string `json:"failed"`
		Skipped []string `json:"skipped"`
	}

	if err := json.Unmarshal(resultData, &results); err != nil {
		t.Fatalf("Failed to parse results: %v", err)
	}

	t.Logf("Validation Results:")
	t.Logf("  Passed:  %d", len(results.Passed))
	t.Logf("  Failed:  %d", len(results.Failed))
	t.Logf("  Skipped: %d", len(results.Skipped))

	if len(results.Failed) > 0 {
		t.Errorf("Failed validations: %v", results.Failed)
	}

	// Log passed for visibility
	for _, p := range results.Passed {
		t.Logf("  PASS: %s", p)
	}
}

// findPython finds a suitable Python interpreter.
// It checks for a virtual environment in the repo first, then falls back to system Python.
func findPython() string {
	// Find repository root
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		repoRoot := filepath.Dir(filepath.Dir(filename))

		// Check for venv in repo (created by scripts/cross_impl_validate.sh or manually)
		venvPython := filepath.Join(repoRoot, ".venv", "bin", "python")
		if _, err := os.Stat(venvPython); err == nil {
			return venvPython
		}
		venvPython3 := filepath.Join(repoRoot, ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython3); err == nil {
			return venvPython3
		}
	}

	// Try python3 first
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	// Fall back to python
	if path, err := exec.LookPath("python"); err == nil {
		// Verify it's Python 3
		cmd := exec.Command(path, "--version")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "Python 3") {
			return path
		}
	}
	return ""
}

// hasPythonCryptography checks if the cryptography library is installed.
func hasPythonCryptography(pythonCmd string) bool {
	cmd := exec.Command(pythonCmd, "-c", "import cryptography")
	return cmd.Run() == nil
}
