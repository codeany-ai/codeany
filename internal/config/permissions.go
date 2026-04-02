package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PermissionRules stores persistent permission rules
type PermissionRules struct {
	AlwaysAllow map[string]bool `json:"alwaysAllow"`         // tool name -> always allow
	AllowRules  []PermRule      `json:"allowRules,omitempty"` // pattern-based allow rules
	DenyRules   []PermRule      `json:"denyRules,omitempty"`  // pattern-based deny rules
	mu          sync.RWMutex
	path        string
}

// PermRule is a pattern-based permission rule
type PermRule struct {
	Tool    string `json:"tool"`              // tool name (e.g., "Bash", "Edit", "*")
	Pattern string `json:"pattern,omitempty"` // pattern to match against input (e.g., "git *", "src/**")
}

// LoadPermissionRules loads rules from disk
func LoadPermissionRules() *PermissionRules {
	path := filepath.Join(GlobalConfigDir(), "permissions.json")
	pr := &PermissionRules{
		AlwaysAllow: make(map[string]bool),
		path:        path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return pr
	}

	json.Unmarshal(data, pr)
	if pr.AlwaysAllow == nil {
		pr.AlwaysAllow = make(map[string]bool)
	}
	return pr
}

// IsAllowed checks if a tool call is allowed by rules
func (pr *PermissionRules) IsAllowed(toolName string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.AlwaysAllow[toolName]
}

// IsAllowedWithInput checks allow/deny rules with input pattern matching
func (pr *PermissionRules) IsAllowedWithInput(toolName string, input map[string]interface{}) (allowed bool, denied bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	// Check deny rules first
	for _, rule := range pr.DenyRules {
		if matchRule(rule, toolName, input) {
			return false, true
		}
	}

	// Check always allow
	if pr.AlwaysAllow[toolName] {
		return true, false
	}

	// Check allow rules
	for _, rule := range pr.AllowRules {
		if matchRule(rule, toolName, input) {
			return true, false
		}
	}

	return false, false
}

// SetAlwaysAllow marks a tool as always allowed and saves to disk
func (pr *PermissionRules) SetAlwaysAllow(toolName string) {
	pr.mu.Lock()
	pr.AlwaysAllow[toolName] = true
	pr.mu.Unlock()
	pr.save()
}

// AddAllowRule adds a pattern-based allow rule
func (pr *PermissionRules) AddAllowRule(tool, pattern string) {
	pr.mu.Lock()
	pr.AllowRules = append(pr.AllowRules, PermRule{Tool: tool, Pattern: pattern})
	pr.mu.Unlock()
	pr.save()
}

// AddDenyRule adds a pattern-based deny rule
func (pr *PermissionRules) AddDenyRule(tool, pattern string) {
	pr.mu.Lock()
	pr.DenyRules = append(pr.DenyRules, PermRule{Tool: tool, Pattern: pattern})
	pr.mu.Unlock()
	pr.save()
}

// RemoveAlwaysAllow removes an always-allow entry
func (pr *PermissionRules) RemoveAlwaysAllow(toolName string) {
	pr.mu.Lock()
	delete(pr.AlwaysAllow, toolName)
	pr.mu.Unlock()
	pr.save()
}

// FormatRules returns a readable summary
func (pr *PermissionRules) FormatRules() string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var b strings.Builder
	b.WriteString("Permission rules:\n\n")

	if len(pr.AlwaysAllow) > 0 {
		b.WriteString("  Always allow:\n")
		for tool := range pr.AlwaysAllow {
			b.WriteString(fmt.Sprintf("    ✓ %s\n", tool))
		}
	}

	if len(pr.AllowRules) > 0 {
		b.WriteString("  Allow rules:\n")
		for _, r := range pr.AllowRules {
			p := r.Pattern
			if p == "" {
				p = "*"
			}
			b.WriteString(fmt.Sprintf("    ✓ %s (%s)\n", r.Tool, p))
		}
	}

	if len(pr.DenyRules) > 0 {
		b.WriteString("  Deny rules:\n")
		for _, r := range pr.DenyRules {
			p := r.Pattern
			if p == "" {
				p = "*"
			}
			b.WriteString(fmt.Sprintf("    ✗ %s (%s)\n", r.Tool, p))
		}
	}

	if len(pr.AlwaysAllow) == 0 && len(pr.AllowRules) == 0 && len(pr.DenyRules) == 0 {
		b.WriteString("  (no rules configured)\n")
	}

	b.WriteString(fmt.Sprintf("\n  Stored in: %s", pr.path))
	return b.String()
}

// save writes rules to disk
func (pr *PermissionRules) save() {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	data, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(pr.path), 0755)
	os.WriteFile(pr.path, data, 0644)
}

// matchRule checks if a rule matches a tool call
func matchRule(rule PermRule, toolName string, input map[string]interface{}) bool {
	if rule.Tool != "*" && rule.Tool != toolName {
		return false
	}
	if rule.Pattern == "" {
		return true
	}
	// Match pattern against relevant input field
	var value string
	switch toolName {
	case "Bash":
		value, _ = input["command"].(string)
	case "Edit", "Write", "Read":
		value, _ = input["file_path"].(string)
	case "Glob":
		value, _ = input["pattern"].(string)
	case "Grep":
		value, _ = input["pattern"].(string)
	default:
		return true
	}
	return simpleMatch(rule.Pattern, value)
}

func simpleMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(value, strings.TrimPrefix(pattern, "*"))
	}
	return pattern == value
}
