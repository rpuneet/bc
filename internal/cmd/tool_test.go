package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/ui"
)

func TestToolList_OutputFormat(t *testing.T) {
	// Capture UI output
	var buf bytes.Buffer
	ui.SetOutput(&buf)
	defer ui.SetOutput(nil) // reset to stdout

	output, err := executeCmd("tool", "list")
	if err != nil {
		t.Fatalf("tool list failed: %v", err)
	}

	// Table output goes through ui.Table.Print() -> ui.output
	tableOutput := buf.String()

	// Verify table headers are present
	if !strings.Contains(tableOutput, "TOOL") {
		t.Error("expected TOOL header in output")
	}
	if !strings.Contains(tableOutput, "STATUS") {
		t.Error("expected STATUS header in output")
	}
	// When bcd is running, shows ENABLED; when using provider registry, shows VERSION
	if !strings.Contains(tableOutput, "VERSION") && !strings.Contains(tableOutput, "ENABLED") {
		t.Error("expected VERSION or ENABLED header in output")
	}
	if !strings.Contains(tableOutput, "COMMAND") {
		t.Error("expected COMMAND header in output")
	}

	// Verify known providers appear
	if !strings.Contains(tableOutput, "claude") {
		t.Error("expected claude in tool list")
	}
	if !strings.Contains(tableOutput, "codex") {
		t.Error("expected codex in tool list")
	}

	// executeCmd output should be empty for non-JSON (all goes to ui output)
	_ = output
}

func TestToolCheck_KnownTool(t *testing.T) {
	// Capture UI output
	var buf bytes.Buffer
	ui.SetOutput(&buf)
	defer ui.SetOutput(nil)

	_, err := executeCmd("tool", "check", "claude")
	if err != nil {
		t.Fatalf("tool check claude failed: %v", err)
	}

	tableOutput := buf.String()

	if !strings.Contains(tableOutput, "claude") {
		t.Error("expected 'claude' in check output")
	}
	if !strings.Contains(tableOutput, "Tool:") {
		t.Error("expected 'Tool:' label in check output")
	}
	if !strings.Contains(tableOutput, "Status:") {
		t.Error("expected 'Status:' label in check output")
	}
	if !strings.Contains(tableOutput, "Command:") {
		t.Error("expected 'Command:' label in check output")
	}
}

func TestToolCheck_UnknownTool(t *testing.T) {
	_, err := executeCmd("tool", "check", "nonexistent-tool")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected 'unknown tool' error, got %q", err.Error())
	}
}

func TestToolListFlags(t *testing.T) {
	f := toolListCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("expected --json flag on tool list command")
	}
	if f.DefValue != "false" {
		t.Errorf("expected default value 'false', got %q", f.DefValue)
	}
}

func TestToolCheckArgs(t *testing.T) {
	_, err := executeCmd("tool", "check")
	if err == nil {
		t.Fatal("expected error when no tool name provided")
	}
}
