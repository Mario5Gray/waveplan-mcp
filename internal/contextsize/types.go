package contextsize

// SectionRef references a heading within a file.
type SectionRef struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
}

// ContextCandidate is a source-agnostic issue candidate for context estimation.
type ContextCandidate struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	Kind               string        `json:"kind,omitempty"`
	ReferencedFiles    []string      `json:"referenced_files"`
	ReferencedSections []SectionRef  `json:"referenced_sections"`
	DependsOn          []string      `json:"depends_on"`
	Source             string        `json:"source"`
}

// Budget defines the acceptable token range.
type Budget struct {
	MinTokens int
	MaxTokens int
}

// ContextEstimate is the output of context estimation.
type ContextEstimate struct {
	EstimatedTokens   int          `json:"estimated_tokens"`
	BudgetMin         int          `json:"budget_min"`
	BudgetMax         int          `json:"budget_max"`
	Fit               string       `json:"fit"`
	Confidence        string       `json:"confidence"`
	Drivers           []string     `json:"drivers"`
	Recommendation    string       `json:"recommendation"`
	MissingFiles      []string     `json:"missing_files"`
	MissingSections   []SectionRef `json:"missing_sections"`
	UnknownFiles      []string     `json:"unknown_files"`
	SplitCandidates   []string     `json:"split_candidates"`
	MergeCandidates   []string     `json:"merge_candidates"`
}