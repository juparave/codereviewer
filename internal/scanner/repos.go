package scanner

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ExcludedDirs are directories to skip during scanning
var ExcludedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
}

// Scanner finds Git repositories in a directory tree
type Scanner struct {
	logger *log.Logger
}

// New creates a new Scanner
func New(logger *log.Logger) *Scanner {
	return &Scanner{logger: logger}
}

// FindRepositories recursively finds all Git repositories under rootPath
func (s *Scanner) FindRepositories(rootPath string) ([]string, error) {
	var repos []string

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		// Skip hidden directories (except .git which we're looking for)
		name := d.Name()
		if strings.HasPrefix(name, ".") && name != ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip excluded directories
		if d.IsDir() && ExcludedDirs[name] {
			return filepath.SkipDir
		}

		// Check if this is a .git directory
		if d.IsDir() && name == ".git" {
			repoPath := filepath.Dir(path)
			repos = append(repos, repoPath)
			return filepath.SkipDir // Don't descend into .git
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return repos, nil
}

// GetRepoName extracts the repository name from its path
func GetRepoName(repoPath string) string {
	return filepath.Base(repoPath)
}
