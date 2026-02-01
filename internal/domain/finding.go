package domain

// Severity represents the importance level of a finding
type Severity string

const (
	SeverityHigh   Severity = "High"
	SeverityMedium Severity = "Medium"
	SeverityLow    Severity = "Low"
)

// Finding represents an issue discovered during code review
type Finding struct {
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	RepoName    string   `json:"repo_name"`
	Files       []string `json:"files"`
	Explanation string   `json:"explanation"`
	Action      string   `json:"suggested_action"`
}

// IsHighPriority returns true if the finding is high severity
func (f *Finding) IsHighPriority() bool {
	return f.Severity == SeverityHigh
}
