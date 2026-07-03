package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// WizardPreset represents a preconfigured workspace setup.
type WizardPreset string

const (
	PresetSolo      WizardPreset = "solo"
	PresetSmallTeam WizardPreset = "small-team"
	PresetFullTeam  WizardPreset = "full-team"
	PresetCustom    WizardPreset = "custom"
)

// WizardState tracks the wizard's progress and collected data.
type WizardState struct {
	Dir       string
	Nickname  string
	Tool      string
	Preset    WizardPreset
	Channels  []string
	Step      int
	TotalStep int
}

// NewWizardState creates a new wizard state for the given directory.
func NewWizardState(dir string) *WizardState {
	return &WizardState{
		Step:      1,
		TotalStep: 3,
		Dir:       dir,
		Nickname:  "@bc",
		Preset:    PresetSolo,
		Tool:      "claude",
		Channels:  []string{"general", "eng"},
	}
}

// RunWizard runs the interactive workspace initialization wizard.
func RunWizard(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}

	// Check for existing workspace
	if isV2Workspace(absDir) {
		return fmt.Errorf("workspace already initialized in %s", absDir)
	}

	state := NewWizardState(absDir)
	reader := bufio.NewReader(os.Stdin)

	// Print welcome banner
	printWelcome()

	// Step 1: Workspace basics
	if err := wizardStepBasics(state, reader); err != nil {
		return err
	}

	// Step 2: Tool selection
	if err := wizardStepTools(state, reader); err != nil {
		return err
	}

	// Step 3: Confirmation
	if err := wizardStepConfirm(state, reader); err != nil {
		return err
	}

	// Create workspace with wizard settings
	return createWorkspaceFromWizard(state)
}

// printWelcome prints the wizard welcome banner.
func printWelcome() {
	fmt.Println()
	fmt.Println("  " + ui.CyanText("Welcome to mycel!"))
	fmt.Println("  " + ui.GrayText("AI Agent Orchestration CLI"))
	fmt.Println()
}

// printStepHeader prints the current step header.
func printStepHeader(step, total int, title string) {
	fmt.Printf("  [%d/%d] %s\n", step, total, ui.BoldText(title))
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()
}

// wizardStepBasics handles step 1: workspace basics (nickname).
func wizardStepBasics(state *WizardState, reader *bufio.Reader) error {
	printStepHeader(1, state.TotalStep, "Workspace Setup")

	// Prompt for nickname
	fmt.Printf("  Your nickname [%s]: ", "@bc")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input != "" {
		nickname, err := workspace.NormalizeNickname(input)
		if err != nil {
			fmt.Printf("  %s Using default: %s\n", ui.YellowText("!"), "@bc")
		} else {
			state.Nickname = nickname
			if !strings.HasPrefix(input, "@") {
				fmt.Printf("  %s Auto-corrected to %s\n", ui.GreenText("✓"), nickname)
			}
		}
	}

	fmt.Println()
	return nil
}

// wizardStepTools handles step 2: tool selection.
func wizardStepTools(state *WizardState, reader *bufio.Reader) error {
	printStepHeader(2, state.TotalStep, "AI Provider Selection")

	fmt.Println("  Select your default AI provider:")
	fmt.Println()
	fmt.Printf("    %s Claude %s\n", ui.CyanText("[1]"), ui.GrayText("(Anthropic Claude Code)"))
	fmt.Printf("    %s Gemini %s\n", ui.CyanText("[2]"), ui.GrayText("(Google Gemini CLI)"))
	fmt.Printf("    %s Cursor %s\n", ui.CyanText("[3]"), ui.GrayText("(Cursor AI Editor)"))
	fmt.Println()

	fmt.Print("  Enter choice [1]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	choice := strings.TrimSpace(input)
	switch choice {
	case "", "1":
		state.Tool = "claude"
		fmt.Printf("  %s Claude selected\n", ui.GreenText("✓"))
	case "2":
		state.Tool = "gemini"
		fmt.Printf("  %s Gemini selected\n", ui.GreenText("✓"))
	case "3":
		state.Tool = "cursor"
		fmt.Printf("  %s Cursor selected\n", ui.GreenText("✓"))
	default:
		fmt.Printf("  %s Invalid choice, using Claude\n", ui.YellowText("!"))
		state.Tool = "claude"
	}

	fmt.Println()
	return nil
}

// wizardStepConfirm handles the final confirmation step.
func wizardStepConfirm(state *WizardState, reader *bufio.Reader) error {
	printStepHeader(3, state.TotalStep, "Confirmation")

	fmt.Println("  Review your configuration:")
	fmt.Println()
	fmt.Printf("    Directory:  %s\n", state.Dir)
	fmt.Printf("    Nickname:   %s\n", state.Nickname)
	fmt.Printf("    AI Provider: %s\n", state.Tool)
	fmt.Println()

	fmt.Print("  Proceed? [Y/n]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	choice := strings.ToLower(strings.TrimSpace(input))
	if choice == "n" || choice == "no" {
		return fmt.Errorf("initialization canceled")
	}

	fmt.Println()
	return nil
}

// createWorkspaceFromWizard creates a workspace using wizard settings.
func createWorkspaceFromWizard(state *WizardState) error {
	stateDir := filepath.Join(state.Dir, ".bc")

	// Create state directory
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return fmt.Errorf("failed to create .bc directory: %w", err)
	}

	// Create agents directory
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	// Create config
	name := filepath.Base(state.Dir)
	cfg := workspace.DefaultConfig()
	cfg.User.Name = state.Nickname
	cfg.Providers.Default = state.Tool

	configPath, err := workspace.ConfigPath(state.Dir)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	if err = cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Create roles directory and default root.md
	roleMgr := workspace.NewRoleManager(stateDir)
	if _, err = roleMgr.EnsureDefaultRoot(); err != nil {
		return fmt.Errorf("failed to create role files: %w", err)
	}

	// Register in global registry
	reg, regErr := workspace.LoadRegistry()
	if regErr == nil {
		reg.Register(state.Dir, name)
		_ = reg.Save()
	}

	// Bootstrap server daemons (non-fatal; warns if Docker unavailable)
	bootstrapServerDaemons(state.Dir)

	// Print success
	printWizardSuccess(state)
	return nil
}

// printWizardSuccess prints the success message after wizard completion.
func printWizardSuccess(state *WizardState) {
	fmt.Println("  " + ui.GreenText("✓") + " Workspace initialized!")
	fmt.Println()
	fmt.Println("  Created:")
	fmt.Println("    preferences.json    # Workspace configuration")
	fmt.Println("    agents/             # Agent state directory")
	fmt.Println("    roles/              # Role definitions")
	fmt.Println("    roles/root.md       # Root agent role")
	fmt.Println("    bc.db               # Workspace database")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    mycel       # Open the dashboard")
	fmt.Println("    mycel up    # Start agents")
	fmt.Println("    mycel status # Check agent status")
	fmt.Println()
}

// InitWithPreset initializes a workspace with a specific preset (non-interactive).
func InitWithPreset(dir string, preset WizardPreset) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}

	log.Debug("init with preset", "dir", absDir, "preset", preset)

	// Check for existing workspace
	if isV2Workspace(absDir) {
		return fmt.Errorf("workspace already initialized in %s", absDir)
	}

	state := NewWizardState(absDir)
	state.Preset = preset

	return createWorkspaceFromWizard(state)
}
