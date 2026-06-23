# Codex Spend Monitor

Codex Spend Monitor is a Go + HTMX application that watches a Codex data directory, imports session JSONL files, and calculates API-equivalent spend from recorded token usage.

## Product Direction

- Track sessions from `sessions/**/*.jsonl` and `archived_sessions/**/*.jsonl`.
- Persist imported data in the app's own SQLite database so totals survive source file removal or archiving.
- Show dashboard totals by session, plus drilldown rows for token-count events that approximate individual API calls.
- Label calculated amounts as API-equivalent spend. Codex Desktop or subscription usage may not match actual OpenAI billing.
- Keep the Windows pinned daily-spend overlay as a phase-two feature, backed by the same stored daily totals.

## Implementation Defaults

- Use a compact layered Go service: HTTP handlers, ingestion, pricing, and SQLite storage stay in separate internal packages.
- Wire dependencies manually in `cmd/codex-spend-monitor`.
- Prefer `net/http`, `html/template`, `database/sql`, and focused helpers before adding framework abstractions.
- Use SQLite through a normal Go SQL driver, parameterized queries, and context-aware database calls.
- On startup, scan once. While running, poll incrementally every 10 seconds. Keep a manual rescan button in the UI.
- Default Codex path to `%USERPROFILE%\.codex` on Windows, but allow the web UI to update it.

## Required Go Skills

The following Go skills from `samber/cc-skills-golang` MUST always be applied when working on this project. Load them at the start of every Go-related task, regardless of whether the user explicitly mentions them.

- `samber/cc-skills-golang@golang-code-style`
- `samber/cc-skills-golang@golang-database`
- `samber/cc-skills-golang@golang-documentation`
- `samber/cc-skills-golang@golang-error-handling`
- `samber/cc-skills-golang@golang-naming`
- `samber/cc-skills-golang@golang-project-layout`
- `samber/cc-skills-golang@golang-safety`
- `samber/cc-skills-golang@golang-security`
- `samber/cc-skills-golang@golang-testing`

## Quality Bar

- Treat parsing as lossy-tolerant: malformed JSONL lines should be counted and skipped, not crash an import.
- Keep raw token-count event JSON in SQLite for future reprocessing.
- Unknown or local models are allowed in the UI but should be shown as unpriced until pricing is known.
- Tests should cover parser behavior, cost calculation, SQLite idempotency, and HTTP handler basics.
