# TODO 003 - Add `--serve` HTML graph viewer with expand/collapse

Goal: run a local HTTP server from `godecide` that serves an interactive DAG view (Cytoscape) with expand/collapse, force layout, and record-like node summaries, avoiding DOT parsing.

- [ ] 003.1 Define CLI flags and usage (`--serve`, `--addr`, optional `--open`); document in usage text.
- [ ] 003.2 Specify a compact UI JSON schema (nodes/edges/metrics/colors) and hyperedge representation (junction nodes).
- [ ] 003.3 Implement HTTP server with `net/http` and embedded UI assets (`//go:embed ui/*`).
- [ ] 003.4 Add `/graph` handler that emits the UI JSON from in-memory AST (no DOT parsing).
- [ ] 003.5 Build UI shell: Cytoscape + expand/collapse + force layout; add node labels (HTML or side panel).
- [ ] 003.6 Style mappings: dynamic colors, edge widths/labels, hover/selection highlighting.
- [ ] 003.7 Add basic tests for JSON shape and handler responses.
- [ ] 003.8 Update README with `--serve` usage and screenshots or notes.
