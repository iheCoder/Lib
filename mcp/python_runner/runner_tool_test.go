package main

import (
	"testing"
)

func TestRunPythonCode(t *testing.T) {
	// normal case
	code := `print("Hello, World!")`
	result, err := runPythonCode(code)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "Hello, World!\n" {
		t.Fatalf("Expected 'Hello, World!', got %s", result)
	}

	// error case
	code = `print("Hello, World!"`
	result, err = runPythonCode(code)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	t.Logf("Expected error: %v", err)
}
