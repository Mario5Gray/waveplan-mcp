package contextsize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	var hasSignal bool
	var hasHugeFile bool
	localImports := map[string]struct{}{}
	fileSizes := map[string]int64{}
	modulePath, err := findModulePath(e.BaseDir)
	if err != nil {
		return ContextEstimate{}, err
	}

	// Estimate tokens from referenced files.
	for _, refFile := range c.ReferencedFiles {
		path := refFile
		if e.BaseDir != "" {
			path = filepath.Join(e.BaseDir, refFile)
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				est.MissingFiles = appendUniqueString(est.MissingFiles, refFile)
				continue
			}
			return ContextEstimate{}, fmt.Errorf("stat %s: %w", refFile, err)
		}

		fileCount++
		hasSignal = true
		bytes := info.Size()
		fileSizes[refFile] = bytes
		if bytes > 50000 {
			hasHugeFile = true
		}
		tokens := estimateFileTokens(path, bytes)
		totalTokens += tokens

		// Track local imports for Go files.
		if strings.HasSuffix(path, ".go") && modulePath != "" {
			imports, err := collectLocalImports(path, modulePath)
			if err != nil {
				return ContextEstimate{}, err
			}
			for _, importPath := range imports {
				localImports[importPath] = struct{}{}
			}
		}

		// Check for unknown extension.
		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] && !proseExtensions[ext] {
			est.UnknownFiles = appendUniqueString(est.UnknownFiles, refFile)
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
			if os.IsNotExist(err) {
				est.MissingFiles = appendUniqueString(est.MissingFiles, section.Path)
				continue
			}
			return ContextEstimate{}, fmt.Errorf("read section file %s: %w", section.Path, err)
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				est.MissingFiles = appendUniqueString(est.MissingFiles, section.Path)
				continue
			}
			return ContextEstimate{}, fmt.Errorf("stat section file %s: %w", section.Path, err)
		}
		if _, seen := fileSizes[section.Path]; !seen {
			fileSizes[section.Path] = info.Size()
			if info.Size() > 50000 {
				hasHugeFile = true
			}
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
	est.Drivers = append(est.Drivers, fmt.Sprintf("%d imported local packages", len(localImports)))
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
	est.Confidence = determineConfidence(est, hasSignal, hasHugeFile)

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
	lines := strings.SplitAfter(string(content), "\n")
	start := -1
	end := len(content)
	offset := 0
	currentLevel := 0

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\n")
		matches := headingRe.FindStringSubmatch(line)
		if matches != nil {
			level := headingLevel(line)
			if start == -1 && matches[1] == heading {
				start = offset
				currentLevel = level
			} else if start != -1 && level <= currentLevel {
				end = offset
				break
			}
		}
		offset += len(rawLine)
	}

	if start == -1 {
		return -1
	}

	// Sections are prose.
	return (end - start) / 4
}

// collectLocalImports returns unique local import paths referenced by a Go file.
func collectLocalImports(path string, modulePath string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go file %s: %w", path, err)
	}

	text := string(content)
	inImportBlock := false
	seen := map[string]struct{}{}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			if !strings.Contains(trimmed, "(") {
				if importPath, ok := quotedImportPath(trimmed); ok && strings.HasPrefix(importPath, modulePath) {
					seen[importPath] = struct{}{}
				}
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
			if importPath, ok := quotedImportPath(trimmed); ok && strings.HasPrefix(importPath, modulePath) {
				seen[importPath] = struct{}{}
			}
		}
	}

	imports := make([]string, 0, len(seen))
	for importPath := range seen {
		imports = append(imports, importPath)
	}
	slices.Sort(imports)
	return imports, nil
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
func determineConfidence(est ContextEstimate, hasSignal bool, hasHugeFile bool) string {
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

func findModulePath(baseDir string) (string, error) {
	start := baseDir
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		start = cwd
	}

	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve base dir %s: %w", start, err)
	}

	for {
		goModPath := filepath.Join(current, "go.mod")
		data, err := os.ReadFile(goModPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(trimmed, "module ")), nil
				}
			}
			return "", nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", goModPath, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil
		}
		current = parent
	}
}

func quotedImportPath(line string) (string, bool) {
	start := strings.IndexByte(line, '"')
	if start == -1 {
		return "", false
	}
	end := strings.IndexByte(line[start+1:], '"')
	if end == -1 {
		return "", false
	}
	return line[start+1 : start+1+end], true
}

func headingLevel(line string) int {
	level := 0
	for _, c := range line {
		if c != '#' {
			break
		}
		level++
	}
	return level
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
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
