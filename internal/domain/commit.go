package domain

import "time"

// Commit represents a Git commit
type Commit struct {
	Hash      string
	Author    string
	Email     string
	Timestamp time.Time
	Message   string
	RepoPath  string
	RepoName  string
}

// IsToday checks if the commit was made today
func (c *Commit) IsToday() bool {
	now := time.Now()
	return c.Timestamp.Year() == now.Year() &&
		c.Timestamp.YearDay() == now.YearDay()
}
