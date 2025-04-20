package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"math/big"
	"os"
	"os/exec"
)

// Define the tool name
const (
	ToolPythonRunner = "python_runner"
)

var (
	secret = ""
)

func init() {
	// Initialize the secret
	updateSecret()
}

func updateSecret() {
	secret, _ = RandString(16)
	fmt.Println("Secret updated: ", secret)
}

func pythonRunnerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract the Python code from the request
	code := request.Params.Arguments["code"].(string)
	if code == "" {
		// If no code is provided, return an error
		return mcp.NewToolResultError("missing or invalid 'code' parameter"), nil
	}

	// Extract the run secret from the request
	reqSecret, ok := request.Params.Arguments["secret"].(string)
	if !ok || reqSecret == "" {
		// If no secret is provided, return an error
		return mcp.NewToolResultError("missing or invalid 'secret' parameter"), nil
	} else if reqSecret != secret {
		return mcp.NewToolResultError("please input the right secret to run the python code"), nil
	}

	// update the secret
	updateSecret()

	// Run the Python code
	output, err := runPythonCode(code)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Return the output as a result
	return mcp.NewToolResultText(output), nil
}

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

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func RandString(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}
