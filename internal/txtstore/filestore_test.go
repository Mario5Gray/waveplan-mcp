package txtstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnchorFromTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Implementation Notes", "implementation-notes"},
		{"Review Notes", "review-notes"},
		{"Design > UI", "design-ui"},
		{"Special!@#$Chars", "specialchars"},
		{"  Spaces  ", "spaces"},
		{"Multiple---Hyphens", "multiple-hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := anchorFromTitle(tt.input)
			if result != tt.expected {
				t.Errorf("anchorFromTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildHeading(t *testing.T) {
	tests := []struct {
		title   string
		unit    string
		section string
		want    string
	}{
		{"Notes", "", "", "## Notes"},
		{"Google", "Auth", "", "## Auth > Google"},
		{"OAuth", "Auth", "Security", "## Auth > Security > OAuth"},
		{"Design", "Frontend", "", "## Frontend > Design"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := buildHeading(tt.title, tt.unit, tt.section)
			if got != tt.want {
				t.Errorf("buildHeading(%q, %q, %q) = %q, want %q", tt.title, tt.unit, tt.section, got, tt.want)
			}
		})
	}
}

func TestBuildIndex(t *testing.T) {
	content := `## Notes
Some content

## Auth > Google
More content

## Design
Even more
`
	index := buildIndex(content)
	expected := `<!-- INDEX -->
- [Notes](#notes)
- [Google](#google)
- [Design](#design)
<!-- /INDEX -->`

	if index != expected {
		t.Errorf("buildIndex() = %q, want %q", index, expected)
	}
}

func TestExtractContentWithoutIndex(t *testing.T) {
	content := `<!-- INDEX -->
- [Notes](#notes)
<!-- /INDEX -->

## Notes
Content here
`
	result := extractContentWithoutIndex(content)
	expected := "## Notes\nContent here\n"

	if result != expected {
		t.Errorf("extractContentWithoutIndex() = %q, want %q", result, expected)
	}
}

func TestFindSectionIndex(t *testing.T) {
	content := `## Notes
Content

## Auth > Google
More
`
	idx := findSectionIndex(content, "Notes")
	if idx != 0 {
		t.Errorf("findSectionIndex() = %d, want 0", idx)
	}

	idx = findSectionIndex(content, "Google")
	if idx != -1 {
		t.Errorf("findSectionIndex() for 'Google' = %d, want -1", idx)
	}

	idx = findSectionIndex(content, "Auth > Google")
	if idx != 3 {
		t.Errorf("findSectionIndex() for 'Auth > Google' = %d, want 3", idx)
	}
}

func TestFindUniqueTitle(t *testing.T) {
	content := `## Notes
Content

## Notes-2
More
`
	title := findUniqueTitle(content, "Notes")
	if title != "Notes-3" {
		t.Errorf("findUniqueTitle() = %q, want %q", title, "Notes-3")
	}

	title = findUniqueTitle(content, "New")
	if title != "New" {
		t.Errorf("findUniqueTitle() for 'New' = %q, want %q", title, "New")
	}
}

func TestFileStoreAppend(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "test.md")

	store := New(filepath)

	// Append first section
	err := store.Append("Notes", "First content", "", "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "<!-- INDEX -->") {
		t.Error("Missing INDEX marker")
	}
	if !strings.Contains(string(data), "- [Notes](#notes)") {
		t.Error("Missing TOC entry for Notes")
	}
	if !strings.Contains(string(data), "## Notes") {
		t.Error("Missing heading for Notes")
	}
	if !strings.Contains(string(data), "First content") {
		t.Error("Missing content")
	}

	// Append duplicate title (should become Notes-2)
	err = store.Append("Notes", "Second content", "", "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	data, err = os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "## Notes-2") {
		t.Error("Missing heading for Notes-2")
	}
	if !strings.Contains(string(data), "- [Notes-2](#notes-2)") {
		t.Error("Missing TOC entry for Notes-2")
	}
}

func TestFileStoreEdit(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "test.md")

	store := New(filepath)

	// Append initial section
	err := store.Append("Notes", "Original content", "", "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Edit the section
	err = store.Edit("Notes", "Updated content", "", "")
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	// Verify content was replaced
	data, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if strings.Contains(string(data), "Original content") {
		t.Error("Old content still present after Edit")
	}
	if !strings.Contains(string(data), "Updated content") {
		t.Error("New content not found after Edit")
	}
}

func TestFileStoreEditWithHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "test.md")

	store := New(filepath)

	// Append with hierarchy
	err := store.Append("Google", "OAuth content", "Auth", "Security")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "## Auth > Security > Google") {
		t.Error("Missing hierarchical heading")
	}

	// Edit with same hierarchy
	err = store.Edit("Google", "Updated OAuth", "Auth", "Security")
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	data, err = os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if strings.Contains(string(data), "OAuth content") {
		t.Error("Old content still present")
	}
	if !strings.Contains(string(data), "Updated OAuth") {
		t.Error("New content not found")
	}
}

func TestFileStoreCreateParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "nested", "deep", "test.md")

	store := New(filepath)

	err := store.Append("Notes", "Content", "", "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		t.Error("File was not created")
	}
}