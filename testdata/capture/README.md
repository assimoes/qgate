# Capture sample

A small project with planted issues that `scripts/capture-fixtures.sh` runs real analyzers, linters, coverage tools and test runners against. Their output becomes the Seam C fixtures under `internal/ingest/*/testdata/`. It is never built as part of qgate (each language directory is its own project).

- `go/`: SQL built with `fmt.Sprintf` (Semgrep, CodeQL), an unchecked error and a complex function (golangci-lint), a vulnerable `golang.org/x/net` (Trivy), tests with a failure and a skip (coverage, go-junit-report).
- `js/`: an unused variable and a console call (ESLint), node:test tests (c8 lcov).
- `py/`: a view with a branch (coverage.py), tests with a setup error and a skip (pytest JUnit).
