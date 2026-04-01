package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Plugin represents a loaded plugin
type Plugin struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Enabled     bool     `json:"enabled"`
	Source      string   `json:"source"` // "builtin", "user", "project"
	Dir         string   `json:"-"`
	Skills      []Skill  `json:"-"`
	Hooks       []Hook   `json:"-"`
	Commands    []string `json:"-"`
}

// Skill is a skill provided by a plugin
type Skill struct {
	Name        string
	Description string
	Content     string
}

// Hook is a hook provided by a plugin
type Hook struct {
	Event   string // "preToolUse", "postToolUse"
	Matcher string
	Command string
}

// Manifest is the plugin.json format
type Manifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// LoadAll discovers and loads plugins from all sources
func LoadAll(configDir string) []Plugin {
	var plugins []Plugin

	// Load from ~/.codeany/plugins/
	userDir := filepath.Join(configDir, "plugins")
	for _, p := range loadFromDir(userDir, "user") {
		plugins = append(plugins, p)
	}

	// Load from project .codeany/plugins/
	cwd, _ := os.Getwd()
	projectDir := filepath.Join(cwd, ".codeany", "plugins")
	for _, p := range loadFromDir(projectDir, "project") {
		plugins = append(plugins, p)
	}

	return plugins
}

func loadFromDir(dir, source string) []Plugin {
	var plugins []Plugin

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(dir, entry.Name())
		p := loadPlugin(pluginDir, entry.Name(), source)
		if p != nil {
			plugins = append(plugins, *p)
		}
	}

	return plugins
}

func loadPlugin(dir, name, source string) *Plugin {
	p := &Plugin{
		ID:      fmt.Sprintf("%s@%s", name, source),
		Name:    name,
		Enabled: true,
		Source:  source,
		Dir:     dir,
	}

	// Read plugin.json manifest if exists
	manifestPath := filepath.Join(dir, "plugin.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var m Manifest
		if json.Unmarshal(data, &m) == nil {
			if m.Name != "" {
				p.Name = m.Name
			}
			p.Description = m.Description
			p.Version = m.Version
		}
	}

	// Load skills from plugin directory
	skillsDir := filepath.Join(dir, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
				if data, err := os.ReadFile(skillFile); err == nil {
					p.Skills = append(p.Skills, Skill{
						Name:    entry.Name(),
						Content: string(data),
					})
				}
			}
		}
	}

	// Also check for skill.md files directly in plugin root
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") && entry.Name() != "README.md" {
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err == nil {
					skillName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
					p.Skills = append(p.Skills, Skill{
						Name:    skillName,
						Content: string(data),
					})
				}
			}
		}
	}

	// Load hooks from hooks.json if exists
	hooksPath := filepath.Join(dir, "hooks.json")
	if data, err := os.ReadFile(hooksPath); err == nil {
		var hooks []Hook
		if json.Unmarshal(data, &hooks) == nil {
			p.Hooks = hooks
		}
	}

	return p
}

// FormatPluginList formats plugins for display
func FormatPluginList(plugins []Plugin) string {
	if len(plugins) == 0 {
		return "No plugins installed.\n\nCreate plugins in ~/.codeany/plugins/<name>/ with a plugin.json manifest."
	}

	var b strings.Builder
	b.WriteString("Installed plugins:\n\n")

	for _, p := range plugins {
		status := "✓"
		if !p.Enabled {
			status = "○"
		}

		ver := ""
		if p.Version != "" {
			ver = " v" + p.Version
		}

		b.WriteString(fmt.Sprintf("  %s %s%s (%s)\n", status, p.Name, ver, p.Source))
		if p.Description != "" {
			b.WriteString(fmt.Sprintf("    %s\n", p.Description))
		}
		if len(p.Skills) > 0 {
			b.WriteString(fmt.Sprintf("    Skills: %d\n", len(p.Skills)))
		}
		if len(p.Hooks) > 0 {
			b.WriteString(fmt.Sprintf("    Hooks: %d\n", len(p.Hooks)))
		}
	}

	return b.String()
}
