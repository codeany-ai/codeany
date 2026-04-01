package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadMemoryIndex reads the MEMORY.md index file
func LoadMemoryIndex(memDir string) string {
	indexPath := filepath.Join(memDir, "MEMORY.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// LoadAllMemories reads all memory files from the directory
func LoadAllMemories(memDir string) []MemoryEntry {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil
	}

	var memories []MemoryEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "MEMORY.md" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(memDir, entry.Name()))
		if err != nil {
			continue
		}

		mem := ParseMemory(string(data))
		mem.File = entry.Name()
		memories = append(memories, mem)
	}

	return memories
}

// MemoryEntry represents a parsed memory file
type MemoryEntry struct {
	Name        string
	Description string
	Type        string // user, feedback, project, reference
	Content     string
	File        string
}

// ParseMemory parses a memory file with frontmatter
func ParseMemory(content string) MemoryEntry {
	var mem MemoryEntry

	// Parse frontmatter
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) >= 3 {
			frontmatter := parts[1]
			mem.Content = strings.TrimSpace(parts[2])

			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					mem.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					mem.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				} else if strings.HasPrefix(line, "type:") {
					mem.Type = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
				}
			}
		}
	} else {
		mem.Content = content
	}

	return mem
}

// FormatForPrompt formats all memories as context for the system prompt
func FormatForPrompt(memDir string) string {
	index := LoadMemoryIndex(memDir)
	if index == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Memory\n\n")
	b.WriteString("## Index (MEMORY.md)\n")
	b.WriteString(index)
	b.WriteString("\n\n")

	memories := LoadAllMemories(memDir)
	if len(memories) > 0 {
		b.WriteString("## Memory Files\n\n")
		for _, mem := range memories {
			if mem.Name != "" {
				b.WriteString("### " + mem.Name)
				if mem.Type != "" {
					b.WriteString(" [" + mem.Type + "]")
				}
				b.WriteString("\n")
			}
			b.WriteString(mem.Content + "\n\n")
		}
	}

	return b.String()
}
