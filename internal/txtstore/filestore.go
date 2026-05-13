package txtstore

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileStore manages a sectioned markdown file with an embedded index.
type FileStore struct {
	filepath string
}

// New creates a new FileStore for the given file path.
func New(filepath string) *FileStore {
	return &FileStore{filepath: filepath}
}

// anchorFromTitle converts a title to a markdown anchor ID.
// e.g., "Implementation Notes" → "implementation-notes"
func anchorFromTitle(title string) string {
	// Lowercase
	a := strings.ToLower(title)
	// Replace spaces with hyphens
	a = strings.ReplaceAll(a, " ", "-")
	// Remove special characters (keep alphanumeric and hyphens)
	re := regexp.MustCompile(`[^a-z0-9\-]`)
	a = re.ReplaceAllString(a, "")
	// Collapse multiple hyphens
	re = regexp.MustCompile(`-+`)
	a = re.ReplaceAllString(a, "-")
	// Trim hyphens from edges
	a = strings.Trim(a, "-")
	return a
}

// buildHeading constructs the full heading with optional unit/section hierarchy.
// e.g., unit="Auth", section="OAuth", title="Google" → "## Auth > OAuth > Google"
func buildHeading(title, unit, section string) string {
	var parts []string
	if unit != "" {
		parts = append(parts, unit)
	}
	if section != "" {
		parts = append(parts, section)
	}
	parts = append(parts, title)
	return "## " + strings.Join(parts, " > ")
}

// indexMarker returns the TOC marker comment.
const indexStart = "<!-- INDEX -->"
const indexEnd = "<!-- /INDEX -->"

// readOrCreate reads the file if it exists, or returns empty content.
// Creates parent directories if needed.
func (fs *FileStore) readOrCreate() (string, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(fs.filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := os.ReadFile(fs.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

// writeAtomically writes content to the file safely.
func (fs *FileStore) writeAtomically(content string) error {
	dir := filepath.Dir(fs.filepath)
	tmpFile := filepath.Join(dir, ".txtstore.tmp")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	return os.Rename(tmpFile, fs.filepath)
}

// buildIndex generates the TOC from all ## headings in the content.
func buildIndex(content string) string {
	lines := strings.Split(content, "\n")
	var toc []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") {
			// Extract title after "## "
			title := strings.TrimPrefix(line, "## ")
			// Use the last part (after " > ") as the anchor text
			parts := strings.Split(title, " > ")
			anchorText := parts[len(parts)-1]
			anchor := anchorFromTitle(anchorText)
			toc = append(toc, fmt.Sprintf("- [%s](#%s)", anchorText, anchor))
		}
	}

	if len(toc) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(indexStart + "\n")
	for _, entry := range toc {
		sb.WriteString(entry + "\n")
	}
	sb.WriteString(indexEnd)
	return sb.String()
}

// extractContentWithoutIndex removes the index block from content.
func extractContentWithoutIndex(content string) string {
	startIdx := strings.Index(content, indexStart)
	endIdx := strings.Index(content, indexEnd)

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return content
	}

	// Get content before index and after index
	before := content[:startIdx]
	after := content[endIdx+len(indexEnd):]

	// Clean up whitespace
	before = strings.TrimRight(before, "\n\r")
	after = strings.TrimLeft(after, "\n\r")

	if before == "" {
		return after
	}
	if after == "" {
		return before
	}
	return before + "\n\n" + after
}

// findSectionIndex finds the line index of a heading matching the title.
// Returns -1 if not found.
func findSectionIndex(content, title string) int {
	lines := strings.Split(content, "\n")
	targetHeading := buildHeading(title, "", "")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == targetHeading {
			return i
		}
	}
	return -1
}

// findUniqueTitle finds a unique title by appending -2, -3, etc.
func findUniqueTitle(content, title string) string {
	if findSectionIndex(content, title) == -1 {
		return title
	}

	counter := 2
	for {
		candidate := fmt.Sprintf("%s-%d", title, counter)
		if findSectionIndex(content, candidate) == -1 {
			return candidate
		}
		counter++
	}
}

// Edit replaces an existing section with new content.
func (fs *FileStore) Edit(title, content, unit, section string) error {
	existing, err := fs.readOrCreate()
	if err != nil {
		return err
	}

	// If file is empty or doesn't exist, create new
	if existing == "" {
		return fs.createFile(title, content, unit, section)
	}

	// Check if section exists
	heading := buildHeading(title, unit, section)
	sectionIdx := findSectionIndex(existing, title)

	if sectionIdx == -1 {
		// Section doesn't exist, create it
		return fs.createFile(title, content, unit, section)
	}

	// Section exists, replace it
	lines := strings.Split(existing, "\n")

	// Find the end of the current section (next ## heading or end of file)
	sectionEnd := len(lines)
	for i := sectionIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			sectionEnd = i
			break
		}
	}

	// Build new section
	newSection := heading + "\n" + content
	if !strings.HasSuffix(content, "\n") {
		newSection += "\n"
	}

	// Replace
	newLines := append(lines[:sectionIdx], append([]string{newSection}, lines[sectionEnd:]...)...)
	newContent := strings.Join(newLines, "\n")

	// Rebuild index
	contentWithoutIndex := extractContentWithoutIndex(newContent)
	index := buildIndex(contentWithoutIndex)

	var sb strings.Builder
	if index != "" {
		sb.WriteString(index + "\n\n")
	}
	sb.WriteString(contentWithoutIndex)

	return fs.writeAtomically(sb.String())
}

// Append adds a new section to the file.
func (fs *FileStore) Append(title, content, unit, section string) error {
	existing, err := fs.readOrCreate()
	if err != nil {
		return err
	}

	// Find unique title
	title = findUniqueTitle(existing, title)

	// If file is empty or doesn't exist, create new
	if existing == "" {
		return fs.createFile(title, content, unit, section)
	}

	// Extract content without index
	contentWithoutIndex := extractContentWithoutIndex(existing)

	// Build new section
	heading := buildHeading(title, unit, section)
	sectionContent := heading + "\n" + content
	if !strings.HasSuffix(content, "\n") {
		sectionContent += "\n"
	}

	// Append section
	newContent := contentWithoutIndex + "\n\n" + sectionContent

	// Rebuild index
	index := buildIndex(newContent)

	var sb strings.Builder
	if index != "" {
		sb.WriteString(index + "\n\n")
	}
	sb.WriteString(newContent)

	return fs.writeAtomically(sb.String())
}

// createFile creates a new section in the file.
func (fs *FileStore) createFile(title, content, unit, section string) error {
	heading := buildHeading(title, unit, section)
	sectionContent := heading + "\n" + content
	if !strings.HasSuffix(content, "\n") {
		sectionContent += "\n"
	}

	var sb strings.Builder
	sb.WriteString(indexStart + "\n")
	sb.WriteString(fmt.Sprintf("- [%s](#%s)\n", title, anchorFromTitle(title)))
	sb.WriteString(indexEnd + "\n\n")
	sb.WriteString(sectionContent)

	return fs.writeAtomically(sb.String())
}

// WriteSwimPlan renders a complete SWIM markdown plan document and overwrites
// the target file atomically.
func (fs *FileStore) WriteSwimPlan(doc SwimPlanDoc) error {
	content, err := RenderSwimPlan(doc)
	if err != nil {
		return err
	}
	if _, err := fs.readOrCreate(); err != nil {
		return err
	}
	return fs.writeAtomically(content)
}
