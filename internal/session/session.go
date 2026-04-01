package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session represents a conversation session
type Session struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	StartTime time.Time `json:"startTime"`
	Dir       string    `json:"dir"`
	Model     string    `json:"model,omitempty"`
	Title     string    `json:"title,omitempty"` // brief summary
	Turns     int       `json:"turns,omitempty"`
	Cost      float64   `json:"cost,omitempty"`
	path      string
}

// SessionSummary is a lightweight view for listing
type SessionSummary struct {
	ID        string
	Project   string
	StartTime time.Time
	Title     string
	Turns     int
	Dir       string
}

// New creates a new session
func New(sessDir, cwd string) *Session {
	id := generateID()
	s := &Session{
		ID:        id,
		Project:   filepath.Base(cwd),
		StartTime: time.Now(),
		Dir:       cwd,
		path:      filepath.Join(sessDir, id+".json"),
	}
	s.Save()
	return s
}

// Resume loads the most recent session for the given directory
func Resume(sessDir, cwd string) *Session {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return New(sessDir, cwd)
	}

	var latest *Session
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessDir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if s.Dir == cwd && s.StartTime.After(latestTime) {
			latest = &s
			latest.path = filepath.Join(sessDir, entry.Name())
			latestTime = s.StartTime
		}
	}

	if latest != nil {
		return latest
	}

	return New(sessDir, cwd)
}

// ListRecent lists recent sessions, newest first
func ListRecent(sessDir string, maxCount int, filterDir string) []SessionSummary {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return nil
	}

	var sessions []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessDir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if filterDir != "" && s.Dir != filterDir {
			continue
		}

		title := s.Title
		if title == "" {
			title = s.Project
		}

		sessions = append(sessions, SessionSummary{
			ID:        s.ID,
			Project:   s.Project,
			StartTime: s.StartTime,
			Title:     title,
			Turns:     s.Turns,
			Dir:       s.Dir,
		})
	}

	// Sort by start time descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	if maxCount > 0 && len(sessions) > maxCount {
		sessions = sessions[:maxCount]
	}

	return sessions
}

// FormatSessionList formats sessions for display
func FormatSessionList(sessions []SessionSummary) string {
	if len(sessions) == 0 {
		return "No recent sessions found."
	}

	var b strings.Builder
	b.WriteString("Recent sessions:\n\n")

	for _, s := range sessions {
		age := time.Since(s.StartTime)
		ageStr := formatAge(age)

		title := s.Title
		if len(title) > 50 {
			title = title[:50] + "..."
		}

		turns := ""
		if s.Turns > 0 {
			turns = fmt.Sprintf(" (%d turns)", s.Turns)
		}

		b.WriteString(fmt.Sprintf("  %s  %s  %s%s\n", s.ID[:8], ageStr, title, turns))
	}

	b.WriteString("\nResume with: codeany --resume or /resume <id>")
	return b.String()
}

// UpdateMeta updates session metadata
func (s *Session) UpdateMeta(model string, turns int, cost float64, title string) {
	s.Model = model
	s.Turns = turns
	s.Cost = cost
	if title != "" {
		s.Title = title
	}
	s.Save()
}

// Save persists the session to disk
func (s *Session) Save() error {
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now  "
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago    ", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago    ", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "yesterday "
	}
	if days < 7 {
		return fmt.Sprintf("%dd ago    ", days)
	}
	return fmt.Sprintf("%dw ago    ", days/7)
}
