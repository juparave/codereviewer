package domain

import "time"

// Report represents the daily code review report
type Report struct {
	Date          time.Time
	Summary       string
	Findings      []Finding
	Repositories  []string
	CommitCount   int
	FileCount     int
	NothingToNote bool
	Model         string // The LLM model used for review
}

// HighCount returns the number of high severity findings
func (r *Report) HighCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityHigh {
			count++
		}
	}
	return count
}

// MediumCount returns the number of medium severity findings
func (r *Report) MediumCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityMedium {
			count++
		}
	}
	return count
}

// LowCount returns the number of low severity findings
func (r *Report) LowCount() int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityLow {
			count++
		}
	}
	return count
}

// TotalFindings returns the total number of findings
func (r *Report) TotalFindings() int {
	return len(r.Findings)
}

// HasFindings returns true if there are any findings
func (r *Report) HasFindings() bool {
	return len(r.Findings) > 0
}
