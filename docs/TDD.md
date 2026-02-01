
cra/
├── cmd/
│   └── review/
│       └── main.go          # CLI entrypoint (cobra)
│
├── internal/
│   ├── app/
│   │   └── runner.go        # Orchestrates the full review flow
│   │
│   ├── config/
│   │   └── config.go        # Config loading & validation
│   │
│   ├── domain/
│   │   ├── commit.go        # Commit model
│   │   ├── diff.go          # Diff model
│   │   ├── finding.go       # Review finding model
│   │   └── report.go        # Review report model
│   │
│   ├── scanner/
│   │   └── repos.go         # Finds git repositories
│   │
│   ├── git/
│   │   └── commits.go       # Git log + metadata
│   │
│   ├── diff/
│   │   └── extract.go       # Diff extraction & filtering
│   │
│   ├── review/
│   │   ├── prompt.go        # Prompt templates
│   │   ├── reviewer.go     # Review engine (LLM interface)
│   │   └── provider.go     # OpenAI / local model adapters
│   │
│   ├── report/
│   │   ├── formatter.go    # Markdown formatting
│   │   └── writer.go       # File persistence
│   │
│   ├── notify/
│   │   └── email.go        # Email delivery
│   │
│   ├── ui/
│   │   └── tui.go           # Charm / Bubble Tea UI
│   │
│   └── util/
│       └── fs.go            # Small shared helpers
│
├── reports/
│   └── 2026-01-31.md
│
├── go.mod
├── go.sum
└── README.md
