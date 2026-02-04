# godecide

A scenario tree (real options, decision tree) tool. It reads YAML, forecasts cash flows over time, and renders a Graphviz DOT diagram for review.

## Install

```
go install -v github.com/stevegt/godecide@latest
```

## Quick start

```
godecide example:hbr stdout

godecide example:hbr xdot

godecide -tb -now=2026-01-01T00:00:00Z examples/college.yaml stdout
```

Inputs:
- `src`: `stdin`, `example:NAME`, or a filename
- `dst`: `stdout` (DOT), `xdot` (launch viewer), `yaml` (echo input), or a filename (writes DOT; if the file exists it is backed up to `/tmp` before overwrite)

Flags:
- `-tb` switches graph direction to top-to-bottom
- `-now` sets the evaluation timestamp (RFC3339)

Note: `xdot` requires the `xdot` viewer to be installed and on your PATH.

## YAML format

Each node is a YAML key with fields:
- `desc`: human-readable description
- `cash`: cash amount (supports math expressions)
- `days`: duration in days (supports math expressions)
- `repeat`: integer count of period repeats (minimum 1)
- `finrate`: finance/discount rate
- `rerate`: reinvestment rate
- `due`: optional due date (RFC3339)
- `paths`: map of child node names to probabilities; a key can be `a,b` to indicate a joint outcome

Example:
```
root:
  desc: "Decision root"
  cash: 0
  days: 0
  finrate: 0.12
  rerate: 0.18
  paths:
    option_a: 0.6
    option_b,option_c: 0.4

option_a:
  desc: "Single path"
  cash: -100000
  days: 90
```

## Theory: decision trees and real options

A scenario tree (decision tree) enumerates choices and uncertain outcomes as nodes and probabilistic branches. The tool evaluates cash flows along each path, computes expected values, and highlights tradeoffs across scenarios.

Real options treat managerial flexibility (defer, expand, abandon, switch) as options with value. This tool models those options as branches with timing and probabilities, then applies time value of money to compare paths and expected outcomes.

## Math and assumptions

Time is modeled as a timeline in days, using `365.2425` days per year. Each node contributes one or more cash flow events at the end of its period (repeated `repeat` times).

For each event `i` at time `t_i` (years since timeline start):

- NPV:
  `NPV = sum_i cash_i / (1 + finrate_i) ^ t_i`
- Present value of negatives:
  `PVneg = sum_{cash_i < 0} (-cash_i) / (1 + finrate_i) ^ t_i`
- Future value of positives at timeline end `T`:
  `FVpos = sum_{cash_i > 0} cash_i * (1 + rerate_i) ^ (T - t_i)`
- MIRR:
  `MIRR = (FVpos / PVneg) ^ (1 / T) - 1`

Notes:
- If a node has `due` and the computed end date exceeds it, a warning is emitted and MIRR is treated as invalid for that path.
- If child probabilities do not sum to 1.0, they are normalized.
