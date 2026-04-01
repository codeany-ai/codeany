package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// PermissionRules stores persistent permission rules
type PermissionRules struct {
	AlwaysAllow map[string]bool `json:"alwaysAllow"` // tool name -> always allow
	mu          sync.RWMutex
	path        string
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

// IsAllowed checks if a tool is always allowed
func (pr *PermissionRules) IsAllowed(toolName string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.AlwaysAllow[toolName]
}

// SetAlwaysAllow marks a tool as always allowed and saves to disk
func (pr *PermissionRules) SetAlwaysAllow(toolName string) {
	pr.mu.Lock()
	pr.AlwaysAllow[toolName] = true
	pr.mu.Unlock()
	pr.save()
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
