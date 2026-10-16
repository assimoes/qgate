# Coding standards

Judgement rules a reviewer applies to a diff. Mechanical rules live in tooling instead: `scripts/check.sh` runs gofmt, goimports, vet, staticcheck, errcheck, errorlint, nilerr, revive (doc comments on exported identifiers), unparam and `go test -race`, so a green check settles those. Implementation choices logged in `docs/DECISIONS.md` or `docs/adr/` are deliberate; review them as decisions, not as violations.

## Contract

- `docs/design.md` is binding where it says MUST. A change that departs from it updates the design section and logs the reason in `docs/DECISIONS.md` in the same diff.
- Names come from `GLOSSARY.md`: identifiers, comments, log messages, report fields and docs use its terms, not the synonyms it lists under _Avoid_.

## Determinism and reproducibility

- **Canonical order.** Every list qgate emits (findings, metrics, gate checks, warnings, merged sources) is in an order defined by its content. Map iteration is sorted; results gathered concurrently are reordered before anything downstream sees them. Changing the order of inputs in the config or of results in an analyzer report leaves the report unchanged.
- **Reproducible decisions.** Everything `pipeline.Decide` depends on is recorded in the report, so `score --report` reaches the same scores and gate from the report alone, on any build. A new input to the decision is a new report field.
- **Pure core.** Scoring, gate evaluation and everything from normalization step 7 on take their inputs as arguments: no I/O, clock, randomness or package state.

## Extension points

qgate grows by adding code. Each axis of variation is an interface plus a registry, wired at the CLI composition root:

| Axis | Extension point |
|---|---|
| Analyzer report formats | `ingest.Ingestor`, registered by format name |
| LLM providers | `llm.Provider`, registered by `type` |
| VCS platforms | `vcs.Client`, registered by platform |
| Report outputs | `report.Reporter`, registered by format |
| Scoring algorithms | one frozen function per `scoring.version`; a change is a new version |
| Builtin prompts | new `@<version>` entries; released versions are immutable |

- A variant is a new package that implements the interface and registers itself; the only `switch` over formats, providers, platforms or reporters is the registry lookup. A diff that edits an existing variant to add another first adds the missing extension point, as its own prefactor.
- Implementations honor the whole contract, including errors and context cancellation. Fakes behave like the real thing from the caller's side.
- Policy code (pipeline, agents, gate) depends on small interfaces, defined where they are consumed; vendor SDKs, `os/exec` and `net/http` appear only in adapters.

## Structure

- `context.Context` is the first parameter of anything that does I/O, blocks or starts goroutines, and lives only on the call stack.
- Package-level values are constants or immutable lookup functions; dependencies arrive as fields set by a constructor or at the composition root.
- `main` only adapts the process to `cli.Invocation`; exit codes and wiring live in `internal/cli`.

## Errors

- Each distinct failure keeps its own identity: a sentinel or typed error for conditions callers branch on, wrapping for everything else. Collapsing different causes into one error (a cancelled context reported as "not a repository") or into a default value (a git failure read as "file absent") is a finding.
- Exit codes come from classifying errors in `internal/cli`: 2 for configuration and usage, 3 for infrastructure, 1 only for a failed blocking gate.
- Anything that can reveal a secret (URLs with credentials, tokens, prompt text, raw model output) is redacted before it reaches an error, log, report or comment.

## Concurrency

Concurrency lives only in input and agent fan-out and in provider calls.

- Goroutines start through `errgroup` with `SetLimit`, and every goroutine has an owner that waits for it. Provider calls are bounded by a per-provider semaphore.
- Every blocking operation selects on `ctx.Done()`, including retries and backoff; deadlines pass down with `context.WithTimeout`.
- A failing input or agent unit is recorded as a result (a degraded run) and returns nil, so it never cancels its siblings; only a step-wide failure returns an error.
- Each goroutine writes its own slot of a pre-sized slice; shared state is guarded by a mutex documented beside the field.
- Concurrent code is tested for cancellation and timeouts, and `goleak` verifies no goroutine outlives the tests.

## Tests

- **Every test can fail.** For each new or changed test, name the code change that would turn it red. A test that passes whether or not the feature works (it re-encodes instead of tampering, it stops on an earlier error, it compares a result with itself) is a finding.
- Tests drive behavior through the seams of design §16: the in-process CLI (`clitest`), `score.Score` and `gate.Evaluate`, and `Ingestor.Parse` with golden files (`ingesttest`). Unexported helpers are tested through them.
- Fixtures for external formats are captured analyzer output. A hand-written fixture is labelled as such in the test and leaves its ticket `ready-for-human` until real output replaces it.
- Tests are hermetic: `t.TempDir()`, `t.Setenv()`, fixture git repos from `clitest.NewRepo`, injected HTTP clients; never the real home directory, network or global state.

## Tickets

- A finished ticket has a `## Demo` section that runs as written (see `docs/agents/finishing-a-ticket.md`). Each step shows the exit code of the qgate command itself, not of a pipe after it.
- The ticket's `Status:` line says where it stands: `resolved`, or `ready-for-human` with an `## Open` section naming what is left.
