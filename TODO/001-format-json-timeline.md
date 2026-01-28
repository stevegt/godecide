# TODO 001 - Add `--format json --include timeline` report output

Goal: extend `godecide` reporting to emit a machine-readable JSON report that can optionally include full timeline events, enabling downstream Markdown/report generation.

- [ ] 001.1 Define the JSON report schema (nodes, stats, hyperedges) and add an optional `timeline` block.
- [ ] 001.2 Add CLI flags `--format json` and `--include timeline` (or `--include=timeline`) and wire them into output selection.
- [ ] 001.3 Serialize `Ast`/`Stats` into JSON with stable ordering; include `meta` (generated_at, now, roots).
- [ ] 001.4 When timeline is included, emit per-node events with `date`, `cash`, `finrate`, `rerate`, and computed `years_elapsed/years_left`.
- [ ] 001.5 Add tests covering schema shape, required fields, and timeline inclusion/exclusion.
- [ ] 001.6 Update README with an example JSON snippet and CLI usage.
