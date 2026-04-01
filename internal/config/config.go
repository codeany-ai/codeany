package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeany-ai/open-agent-sdk-go/types"
	"gopkg.in/yaml.v3"
)

const (
	AppName    = "codeany"
	ConfigDir  = ".codeany"
	ConfigFile = "settings.json"
	MemoryDir  = "memory"
	SessionDir = "sessions"
)

// Config represents the application configuration
type Config struct {
	// Model settings
	Model    string `json:"model,omitempty"`
	SmallModel string `json:"smallModel,omitempty"`

	// API settings
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"`

	// Permission settings
	PermissionMode string `json:"permissionMode,omitempty"`

	// Tool settings
	AllowedTools []string `json:"allowedTools,omitempty"`

	// MCP servers
	MCPServers map[string]types.MCPServerConfig `json:"mcpServers,omitempty"`

	// Custom system prompt
	SystemPrompt       string `json:"systemPrompt,omitempty"`
	AppendSystemPrompt string `json:"appendSystemPrompt,omitempty"`

	// Hooks
	Hooks *HookConfig `json:"hooks,omitempty"`

	// UI
	Theme string `json:"theme,omitempty"`

	// Proxy
	ProxyURL string `json:"proxyURL,omitempty"`

	// Max turns
	MaxTurns int `json:"maxTurns,omitempty"`

	// Budget
	MaxBudgetUSD float64 `json:"maxBudgetUSD,omitempty"`

	// Custom headers
	CustomHeaders map[string]string `json:"customHeaders,omitempty"`
}

type HookConfig struct {
	PreToolUse  []HookRule `json:"preToolUse,omitempty"`
	PostToolUse []HookRule `json:"postToolUse,omitempty"`
}

type HookRule struct {
	Matcher string `json:"matcher"`
	Command string `json:"command"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Model:          "sonnet-4-6",
		PermissionMode: "default",
		MaxTurns:       100,
	}
}

// Load loads configuration from all sources (env -> global config -> project config -> CLI flags)
func Load() *Config {
	cfg := DefaultConfig()

	// Load global config (JSON first, then YAML fallback)
	globalCfg := loadConfigFile(GlobalConfigPath())
	if globalCfg != nil {
		mergeConfig(cfg, globalCfg)
	} else {
		// Try YAML config
		yamlPath := filepath.Join(GlobalConfigDir(), "config.yaml")
		yamlCfg := loadYAMLConfig(yamlPath)
		if yamlCfg != nil {
			mergeConfig(cfg, yamlCfg)
		}
	}

	// Load project config
	projectCfg := loadConfigFile(ProjectConfigPath())
	if projectCfg != nil {
		mergeConfig(cfg, projectCfg)
	}

	// Override from environment
	applyEnvOverrides(cfg)

	return cfg
}

// GlobalConfigDir returns ~/.codeany/
func GlobalConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDir)
}

// GlobalConfigPath returns ~/.codeany/settings.json
func GlobalConfigPath() string {
	return filepath.Join(GlobalConfigDir(), ConfigFile)
}

// ProjectConfigPath returns .codeany/settings.json in CWD
func ProjectConfigPath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ConfigDir, ConfigFile)
}

// MemoryPath returns the global memory directory
func MemoryPath() string {
	return filepath.Join(GlobalConfigDir(), MemoryDir)
}

// ProjectMemoryPath returns project-specific memory directory
func ProjectMemoryPath() string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	// Create a project-specific path under ~/.codeany/projects/
	projectKey := strings.ReplaceAll(cwd, "/", "-")
	projectKey = strings.TrimPrefix(projectKey, "-")
	return filepath.Join(home, ConfigDir, "projects", projectKey, MemoryDir)
}

// SessionPath returns the sessions directory
func SessionPath() string {
	return filepath.Join(GlobalConfigDir(), SessionDir)
}

// EnsureDirs creates necessary directories
func EnsureDirs() error {
	dirs := []string{
		GlobalConfigDir(),
		MemoryPath(),
		SessionPath(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func loadConfigFile(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

// YAMLConfig represents the legacy YAML config format
type YAMLConfig struct {
	DefaultModel string `yaml:"default_model"`
	PermissionMode string `yaml:"permission_mode"`
	MaxIterations int `yaml:"max_iterations"`
	Models map[string]struct {
		APIKey  string `yaml:"api_key"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"models"`
}

func loadYAMLConfig(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ycfg YAMLConfig
	if err := yaml.Unmarshal(data, &ycfg); err != nil {
		return nil
	}

	cfg := &Config{}
	if ycfg.DefaultModel != "" {
		cfg.Model = ycfg.DefaultModel
	}
	if ycfg.PermissionMode != "" {
		cfg.PermissionMode = ycfg.PermissionMode
	}
	if ycfg.MaxIterations > 0 {
		cfg.MaxTurns = ycfg.MaxIterations
	}
	// Extract API key from anthropic provider
	if m, ok := ycfg.Models["anthropic"]; ok && m.APIKey != "" {
		cfg.APIKey = m.APIKey
	}
	return cfg
}

func mergeConfig(dst, src *Config) {
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.SmallModel != "" {
		dst.SmallModel = src.SmallModel
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.PermissionMode != "" {
		dst.PermissionMode = src.PermissionMode
	}
	if len(src.AllowedTools) > 0 {
		dst.AllowedTools = src.AllowedTools
	}
	if len(src.MCPServers) > 0 {
		if dst.MCPServers == nil {
			dst.MCPServers = make(map[string]types.MCPServerConfig)
		}
		for k, v := range src.MCPServers {
			dst.MCPServers[k] = v
		}
	}
	if src.SystemPrompt != "" {
		dst.SystemPrompt = src.SystemPrompt
	}
	if src.AppendSystemPrompt != "" {
		dst.AppendSystemPrompt = src.AppendSystemPrompt
	}
	if src.ProxyURL != "" {
		dst.ProxyURL = src.ProxyURL
	}
	if src.MaxTurns > 0 {
		dst.MaxTurns = src.MaxTurns
	}
	if src.MaxBudgetUSD > 0 {
		dst.MaxBudgetUSD = src.MaxBudgetUSD
	}
	if len(src.CustomHeaders) > 0 {
		dst.CustomHeaders = src.CustomHeaders
	}
	if src.Hooks != nil {
		dst.Hooks = src.Hooks
	}
}

func applyEnvOverrides(cfg *Config) {
	envKeys := []string{"CODEANY_API_KEY", "ANTHROPIC_API_KEY"}
	for _, key := range envKeys {
		if v := os.Getenv(key); v != "" {
			cfg.APIKey = v
			break
		}
	}

	envURLs := []string{"CODEANY_BASE_URL", "ANTHROPIC_BASE_URL"}
	for _, key := range envURLs {
		if v := os.Getenv(key); v != "" {
			cfg.BaseURL = v
			break
		}
	}

	envModels := []string{"CODEANY_MODEL", "ANTHROPIC_MODEL"}
	for _, key := range envModels {
		if v := os.Getenv(key); v != "" {
			cfg.Model = v
			break
		}
	}

	if v := os.Getenv("HTTPS_PROXY"); v != "" {
		cfg.ProxyURL = v
	} else if v := os.Getenv("HTTP_PROXY"); v != "" {
		cfg.ProxyURL = v
	}
}

// GetPermissionMode converts string to types.PermissionMode
func (c *Config) GetPermissionMode() types.PermissionMode {
	switch c.PermissionMode {
	case "acceptEdits":
		return types.PermissionModeAcceptEdits
	case "bypassPermissions":
		return types.PermissionModeBypassPermissions
	case "plan":
		return types.PermissionModePlan
	default:
		return types.PermissionModeDefault
	}
}
