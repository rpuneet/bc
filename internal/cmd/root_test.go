package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand(t *testing.T) {
	// Capture output
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "mycel") {
		t.Errorf("Expected output to contain 'mycel', got: %s", output)
	}
}

func TestVersionCommand(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2024-01-01")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1.0.0") {
		t.Errorf("Expected output to contain version '1.0.0', got: %s", output)
	}
	if !strings.Contains(output, "abc123") {
		t.Errorf("Expected output to contain commit 'abc123', got: %s", output)
	}
}

func TestRootReturnsCommand(t *testing.T) {
	cmd := Root()
	if cmd == nil {
		t.Fatal("Root() returned nil")
	}
	if cmd.Use != "mycel" {
		t.Errorf("Expected Use = 'mycel', got '%s'", cmd.Use)
	}
}
