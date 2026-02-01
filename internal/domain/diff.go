package domain

// Diff represents a code diff from a commit
type Diff struct {
	FilePath   string
	OldPath    string // For renames
	Content    string
	LineCount  int
	IsNew      bool
	IsDeleted  bool
	IsRenamed  bool
	CommitHash string
	RepoPath   string
	RepoName   string
	Language   string
}

// MaxDiffLines is the maximum number of lines to include per file
const MaxDiffLines = 300

// SupportedExtensions lists file extensions we analyze
var SupportedExtensions = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".dart": "dart",
	".sql":  "sql",
}

// IsTruncated returns true if the diff exceeds max lines
func (d *Diff) IsTruncated() bool {
	return d.LineCount > MaxDiffLines
}
