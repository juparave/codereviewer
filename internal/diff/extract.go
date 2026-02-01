package diff

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juparave/codereviewer/internal/domain"
	"github.com/juparave/codereviewer/internal/scanner"
)

// Extractor extracts and filters diffs from commits
type Extractor struct {
	logger *log.Logger
}

// NewExtractor creates a new Extractor
func NewExtractor(logger *log.Logger) *Extractor {
	return &Extractor{logger: logger}
}

// Extract extracts diffs from a commit, filtering to supported file types
func (e *Extractor) Extract(ctx context.Context, commit domain.Commit) ([]domain.Diff, error) {
	// Get changed files
	files, err := e.getChangedFiles(ctx, commit.RepoPath, commit.Hash)
	if err != nil {
		return nil, err
	}

	var diffs []domain.Diff
	for _, file := range files {
		// Check if file extension is supported
		ext := filepath.Ext(file)
		lang, ok := domain.SupportedExtensions[ext]
		if !ok {
			continue
		}

		// Skip excluded paths
		if e.shouldExclude(file) {
			continue
		}

		// Get diff for this file
		content, err := e.getFileDiff(ctx, commit.RepoPath, commit.Hash, file)
		if err != nil {
			e.logger.Printf("Warning: failed to get diff for %s: %v", file, err)
			continue
		}

		// Count lines and truncate if needed
		lines := strings.Split(content, "\n")
		lineCount := len(lines)
		if lineCount > domain.MaxDiffLines {
			content = strings.Join(lines[:domain.MaxDiffLines], "\n")
			content += "\n... [truncated]"
		}

		diffs = append(diffs, domain.Diff{
			FilePath:   file,
			Content:    content,
			LineCount:  lineCount,
			CommitHash: commit.Hash,
			RepoPath:   commit.RepoPath,
			RepoName:   scanner.GetRepoName(commit.RepoPath),
			Language:   lang,
		})
	}

	return diffs, nil
}

// shouldExclude checks if a file path should be excluded
func (e *Extractor) shouldExclude(path string) bool {
	excludePaths := []string{
		"vendor/",
		"node_modules/",
		".gen.",
		".generated.",
		"_generated",
		".pb.go",
		".mock.go",
		"_mock.go",
		"mocks/",
		"testdata/",
	}

	for _, exclude := range excludePaths {
		if strings.Contains(path, exclude) {
			return true
		}
	}

	return false
}

func (e *Extractor) getChangedFiles(ctx context.Context, repoPath, commitHash string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "show",
		"--format=",
		"--name-status",
		commitHash,
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	s := bufio.NewScanner(bytes.NewReader(output))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		// Format: "M\tfilename" or "A\tfilename" etc.
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			// Skip deleted files
			if parts[0] != "D" {
				files = append(files, parts[len(parts)-1])
			}
		}
	}

	return files, s.Err()
}

func (e *Extractor) getFileDiff(ctx context.Context, repoPath, commitHash, filePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "show",
		"--format=",
		"--patch",
		"--no-color",
		commitHash,
		"--",
		filePath,
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}
