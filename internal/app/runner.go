package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/juparave/codereviewer/internal/config"
	"github.com/juparave/codereviewer/internal/diff"
	"github.com/juparave/codereviewer/internal/domain"
	"github.com/juparave/codereviewer/internal/git"
	"github.com/juparave/codereviewer/internal/notify"
	"github.com/juparave/codereviewer/internal/report"
	"github.com/juparave/codereviewer/internal/review"
	"github.com/juparave/codereviewer/internal/scanner"
)

// Runner orchestrates the full code review flow
type Runner struct {
	config  *config.Config
	logger  *log.Logger
	scanner *scanner.Scanner
	git     *git.Client
	diff    *diff.Extractor
	review  *review.Reviewer
	report  *report.Formatter
	notify  *notify.Service
}

// NewRunner creates a new Runner instance
func NewRunner(cfg *config.Config) *Runner {
	logger := log.New(os.Stdout, "[CRA] ", log.LstdFlags)

	return &Runner{
		config:  cfg,
		logger:  logger,
		scanner: scanner.New(logger),
		git:     git.NewClient(logger),
		diff:    diff.NewExtractor(logger),
		report:  report.NewFormatter(cfg.Reports.OutputDir),
		// review and notify initialized in Run() after validation
	}
}

// Run executes the full review pipeline
func (r *Runner) Run(ctx context.Context) error {
	startTime := time.Now()

	// Validate configuration
	if err := r.config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	r.log("Starting code review for %s", r.config.RootPath)

	// Step 1: Scan for repositories
	r.log("Scanning for Git repositories...")
	repos, err := r.scanner.FindRepositories(r.config.RootPath)
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}
	r.log("Found %d repositories", len(repos))

	if len(repos) == 0 {
		r.log("No repositories found, nothing to review")
		return nil
	}

	// Step 2: Find today's commits
	r.log("Finding today's commits...")
	var allCommits []domain.Commit
	for _, repoPath := range repos {
		commits, err := r.git.GetTodaysCommits(ctx, repoPath)
		if err != nil {
			r.log("Warning: failed to get commits from %s: %v", repoPath, err)
			continue
		}
		allCommits = append(allCommits, commits...)
	}
	r.log("Found %d commits from today", len(allCommits))

	if len(allCommits) == 0 {
		r.log("No commits today, nothing to review")
		return r.handleNoFindings(ctx)
	}

	// Step 3: Extract diffs
	r.log("Extracting diffs...")
	var allDiffs []domain.Diff
	for _, commit := range allCommits {
		diffs, err := r.diff.Extract(ctx, commit)
		if err != nil {
			r.log("Warning: failed to extract diff for %s: %v", commit.Hash[:8], err)
			continue
		}
		allDiffs = append(allDiffs, diffs...)
	}
	r.log("Extracted %d file diffs", len(allDiffs))

	if len(allDiffs) == 0 {
		r.log("No relevant diffs found, nothing to review")
		return r.handleNoFindings(ctx)
	}

	// Step 4: Initialize reviewer and perform review
	r.log("Initializing LLM reviewer...")
	reviewer, err := review.NewReviewer(r.config.Review, r.logger)
	if err != nil {
		return fmt.Errorf("initializing reviewer: %w", err)
	}
	r.review = reviewer

	r.log("Reviewing code changes...")
	findings, summary, err := r.review.Review(ctx, allDiffs)
	if err != nil {
		return fmt.Errorf("reviewing code: %w", err)
	}
	r.log("Found %d issues", len(findings))

	// Step 5: Generate report
	r.log("Generating report...")
	rpt := &domain.Report{
		Date:         time.Now(),
		Summary:      summary,
		Findings:     findings,
		Repositories: repos,
		CommitCount:  len(allCommits),
		FileCount:    len(allDiffs),
	}

	reportPath, err := r.report.Write(rpt)
	if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	r.log("Report saved to %s", reportPath)

	// Step 6: Send email notification
	if r.config.Email.Enabled && rpt.HasFindings() {
		r.log("Sending email notification...")
		notifier, err := notify.NewService(r.config.Email, r.logger)
		if err != nil {
			return fmt.Errorf("initializing email service: %w", err)
		}
		r.notify = notifier

		if err := r.notify.SendReport(ctx, rpt); err != nil {
			return fmt.Errorf("sending email: %w", err)
		}
		r.log("Email sent successfully")
	}

	elapsed := time.Since(startTime)
	r.log("Review complete in %s", elapsed.Round(time.Millisecond))

	return nil
}

func (r *Runner) handleNoFindings(ctx context.Context) error {
	rpt := &domain.Report{
		Date:          time.Now(),
		Summary:       "No code changes to review today.",
		NothingToNote: true,
	}

	reportPath, err := r.report.Write(rpt)
	if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	r.log("Report saved to %s", reportPath)

	return nil
}

func (r *Runner) log(format string, args ...interface{}) {
	if r.config.Verbose {
		r.logger.Printf(format, args...)
	}
}
