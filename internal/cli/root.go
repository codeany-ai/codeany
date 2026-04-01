package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/codeany-ai/codeany/internal/config"
	"github.com/codeany-ai/codeany/internal/pipe"
	"github.com/codeany-ai/codeany/internal/tui"
)

var (
	flagModel        string
	flagPipe         bool
	flagPrint        bool
	flagOutputFmt    string
	flagResume       bool
	flagPermission   string
	flagMaxTurns     int
	flagSystemPrompt string
	flagNoMCP        bool
	flagVerbose      bool
	flagCWD          string
	flagYes          bool
)

var (
	appVersion = "dev"
	appCommit  = "unknown"
	appDate    = "unknown"
)

// SetVersion is called from main to inject build-time version info
func SetVersion(version, commit, date string) {
	appVersion = version
	appCommit = commit
	appDate = date
}

var rootCmd = &cobra.Command{
	Use:   "codeany [prompt]",
	Short: "AI-powered terminal agent",
	Long:  `Codeany is an open-source AI-powered terminal agent for software engineering.`,
	Args:  cobra.ArbitraryArgs,
	RunE:  run,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Flags().StringVarP(&flagModel, "model", "m", "", "Model to use (e.g., sonnet-4-6, opus-4-6, haiku-4-5)")
	rootCmd.Flags().BoolVarP(&flagPipe, "pipe", "p", false, "Read from stdin, print response to stdout (non-interactive)")
	rootCmd.Flags().BoolVar(&flagPrint, "print", false, "Print response and exit (non-interactive)")
	rootCmd.Flags().StringVar(&flagOutputFmt, "output-format", "text", "Output format: text, json, stream-json")
	rootCmd.Flags().BoolVarP(&flagResume, "resume", "r", false, "Resume last conversation")
	rootCmd.Flags().StringVar(&flagPermission, "permission-mode", "", "Permission mode: default, acceptEdits, bypassPermissions, plan")
	rootCmd.Flags().IntVar(&flagMaxTurns, "max-turns", 0, "Maximum number of agentic turns")
	rootCmd.Flags().StringVar(&flagSystemPrompt, "system-prompt", "", "Custom system prompt")
	rootCmd.Flags().BoolVar(&flagNoMCP, "no-mcp", false, "Disable MCP servers")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	rootCmd.Flags().StringVar(&flagCWD, "cwd", "", "Working directory")
	rootCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Skip permission prompts (bypass all)")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(doctorCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		if appVersion == "dev" {
			fmt.Println("codeany dev (built from source)")
		} else {
			commit := appCommit
			if len(commit) > 7 {
				commit = commit[:7]
			}
			fmt.Printf("codeany %s (%s %s)\n", appVersion, commit, appDate)
		}
		fmt.Printf("  go:   %s\n", runtime.Version())
		fmt.Printf("  os:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		fmt.Printf("Model:           %s\n", cfg.Model)
		fmt.Printf("Permission Mode: %s\n", cfg.PermissionMode)
		fmt.Printf("Max Turns:       %d\n", cfg.MaxTurns)
		fmt.Printf("Config Dir:      %s\n", config.GlobalConfigDir())
		if len(cfg.MCPServers) > 0 {
			fmt.Printf("MCP Servers:     %d configured\n", len(cfg.MCPServers))
		}
	},
}

var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"upgrade"},
	Short:   "Update codeany to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		return selfUpdate()
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment and configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("codeany %s\n\n", appVersion)
		fmt.Printf("Environment:\n")
		fmt.Printf("  OS:       %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  Go:       %s\n", runtime.Version())

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "(unknown)"
		}
		fmt.Printf("  Shell:    %s\n", shell)

		if out, err := exec.Command("git", "--version").Output(); err == nil {
			fmt.Printf("  Git:      %s", strings.TrimSpace(string(out)))
		} else {
			fmt.Printf("  Git:      ✗ not found\n")
		}

		// API key
		for _, env := range []string{"CODEANY_API_KEY", "ANTHROPIC_API_KEY"} {
			if v := os.Getenv(env); v != "" {
				fmt.Printf("  API Key:  ✓ from %s\n", env)
				break
			}
		}

		fmt.Printf("  Config:   %s\n", config.GlobalConfigDir())

		cfg := config.Load()
		fmt.Printf("\nSettings:\n")
		fmt.Printf("  Model:    %s\n", cfg.Model)
		fmt.Printf("  Mode:     %s\n", cfg.PermissionMode)
		if len(cfg.MCPServers) > 0 {
			fmt.Printf("  MCP:      %d servers\n", len(cfg.MCPServers))
		}
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	cfg := config.Load()
	if err := config.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create config dirs: %w", err)
	}

	// Apply CLI flag overrides
	if flagModel != "" {
		cfg.Model = flagModel
	}
	if flagPermission != "" {
		cfg.PermissionMode = flagPermission
	}
	if flagYes {
		cfg.PermissionMode = "bypassPermissions"
	}
	if flagMaxTurns > 0 {
		cfg.MaxTurns = flagMaxTurns
	}
	if flagSystemPrompt != "" {
		cfg.SystemPrompt = flagSystemPrompt
	}
	if flagNoMCP {
		cfg.MCPServers = nil
	}
	if flagCWD != "" {
		if err := os.Chdir(flagCWD); err != nil {
			return fmt.Errorf("failed to change directory: %w", err)
		}
	}

	// Collect initial prompt from args
	initialPrompt := strings.Join(args, " ")

	// Pipe mode: read from stdin
	if flagPipe || flagPrint {
		return runPipeMode(cmd.Context(), cfg, initialPrompt)
	}

	// Check if stdin is a pipe (non-TTY)
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return runPipeMode(cmd.Context(), cfg, initialPrompt)
	}

	// Interactive REPL mode
	return runInteractive(cfg, initialPrompt)
}

func runPipeMode(ctx context.Context, cfg *config.Config, prompt string) error {
	if prompt == "" {
		reader := bufio.NewReader(os.Stdin)
		var sb strings.Builder
		for {
			line, err := reader.ReadString('\n')
			sb.WriteString(line)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		prompt = strings.TrimSpace(sb.String())
	}

	if prompt == "" {
		return fmt.Errorf("no prompt provided")
	}

	return pipe.Run(ctx, cfg, prompt, flagOutputFmt)
}

func runInteractive(cfg *config.Config, initialPrompt string) error {
	m := tui.NewModel(cfg, initialPrompt, flagResume)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	m.SetProgram(p)
	_, err := p.Run()
	return err
}

// ─── Self-update ───────────────────────────────

func selfUpdate() error {
	fmt.Println("Checking for updates...")

	// Get latest release from GitHub
	resp, err := http.Get("https://api.github.com/repos/codeany-ai/codeany/releases/latest")
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to check for updates (HTTP %d)", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	if release.TagName == appVersion || release.TagName == "v"+appVersion {
		fmt.Printf("Already up to date (%s)\n", appVersion)
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", release.TagName, appVersion)

	// Find matching asset
	osName := runtime.GOOS
	archName := runtime.GOARCH
	targetName := fmt.Sprintf("codeany_%s_%s", osName, archName)

	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, targetName) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary available for %s/%s. Install from source:\n  go install github.com/codeany-ai/codeany/cmd/codeany@latest", osName, archName)
	}

	fmt.Printf("Downloading %s...\n", filepath.Base(downloadURL))

	// Download to temp file
	dlResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer dlResp.Body.Close()

	tmpFile, err := os.CreateTemp("", "codeany-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// Extract if tar.gz
	if strings.HasSuffix(downloadURL, ".tar.gz") {
		extractDir, err := os.MkdirTemp("", "codeany-extract-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(extractDir)

		cmd := exec.Command("tar", "-xzf", tmpFile.Name(), "-C", extractDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}

		// Find the binary
		extractedBin := filepath.Join(extractDir, "codeany")
		if _, err := os.Stat(extractedBin); err != nil {
			return fmt.Errorf("binary not found in archive")
		}

		os.Remove(tmpFile.Name())
		tmpFile2, _ := os.CreateTemp("", "codeany-bin-*")
		tmpFile2.Close()

		// Copy extracted binary
		src, _ := os.ReadFile(extractedBin)
		os.WriteFile(tmpFile2.Name(), src, 0755)
		os.Remove(tmpFile.Name())
		tmpFile = tmpFile2
	}

	os.Chmod(tmpFile.Name(), 0755)

	// Find current binary path
	currentBin, err := os.Executable()
	if err != nil {
		currentBin = "/usr/local/bin/codeany"
	}
	currentBin, _ = filepath.EvalSymlinks(currentBin)

	// Replace binary
	if err := os.Rename(tmpFile.Name(), currentBin); err != nil {
		// Try copy if rename fails (cross-device)
		src, _ := os.ReadFile(tmpFile.Name())
		if err := os.WriteFile(currentBin, src, 0755); err != nil {
			return fmt.Errorf("failed to install update: %w\nTry: sudo mv %s %s", err, tmpFile.Name(), currentBin)
		}
	}

	fmt.Printf("✓ Updated to %s\n", release.TagName)
	return nil
}
