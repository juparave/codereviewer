# Code Review Agent (CRA)

## 1. Overview

The Code Review Agent (CRA) is a CLI-based autonomous tool written in Go that performs a nightly review of code changes made across multiple Git repositories. Its purpose is to act as a **personal senior engineer**, identifying only *meaningful* issues and opportunities in the day’s work and delivering a concise, actionable report via email each morning.

The agent prioritizes **signal over noise**, avoids nitpicking, and adapts to the developer’s stack and preferences.

---

## 2. Problem Statement

Daily development across multiple repositories makes it easy to miss:

* subtle bugs
* architectural drift
* growing technical debt
* inconsistent practices across projects

Traditional linters and CI checks focus on syntax and style, while human code reviews are limited by time and context. CRA fills this gap by providing an **asynchronous, contextual, design-focused review** without interrupting the developer’s workflow.

---

## 3. Goals & Non-Goals

### 3.1 Goals

* Automatically detect Git commits made during the current day
* Review only meaningful code changes
* Identify high-impact risks, bugs, and design issues
* Generate a short, prioritized daily report
* Deliver the report via email at a scheduled time (default: 08:00)
* Run unattended (cron / launchd friendly)
* Ship as a single Go binary

### 3.2 Non-Goals (MVP)

* Interactive code review or inline comments
* Full static analysis or lint replacement
* PR-level CI integration
* Real-time notifications
* Team or multi-user support

---

## 4. Target User

* Senior or mid-level individual developer
* Works across multiple repositories
* Uses Go, Angular, Flutter, backend APIs, and SQL
* Values architectural consistency and long-term maintainability
* Wants minimal noise and high trust in automated feedback

---

## 5. Key Principles

1. **Signal over noise** – If nothing meaningful is found, say so.
2. **Mentor mindset** – Act like a senior engineer, not a linter.
3. **Context-aware** – Respect stack, conventions, and intent.
4. **Concise output** – One page or less, always.
5. **Deterministic structure** – Stable, predictable report format.

---

## 6. User Experience

### 6.1 CLI Usage

```bash
review --root ~/projects
```

Runs unattended. No interaction required.

### 6.2 Optional TUI (Charm / Bubble Tea)

A minimal status-only interface displaying progress:

* Scanning repositories
* Finding commits
* Extracting diffs
* Reviewing code
* Generating report
* Scheduling email

No configuration or navigation in MVP.

---

## 7. Functional Requirements

### 7.1 Repository Discovery

* Recursively scan a configured root path
* Detect valid Git repositories
* Exclude hidden and ignored directories

### 7.2 Commit Detection

* Identify commits made during the current day
* Group commits by repository
* Ignore merge commits and trivial commits where possible

### 7.3 Diff Extraction

* Extract diffs only for relevant files
* Supported file types (MVP):

  * `.go`, `.ts`, `.dart`, `.sql`
* Exclude:

  * `vendor/`
  * `node_modules/`
  * generated files
* Limit diff size per file (~300 lines)

### 7.4 Code Review Analysis

#### Analysis Strategy

Multi-pass reasoning:

1. Change summarization
2. Risk detection (bugs, data integrity, security)
3. Design & maintainability review
4. Stack-specific best practices

#### Constraints

* No formatting or naming nitpicks
* No speculative issues without evidence
* Explicitly state when no issues are found

### 7.5 LLM Prompting

The agent must use a fixed system prompt and a structured user prompt to ensure consistent output.

#### Output Format (Required)

* Summary (1–2 sentences)
* Findings list (ranked)
* Each finding includes:

  * Title
  * Severity (High / Medium / Low)
  * Repo
  * File(s)
  * Explanation
  * Suggested action

### 7.6 Report Generation

* Generate a Markdown report per run
* Filename: `YYYY-MM-DD.md`
* Store locally for future reference

### 7.7 Email Delivery

* Send the generated report via email
* Default delivery time: 08:00 local time
* Email subject includes date and number of findings
* SMTP or Gmail API (implementation detail)

---

## 8. Non-Functional Requirements

* Single binary deployment
* Fast startup and execution
* Safe to run nightly
* Graceful failure (partial report if needed)
* Provider-agnostic LLM integration

---

## 9. Technical Architecture (MVP)

```
cra/
├── cmd/
│   └── review/
│       └── main.go
├── internal/
│   ├── config/
│   ├── scanner/
│   ├── git/
│   ├── diff/
│   ├── review/
│   ├── report/
│   └── ui/
└── go.mod
```

### Key Components

* **scanner**: Finds Git repositories
* **git**: Extracts commits and metadata
* **diff**: Filters and truncates diffs
* **review**: Builds prompt and calls LLM
* **report**: Formats and saves output
* **ui**: Optional Charm TUI

---

## 10. Configuration (MVP)

```yaml
root_path: ~/projects
email:
  enabled: true
  send_at: "08:00"
review:
  strictness: medium
```

---

## 11. Success Metrics

* Daily reports are read consistently
* Low false-positive rate
* No repeated or obvious feedback
* Developer trust increases over time

---

## 12. Risks & Mitigations

| Risk                   | Mitigation                            |
| ---------------------- | ------------------------------------- |
| Noisy feedback         | Strict filtering + severity threshold |
| Large diffs            | Hard diff size limits                 |
| Overgeneral LLM advice | Strong system prompt constraints      |
| Email fatigue          | Send nothing when appropriate         |

---

## 13. Future Enhancements (Post-MVP)

* Persistent memory and feedback learning
* Finding scoring and trend detection
* Weekly summary reports
* Go/Angular-specific rule packs
* Test coverage awareness
* Commit intent detection
* Multi-project prioritization

---

## 14. MVP Definition of Done

* CLI runs successfully unattended
* Detects same-day commits across repos
* Generates a concise Markdown report
* Sends a readable, useful email
* Produces zero output when nothing matters

## 15. Engineering Structure 

code-review-agent/
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


---

**CRA is not a linter.**

It is a calm, opinionated, senior engineer reviewing your work once per day — so you can focus on building.
