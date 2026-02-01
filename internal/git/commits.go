package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/juparave/codereviewer/internal/domain"
	"github.com/juparave/codereviewer/internal/scanner"
)

// Client interacts with Git repositories
type Client struct {
	logger *log.Logger
}

// NewClient creates a new Git client
func NewClient(logger *log.Logger) *Client {
	return &Client{logger: logger}
}

// GetTodaysCommits returns commits made today in the given repository
func (c *Client) GetTodaysCommits(ctx context.Context, repoPath string) ([]domain.Commit, error) {
	// Get today's date at midnight in local timezone
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	since := midnight.Format("2006-01-02T15:04:05")

	// Git log format: hash|author|email|timestamp|subject
	format := "%H|%an|%ae|%aI|%s"

	cmd := exec.CommandContext(ctx, "git", "log",
		"--since="+since,
		"--no-merges",
		"--format="+format,
		"--all",
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// Check if it's just an empty repo or no commits
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "does not have any commits") {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	return c.parseCommits(output, repoPath)
}

func (c *Client) parseCommits(output []byte, repoPath string) ([]domain.Commit, error) {
	var commits []domain.Commit
	repoName := scanner.GetRepoName(repoPath)

	s := bufio.NewScanner(bytes.NewReader(output))
	for s.Scan() {
		line := s.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339, parts[3])
		if err != nil {
			c.logger.Printf("Warning: failed to parse timestamp %s: %v", parts[3], err)
			continue
		}

		commits = append(commits, domain.Commit{
			Hash:      parts[0],
			Author:    parts[1],
			Email:     parts[2],
			Timestamp: timestamp,
			Message:   parts[4],
			RepoPath:  repoPath,
			RepoName:  repoName,
		})
	}

	return commits, s.Err()
}

// GetDiff returns the diff for a specific commit
func (c *Client) GetDiff(ctx context.Context, repoPath, commitHash string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "show",
		"--format=",
		"--patch",
		"--no-color",
		commitHash,
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show failed: %w", err)
	}

	return string(output), nil
}

// GetChangedFiles returns the list of files changed in a commit
func (c *Client) GetChangedFiles(ctx context.Context, repoPath, commitHash string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "show",
		"--format=",
		"--name-only",
		commitHash,
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show --name-only failed: %w", err)
	}

	var files []string
	s := bufio.NewScanner(bytes.NewReader(output))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	return files, s.Err()
}

// GetFileDiff returns the diff for a specific file in a commit
func (c *Client) GetFileDiff(ctx context.Context, repoPath, commitHash, filePath string) (string, error) {
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
		return "", fmt.Errorf("git show for file failed: %w", err)
	}

	return string(output), nil
}

// IsValidRepo checks if a path is a valid Git repository
func IsValidRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}
