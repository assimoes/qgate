# qgate

An in-house Go CLI that gates CI pipelines on deterministic scores over analyzer and LLM-agent findings.

- Technical contract: `docs/design.md` (its MUST rules and §1 rules for implementers are binding)
- Spec: `.scratch/qgate/spec.md`; tickets: `.scratch/qgate/issues/`
- Vocabulary: `GLOSSARY.md`. Use its terms in code identifiers, comments, logs and docs.
- Decisions: architectural ones in `docs/adr/`; small "simplest reasonable choice" calls in `docs/DECISIONS.md`

## Go practices

### Style and tooling

- Code is `gofmt`/`goimports` clean. `go vet ./...`, `staticcheck ./...` and `go test -race ./...` pass before any change is considered done.
- Follow Effective Go and the Go Code Review Comments: short receiver names, `MixedCaps`, no stutter (`report.JSON`, not `report.JSONReport`), initialisms in one case (`ID`, `URL`, `SHA`).
- Packages are named for what they provide, are lowercase single words, and live under `internal/` unless they are a deliberate public contract. No `util`, `common`, `helpers` or `misc` packages.
- Keep `main` thin: it builds dependencies and calls the in-process CLI entry point.
- Doc comments on every exported identifier, starting with its name. Comments explain *why*, not *what*.

### Errors

- Return errors; never `panic` for expected failures. `panic` is reserved for programmer errors that make continuing unsafe.
- Wrap with context using `fmt.Errorf("parse sarif %s: %w", path, err)`. Messages are lowercase, no trailing punctuation, no "failed to" prefix.
- Define sentinel errors (`var ErrInvalidOutput = errors.New(...)`) or typed errors for conditions callers branch on; check them with `errors.Is` / `errors.As`, never by string matching.
- Handle each error once: either return it or log it, not both.
- Exit codes are decided in one place (the CLI layer) by classifying errors, never by `os.Exit` deep in the call stack.

### Design of types and functions

- No global mutable state and no `init()` side effects beyond registering builtins. Dependencies are passed explicitly, usually as struct fields set by a constructor.
- `context.Context` is the first parameter of every function that does I/O, blocks, or spawns goroutines. Never store a context in a struct.
- Make the zero value useful where practical; otherwise provide a `New...` constructor and keep fields unexported.
- Keep the deterministic core (normalize from step 7, score, gate) free of I/O, clocks and randomness. Time, randomness, filesystem and network reach it only through injected parameters.
- Prefer returning concrete types; accept the narrowest interface you need.
- Avoid premature generics; use them when they remove real duplication across types.
- Log with `log/slog` structured attributes, never `fmt.Print` or the `log` package. Never log secrets, prompts or raw model output (see design §15).

### Testing

- Test behavior through the three seams in design §16 (CLI entry point, `Score`/`Evaluate`, `Ingestor.Parse`). Don't test unexported functions directly.
- Table-driven tests with `t.Run` subtests; `t.Parallel()` wherever tests don't share state.
- Golden files under `testdata/`, regenerated with `-update`.
- Use `t.TempDir()`, `t.Setenv()` and `t.Cleanup()`; tests never touch the real home directory, network or global state.
- Fakes over mocks: use the `fake` provider and recorded HTTP, not mocking frameworks.

## SOLID in Go

- **Single responsibility.** A package has one reason to change: `ingest/sarif` changes when SARIF does, `score` when the scoring algorithm does. A function does one step of the pipeline; the pipeline composes them.
- **Open/closed (most important here).** qgate grows by *adding* code, not editing existing code. Every axis of variation listed below is an extension point: an interface plus a registry. Adding a new variant means a new package that implements the interface and registers itself. No `switch` over format, provider type, VCS platform or reporter type anywhere outside the registry.

  | Axis of variation | Extension point |
  |---|---|
  | Analyzer report formats | `Ingestor`, registered by format name |
  | LLM providers | `llm.Provider`, registered by `type` |
  | VCS platforms | `vcs.Client`, registered by platform |
  | Report outputs | a `Reporter` interface, registered by format |
  | Scoring algorithms | one implementation per `scoring.version`, frozen once released; a change is a new version, never an edit |
  | Builtin prompts | new `@<version>` files, never edits to released ones |

  If a new requirement forces you to edit a type switch or an existing implementation to add a variant, introduce the missing extension point first (prefactor), then add the variant.
- **Liskov substitution.** Every implementation honors its interface's full contract, including errors and context cancellation. The `fake` provider and recorded VCS behave like the real ones from the caller's point of view; the agent runner never type-asserts to a concrete provider.
- **Interface segregation.** Keep interfaces small (one to three methods). Define them in the consuming package when only that package needs them; the exceptions are the extension points above, which are deliberate contracts and live with their registries.
- **Dependency inversion.** High-level policy (pipeline, agents, gate) depends on interfaces, not on vendor SDKs, `os/exec` or `net/http`. Concrete adapters are wired at the CLI composition root.

## Concurrency

Concurrency exists only in pipeline steps 3 (inputs) and 6 (agents) and in provider calls. Everything downstream is sequential and deterministic.

- **Structured concurrency only.** Spawn goroutines through `errgroup.WithContext` (with `SetLimit` for bounded fan-out). Every goroutine has an owner that waits for it; no fire-and-forget `go` statements.
- **Bound everything.** Fan-out is limited by `errgroup.SetLimit`, and provider calls by a per-provider `semaphore.Weighted` (`max_concurrency`). Never start one goroutine per item for unbounded inputs.
- **Respect cancellation.** Every blocking operation selects on `ctx.Done()`: channel sends and receives, semaphore acquisition (`Acquire(ctx, n)`), retries and backoff sleeps (use a timer with `select`, never `time.Sleep`). Pass deadlines down with `context.WithTimeout`, and always `defer cancel()`.
- **Decide failure semantics explicitly.** `errgroup` cancels siblings on the first error. That's right for fatal errors, but a failing input or agent unit must *not* cancel the others: record it as a result (degraded run) and return `nil` from the goroutine. Only return an error from a goroutine when the whole step should abort.
- **Share memory by communicating, or confine it.** Prefer giving each goroutine its own slot (`results[i] = ...` into a pre-sized slice) over shared maps. When state must be shared, guard it with a `sync.Mutex` held for the shortest possible time and documented next to the field. Use `sync/atomic` only for simple counters.
- **Channel discipline.** The sender owns and closes a channel; receivers never close it. Document buffer sizes and why. Never leave a goroutine blocked on a channel nobody will drain.
- **Determinism after concurrency.** Results gathered concurrently are put into a canonical order (by input name, agent name, path, line, fingerprint) before anything downstream sees them. Output must never depend on scheduling order (design §2.3).
- **No hidden concurrency in libraries.** Functions are synchronous unless their name and doc say otherwise; callers decide whether to run them concurrently.
- **Verify.** Every package with concurrent code is tested under `-race`, including cancellation and timeout paths (fake provider delays, deadline expiry), and asserts that no goroutine outlives its test. A leak-detection helper such as `go.uber.org/goleak` is a new dependency, so log it in `docs/DECISIONS.md` before adding it.

## Agent skills

### Issue tracker

Issues are local markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. In local files they go on a `Status:` line. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
