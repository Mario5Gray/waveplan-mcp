package model

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// NotesFile is a txtstore-style sectioned markdown document.
type NotesFile struct {
	Path     string        `json:"path"`
	Sections []NoteSection `json:"sections"`
}

// NoteSection represents one markdown heading and its body.
type NoteSection struct {
	Heading string `json:"heading"`
	Unit    string `json:"unit,omitempty"`
	Section string `json:"section,omitempty"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// LoadNotes reads a txtstore-style markdown notes file.
func LoadNotes(path string) (*NotesFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read notes %q: %w", path, err)
	}
	return ParseNotes(path, string(body)), nil
}

// ParseNotes parses level-two markdown headings into note sections.
func ParseNotes(path, body string) *NotesFile {
	notes := &NotesFile{Path: path}
	scanner := bufio.NewScanner(strings.NewReader(body))
	var current *NoteSection
	var lines []string

	flush := func() {
		if current == nil {
			return
		}
		current.Content = strings.TrimSpace(strings.Join(lines, "\n"))
		notes.Sections = append(notes.Sections, *current)
		lines = nil
	}

	inIndex := false
	for scanner.Scan() {
		line := scanner.Text()
		switch strings.TrimSpace(line) {
		case "<!-- INDEX -->":
			inIndex = true
			continue
		case "<!-- /INDEX -->":
			inIndex = false
			continue
		}
		if inIndex {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			flush()
			current = newNoteSection(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if current != nil {
			lines = append(lines, line)
		}
	}
	flush()
	return notes
}

func newNoteSection(heading string) *NoteSection {
	parts := strings.Split(heading, " > ")
	section := &NoteSection{
		Heading: heading,
		Title:   heading,
	}
	switch len(parts) {
	case 0:
	case 1:
		section.Title = parts[0]
	case 2:
		section.Unit = parts[0]
		section.Title = parts[1]
	default:
		section.Unit = parts[0]
		section.Section = strings.Join(parts[1:len(parts)-1], " > ")
		section.Title = parts[len(parts)-1]
	}
	return section
}
