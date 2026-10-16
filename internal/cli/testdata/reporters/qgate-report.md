## qgate: FAIL

Stage **dev** · blocking enforcement · gating scope diff

### Scores

| Category | Score |
|---|---|
| **overall** | 62 |
| dependencies | not assessed |
| maintainability | not assessed |
| quality | 80 |
| security | 50 |
| testing | not assessed |

### Failed gate checks

| Check | Expected | Actual |
|---|---|---|
| `category_min.security` | &gt;= 90 | 50 |
| `max_new.critical` | &lt;= 0 | 1 |

### New findings

| Critical | High | Medium | Low | Info |
|---|---|---|---|---|
| 1 | 1 | 1 | 1 | 0 |

### Top findings

| Severity | Rule | Location | Title |
|---|---|---|---|
| critical | `semgrep:sqli` | `a.go:5` | title of sqli |
| high | `lint:unused` | `new.go:2` | title of unused |
| medium | `semgrep:weak-hash` | `a.go:5` | title of weak-hash |
| low | `semgrep:todo` | `new.go:1` | title of todo |

<sub>qgate dev · full report: `qgate-report.json`</sub>
