# qgate

An in-house Go CLI that gates CI pipelines on deterministic scores over analyzer and LLM-agent findings.

- Technical contract: `docs/design.md` (its MUST rules and §1 rules for implementers are binding)
- Spec: `.scratch/qgate/spec.md`; tickets: `.scratch/qgate/issues/`
- Vocabulary: `GLOSSARY.md`. Use its terms in code identifiers, comments, logs and docs.
- Decisions: architectural ones in `docs/adr/`; small "simplest reasonable choice" calls in `docs/DECISIONS.md`
- Review rules: `CODING_STANDARDS.md` (read it when reviewing a diff or designing a new extension point)
- Check: `scripts/check.sh` (lint, formatting, module tidiness, `go test -race`); the pre-commit hook runs it once enabled with `git config core.hooksPath .githooks`
- Finishing a ticket (tests proven red, check, stakeholder demo, status): `docs/agents/finishing-a-ticket.md`

## Agent skills

### Issue tracker

Issues are local markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. In local files they go on a `Status:` line; a finished ticket's is `resolved`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
