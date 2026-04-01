package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill definition
type Skill struct {
	Name        string
	Description string
	WhenToUse   string
	ArgHint     string
	Content     string // The instruction body
	FilePath    string
	Source      string // "project", "user"
}

// LoadAll discovers and loads skills from project and user directories
func LoadAll() []Skill {
	var skills []Skill

	// Project skills: .codeany/skills/ and .claude/skills/
	cwd, _ := os.Getwd()
	projectDirs := []string{
		filepath.Join(cwd, ".codeany", "skills"),
		filepath.Join(cwd, ".claude", "skills"),
	}
	for _, dir := range projectDirs {
		for _, s := range loadFromDir(dir, "project") {
			skills = append(skills, s)
		}
	}

	// User skills: ~/.codeany/skills/ and ~/.claude/skills/
	home, _ := os.UserHomeDir()
	userDirs := []string{
		filepath.Join(home, ".codeany", "skills"),
		filepath.Join(home, ".claude", "skills"),
	}
	for _, dir := range userDirs {
		for _, s := range loadFromDir(dir, "user") {
			skills = append(skills, s)
		}
	}

	return skills
}

// FindByName finds a skill by name (case-insensitive)
func FindByName(skills []Skill, name string) *Skill {
	name = strings.ToLower(name)
	for i, s := range skills {
		if strings.ToLower(s.Name) == name {
			return &skills[i]
		}
	}
	return nil
}

func loadFromDir(dir, source string) []Skill {
	var skills []Skill

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		s := parseSkill(string(data), entry.Name(), source)
		s.FilePath = skillFile
		skills = append(skills, s)
	}

	return skills
}

func parseSkill(content, dirName, source string) Skill {
	s := Skill{
		Name:   dirName,
		Source: source,
	}

	// Parse frontmatter
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) >= 3 {
			parseFrontmatter(parts[1], &s)
			s.Content = strings.TrimSpace(parts[2])
		} else {
			s.Content = content
		}
	} else {
		s.Content = content
	}

	if s.Name == "" {
		s.Name = dirName
	}

	return s
}

func parseFrontmatter(fm string, s *Skill) {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			s.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			s.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		} else if strings.HasPrefix(line, "whenToUse:") {
			s.WhenToUse = strings.TrimSpace(strings.TrimPrefix(line, "whenToUse:"))
		} else if strings.HasPrefix(line, "argumentHint:") {
			s.ArgHint = strings.TrimSpace(strings.TrimPrefix(line, "argumentHint:"))
		}
	}
}

// FormatForPrompt formats all skills for inclusion in system prompt
func FormatForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Available Skills\n\n")
	b.WriteString("The following skills are available. Invoke via /skill-name:\n\n")

	for _, s := range skills {
		b.WriteString(fmt.Sprintf("- **%s**", s.Name))
		if s.Description != "" {
			b.WriteString(": " + s.Description)
		}
		if s.ArgHint != "" {
			b.WriteString(fmt.Sprintf(" (args: %s)", s.ArgHint))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatSkillList formats skills for display in /skills command
func FormatSkillList(skills []Skill) string {
	if len(skills) == 0 {
		return "No skills found.\n\nCreate skills in .codeany/skills/<name>/SKILL.md or ~/.codeany/skills/<name>/SKILL.md"
	}

	var b strings.Builder
	b.WriteString("Available skills:\n\n")

	for _, s := range skills {
		src := ""
		if s.Source == "user" {
			src = " (user)"
		}
		b.WriteString(fmt.Sprintf("  /%s%s", s.Name, src))
		if s.Description != "" {
			b.WriteString(" — " + s.Description)
		}
		b.WriteString("\n")
	}

	b.WriteString("\nInvoke with: /<skill-name> [arguments]")
	return b.String()
}
