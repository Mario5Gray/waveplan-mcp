package contextsize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// codeExtensions are file extensions treated as code (higher density).
var codeExtensions = map[string]bool{
	".go":    true,
	".json":  true,
	".yaml":  true,
	".yml":   true,
	".toml":  true,
	".proto": true,
	".html":  true,
	".css":   true,
	".js":    true,
	".ts":    true,
	".tsx":   true,
	".py":    true,
	".sh":    true,
	".sql":   true,
}

// proseExtensions are file extensions treated as prose (lower density).
var proseExtensions = map[string]bool{
	".md":  true,
	".txt": true,
	".rst": true,
}

// headingRe matches markdown headings (## Title or ### Title, etc.).
var headingRe = regexp.MustCompile(`^#{1,6}\s+(.+)$`)

// Estimator performs deterministic context estimation.
type Estimator struct {
	BaseDir string // root for resolving relative file paths; empty = cwd
}

// Estimate returns a ContextEstimate for the given candidate and budget.
// Returns error only for invalid inputs (zero/negative budget) or IO conditions
// that prevent deterministic behavior (unreadable directory).
// Sparse candidates (no signal) return a valid estimate with low confidence.
func (e *Estimator) Estimate(c ContextCandidate, budget Budget) (ContextEstimate, error) {
	if budget.MinTokens <= 0 || budget.MaxTokens <= 0 {
		return ContextEstimate{}, fmt.Errorf("budget must have positive min and max tokens")
	}
	if budget.MinTokens > budget.MaxTokens {
		return ContextEstimate{}, fmt.Errorf("budget min (%d) exceeds max (%d)", budget.MinTokens, budget.MaxTokens)
	}

	est := ContextEstimate{
		BudgetMin:       budget.MinTokens,
		BudgetMax:       budget.MaxTokens,
		Drivers:         []string{},
		MissingFiles:    []string{},
		MissingSections: []SectionRef{},
		UnknownFiles:    []string{},
		SplitCandidates: []string{},
		MergeCandidates: []string{},
	}

	var totalTokens int
	var fileCount int
	var localImportCount int
	var hasSignal bool
	var hasHugeFile bool

	// Estimate tokens from referenced files.
	for _, refFile := range c.ReferencedFiles {
		path := refFile
		if e.BaseDir != "" {
			path = filepath.Join(e.BaseDir, refFile)
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				est.MissingFiles = append(est.MissingFiles, refFile)
				continue
			}
			// Other IO error — treat as missing.
			est.MissingFiles = append(est.MissingFiles, refFile)
			continue
		}

		fileCount++
		hasSignal = true
		bytes := info.Size()
		if bytes > 50000 {
			hasHugeFile = true
		}
		tokens := estimateFileTokens(path, bytes)
		totalTokens += tokens

		// Track local imports for Go files.
		if strings.HasSuffix(path, ".go") {
			localImportCount += countLocalImports(path)
		}

		// Check for unknown extension.
		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] && !proseExtensions[ext] {
			est.UnknownFiles = append(est.UnknownFiles, refFile)
		}
	}

	// Estimate tokens from description.
	if c.Description != "" {
		hasSignal = true
		totalTokens += len([]rune(c.Description)) / 4
	}

	// Estimate tokens from referenced sections.
	for _, section := range c.ReferencedSections {
		hasSignal = true
		path := section.Path
		if e.BaseDir != "" {
			path = filepath.Join(e.BaseDir, section.Path)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			// File missing — section also missing.
			est.MissingSections = append(est.MissingSections, section)
			continue
		}

		sectionTokens := estimateSectionTokens(content, section.Heading)
		if sectionTokens == -1 {
			// Heading not found.
			est.MissingSections = append(est.MissingSections, section)
		} else {
			totalTokens += sectionTokens
		}
	}

	// Build drivers.
	est.Drivers = append(est.Drivers, fmt.Sprintf("%d referenced files", fileCount))
	est.Drivers = append(est.Drivers, fmt.Sprintf("%d imported local packages", localImportCount))
	if c.Description != "" {
		est.Drivers = append(est.Drivers, fmt.Sprintf("description %d chars", len([]rune(c.Description))))
	}
	for _, missing := range est.MissingFiles {
		est.Drivers = append(est.Drivers, fmt.Sprintf("file not found: %s", missing))
	}
	for _, missing := range est.MissingSections {
		est.Drivers = append(est.Drivers, fmt.Sprintf("section heading not found: %s#%s", missing.Path, missing.Heading))
	}
	for _, unknown := range est.UnknownFiles {
		info, _ := os.Stat(filepath.Join(e.BaseDir, unknown))
		if info != nil {
			est.Drivers = append(est.Drivers, fmt.Sprintf("unknown extension: %s (%d bytes)", unknown, info.Size()))
		}
	}

	// Classify fit.
	est.Fit = classifyFit(totalTokens, budget)

	// Determine recommendation.
	est.Recommendation = classifyRecommendation(totalTokens, budget, est.Fit)

	// Determine confidence.
	est.Confidence = determineConfidence(est, fileCount, hasSignal, hasHugeFile)

	// Set estimated tokens before building candidates (they reference it).
	est.EstimatedTokens = totalTokens

	// Build split/merge candidates.
	est.SplitCandidates = buildSplitCandidates(est, c)
	est.MergeCandidates = buildMergeCandidates(est, budget)

	return est, nil
}

// estimateFileTokens returns token estimate for a file based on extension and size.
func estimateFileTokens(path string, bytes int64) int {
	ext := strings.ToLower(filepath.Ext(path))
	if codeExtensions[ext] {
		return int(bytes) / 3
	}
	if proseExtensions[ext] {
		return int(bytes) / 4
	}
	// Unknown extension: assume code-like density (conservative).
	return int(bytes) / 3
}

// estimateSectionTokens returns token estimate for a markdown section, or -1 if heading not found.
func estimateSectionTokens(content []byte, heading string) int {
	lines := strings.Split(string(content), "\n")
	found := false
	var sectionBytes int

	for i, line := range lines {
		matches := headingRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if matches[1] == heading {
			found = true
			// Count bytes from this heading to next heading at same/lower level or EOF.
			for j := i + 1; j < len(lines); j++ {
				nextMatch := headingRe.FindStringSubmatch(lines[j])
				if nextMatch != nil {
					// Check heading level (count # characters).
					currentLevel := 0
					for _, c := range line {
						if c == '#' {
							currentLevel++
						}
					}
					nextLevel := 0
					for _, c := range nextMatch[0] {
						if c == '#' {
							nextLevel++
						}
					}
					if nextLevel <= currentLevel {
						// Next heading at same or lower level — stop here.
						break
					}
				}
				sectionBytes += len(lines[j]) + 1 // +1 for newline
			}
			break
		}
	}

	if !found {
		return -1
	}

	// Sections are prose.
	return sectionBytes / 4
}

// countLocalImports counts import paths in a Go file.
func countLocalImports(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	// Simple heuristic: count import paths in import blocks.
	text := string(content)
	importCount := 0
	inImportBlock := false

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			// Single import: import "path"
			if !strings.Contains(trimmed, "(") {
				importCount++
				continue
			}
			inImportBlock = true
			continue
		}
		if inImportBlock {
			if strings.Contains(trimmed, ")") {
				inImportBlock = false
				continue
			}
			// Count this import line.
			importCount++
		}
	}

	return importCount
}

// classifyFit returns "within", "over", or "under".
func classifyFit(tokens int, budget Budget) string {
	if tokens >= budget.MinTokens && tokens <= budget.MaxTokens {
		return "within"
	}
	if tokens > budget.MaxTokens {
		return "over"
	}
	return "under"
}

// classifyRecommendation returns "keep", "split", or "merge_candidate".
func classifyRecommendation(tokens int, budget Budget, fit string) string {
	if fit == "over" {
		return "split"
	}
	if fit == "under" && tokens < budget.MinTokens*3/10 {
		return "merge_candidate"
	}
	return "keep"
}

// determineConfidence returns "high", "medium", or "low".
func determineConfidence(est ContextEstimate, fileCount int, hasSignal bool, hasHugeFile bool) string {
	confidence := "high"

	// Missing files → low.
	if len(est.MissingFiles) > 0 {
		return "low"
	}

	// Huge files → downgrade one level.
	if hasHugeFile {
		confidence = downgrade(confidence)
	}

	// Missing sections → downgrade one level.
	if len(est.MissingSections) > 0 {
		confidence = downgrade(confidence)
	}

	// Unknown extensions → downgrade one level.
	if len(est.UnknownFiles) > 0 {
		confidence = downgrade(confidence)
	}

	// No signal at all → low.
	if !hasSignal {
		return "low"
	}

	return confidence
}

// downgrade downgrades confidence by one level.
func downgrade(confidence string) string {
	switch confidence {
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "low"
	}
}

// buildSplitCandidates returns human-readable split hints.
func buildSplitCandidates(est ContextEstimate, c ContextCandidate) []string {
	candidates := []string{}
	if est.Fit != "over" {
		return candidates
	}

	// Simple heuristic: if multiple files, suggest package-based split.
	if len(c.ReferencedFiles) > 2 {
		// Count unique directory prefixes as a proxy for packages.
		packages := make(map[string]bool)
		for _, f := range c.ReferencedFiles {
			dir := filepath.Dir(f)
			packages[dir] = true
		}
		if len(packages) > 1 {
			candidates = append(candidates, fmt.Sprintf("Consider splitting: %d files touch different directories", len(c.ReferencedFiles)))
		}
	}

	return candidates
}

// buildMergeCandidates returns human-readable merge hints.
func buildMergeCandidates(est ContextEstimate, budget Budget) []string {
	candidates := []string{}
	if est.Fit != "under" || est.Recommendation != "merge_candidate" {
		return candidates
	}

	candidates = append(candidates, fmt.Sprintf("Consider merging: %d tokens, well under %d minimum budget", est.EstimatedTokens, budget.MinTokens))

	return candidates
}