# TODO 002 - Add Markdown renderer that consumes the JSON report

Goal: generate a long-form narrative Markdown report using the JSON output so reports are accurate, reproducible, and human-readable.

- [ ] 002.1 Define report sections (summary, decision options, key risk gates, top paths, node table, appendix).
- [ ] 002.2 Implement a renderer that reads the JSON report and emits Markdown (stable ordering, configurable depth/top-N).
- [ ] 002.3 Add CLI entry point (e.g., `godecide report --format md --from json`) or a small helper tool under `cmd/`.
- [ ] 002.4 Include formatting options: `--depth`, `--top`, `--min-prob`, `--show-notes`, `--include-timeline`.
- [ ] 002.5 Add fixtures and tests to verify deterministic output.
- [ ] 002.6 Document usage and provide a sample report snippet.
