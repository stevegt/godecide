# Repository Guidelines

## Project Structure & Module Organization
- `cmd/godecide/` is the CLI entry point.
- Core library code lives at the repo root in `godecide.go` (package `tree`).
- Financial helpers and tests are under `fin/`.
- `examples/` holds sample YAML inputs embedded at build time; `testdata/` and `fin/testdata/` hold fixtures and golden outputs.
- `x/` is reserved for experimental prototypes; avoid `internal/` and `pkg/`.
- Local Grokker state like `.grok` and generated binaries should not be committed.

## Build, Test, and Development Commands
- `make` or `make godecide`: static build with external link flags (see `Makefile`).
- `go build ./cmd/godecide`: standard local build during development.
- `go test ./...`: run all tests.
- `go run ./cmd/godecide example:hbr stdout`: render an example to DOT on stdout; use `xdot` instead of `stdout` if installed.

## Coding Style & Naming Conventions
- Run `gofmt` on Go code; keep package names short and lowercase.
- Tests use `*_test.go`; prefer table-driven tests when multiple cases are similar.
- Keep new packages at the repo root or under `x/` only.

## Testing Guidelines
- Use Go’s standard `testing` package; keep tests deterministic and avoid network calls.
- Prefer fixtures in `testdata/` and `fin/testdata/`; add coverage alongside new features.

## TODO Tracking
- Track work in `TODO/` with `TODO.md` as the index.
- Number TODOs with 3 digits (e.g., `005`); do not renumber; sort `TODO.md` by priority.
- Use numbered subtasks like `005.1` inside each TODO file; mark completed items with `[x]`.

## Commit & Pull Request Guidelines
- Commit messages are short, imperative, and capitalized (e.g., "Add YAML validation").
- Include a commit body with per-file bullet summaries when changes are non-trivial.
- Use explicit `git add <path>` per file; avoid `git add .`.
- PRs should include a concise summary, tests run, linked issues, and before/after notes when behavior changes.
