package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/juparave/codereviewer/internal/config"
	"github.com/juparave/codereviewer/internal/domain"
	"github.com/openai/openai-go/option"
)

// ReviewOutput is the structured output from the LLM
type ReviewOutput struct {
	Summary  string           `json:"summary"`
	Findings []domain.Finding `json:"findings"`
}

// Reviewer performs code review using an LLM
type Reviewer struct {
	config  config.ReviewConfig
	logger  *log.Logger
	genkit  *genkit.Genkit
	modelID string
}

// NewReviewer creates a new Reviewer
func NewReviewer(cfg config.ReviewConfig, logger *log.Logger) (*Reviewer, error) {
	ctx := context.Background()

	var g *genkit.Genkit
	var modelID string

	switch cfg.Provider {
	case "openai":
		// OpenAI-compatible API (Zhipu AI, etc.)
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("ZHIPU_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("OPENAI_API_KEY")
			}
		}

		// Build options for custom base URL
		var opts []option.RequestOption
		if cfg.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(cfg.BaseURL))
		}

		plugin := &oai.OpenAI{
			APIKey: apiKey,
			Opts:   opts,
		}

		modelID = cfg.Model
		if modelID == "" {
			modelID = "glm-4.7"
		}
		// Prefix with openai/ for Genkit
		if !strings.Contains(modelID, "/") {
			modelID = "openai/" + modelID
		}

		g = genkit.Init(ctx,
			genkit.WithDefaultModel(modelID),
			genkit.WithPlugins(plugin),
		)

	case "googleai":
		fallthrough
	default:
		// Google AI (Gemini)
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("GOOGLE_API_KEY")
			}
		}

		modelID = cfg.Model
		if modelID == "" {
			modelID = "gemini-2.0-flash"
		}
		// Prefix with googleai/ for Genkit
		if !strings.Contains(modelID, "/") {
			modelID = "googleai/" + modelID
		}

		g = genkit.Init(ctx,
			genkit.WithDefaultModel(modelID),
			genkit.WithPlugins(&googlegenai.GoogleAI{
				APIKey: apiKey,
			}),
		)
	}

	return &Reviewer{
		config:  cfg,
		logger:  logger,
		genkit:  g,
		modelID: modelID,
	}, nil
}

// Review analyzes diffs and returns findings
func (r *Reviewer) Review(ctx context.Context, diffs []domain.Diff) ([]domain.Finding, string, error) {
	if len(diffs) == 0 {
		return nil, "No changes to review.", nil
	}

	// Build the prompt
	prompt := r.buildPrompt(diffs)

	// Generate response
	answer, err := genkit.GenerateText(ctx, r.genkit,
		ai.WithModelName(r.modelID),
		ai.WithPrompt(prompt),
	)
	if err != nil {
		return nil, "", fmt.Errorf("generating review: %w", err)
	}

	// Parse the response
	output, err := r.parseResponse(answer)
	if err != nil {
		return nil, "", fmt.Errorf("parsing response: %w", err)
	}

	return output.Findings, output.Summary, nil
}

func (r *Reviewer) buildPrompt(diffs []domain.Diff) string {
	var sb strings.Builder

	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("## Code Changes to Review\n\n")

	for _, d := range diffs {
		sb.WriteString(fmt.Sprintf("### Repository: %s\n", d.RepoName))
		sb.WriteString(fmt.Sprintf("### File: %s (%s)\n", d.FilePath, d.Language))
		sb.WriteString("```diff\n")
		sb.WriteString(d.Content)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString(outputInstructions)

	return sb.String()
}

func (r *Reviewer) parseResponse(text string) (*ReviewOutput, error) {
	// Try to find JSON in the response
	text = strings.TrimSpace(text)

	// Handle markdown code blocks
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	}

	text = strings.TrimSpace(text)

	var output ReviewOutput
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w\nResponse was: %s", err, text)
	}

	return &output, nil
}

const systemPrompt = `You are a senior software engineer performing a daily code review. Your role is to identify meaningful issues that matter for production code quality.

## Your Review Principles

1. **Signal over noise** – Only flag issues that genuinely matter. If the code looks fine, say so.
2. **Mentor mindset** – Explain why something is a problem, not just that it is one.
3. **Context-aware** – Consider the language, framework, and apparent intent.
4. **No nitpicking** – Ignore formatting, naming style, and minor preferences.
5. **Evidence-based** – Only flag issues you can point to in the code.

## What to Look For

- **Bugs**: Logic errors, edge cases, null/nil handling, race conditions
- **Security**: Injection risks, auth issues, sensitive data exposure
- **Data integrity**: Missing validation, transaction issues, constraint violations
- **Design**: Architectural problems, tight coupling, missing abstractions
- **Performance**: Obvious inefficiencies, N+1 queries, memory leaks

## What to Ignore

- Formatting and whitespace
- Naming conventions (unless truly confusing)
- Missing comments (unless logic is complex)
- Speculative future problems
- Style preferences`

const outputInstructions = `
## Required Output Format

Respond with a JSON object in this exact format:

{
  "summary": "One or two sentence summary of the changes reviewed",
  "findings": [
    {
      "title": "Brief issue title",
      "severity": "High|Medium|Low",
      "repo_name": "repository-name",
      "files": ["file1.go", "file2.go"],
      "explanation": "Why this is a problem and what could go wrong",
      "suggested_action": "Specific recommendation to fix the issue"
    }
  ]
}

If no meaningful issues are found, return:
{
  "summary": "Summary of changes reviewed",
  "findings": []
}

Respond ONLY with the JSON object, no additional text.`
