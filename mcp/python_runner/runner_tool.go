package python_runner

import (
	"os"
	"os/exec"
)

func runPythonCode(code string) (string, error) {
	// Create a temporary file to store the Python code
	tmpFile, err := os.CreateTemp("", "temp_code_*.py")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file after execution

	// Write the code to the temporary file
	if _, err := tmpFile.WriteString(code); err != nil {
		return "", err
	}

	// Close the file to ensure all data is written
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	// Execute the Python code using the Python interpreter
	cmd := exec.Command("python3", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), nil
}
