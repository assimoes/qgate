# Finishing a ticket

Steps from "tests green" to "ticket done", in order. Each ends on its completion criterion.

## 1. Prove each new test can fail

For every test the ticket adds or changes, break the code it covers (invert a condition, return early, drop the new branch), run that test, and watch it go **red**; then restore the code. A test that stays green while its feature is broken gets rewritten until it goes red.

Done when every new test has gone red once against broken code.

## 2. Run the check

Run `scripts/check.sh`: module tidiness, golangci-lint (formatting, vet, staticcheck and the review-finding linters) and `go test -race`. The pre-commit hook runs it too.

Done when it prints `all checks passed`.

## 3. Write and run the demo

Add a `## Demo` section to the ticket file in `.scratch/qgate/issues/`: the steps to show what the ticket delivered to stakeholders, by hand, with the real binary.

- Start from nothing: build the binary (`go build -o /tmp/qgate ./cmd/qgate`) and set up a throwaway repo, config and analyzer reports with copy-pasteable commands.
- One step per behavior the ticket added, each with the command to run and what the audience should see: the outcome line, the exit code, the relevant part of the report or error.
- Include the failure cases the ticket handles.
- Print the exit code of the qgate command itself (`echo "exit=$?"` right after it, or `${PIPESTATUS[0]}` after a pipe).
- Keep it to what a five-minute demo needs.

Run the script end to end, then follow it with a "What to point out" list, one line per step.

Done when every step's output matches what the section says it shows.

## 4. Record and close

- Log every "simplest reasonable choice" in `docs/DECISIONS.md`; a choice that changes design behavior also updates the design section.
- Tick the acceptance boxes the work meets. Set `Status:` to `resolved`, or to `ready-for-human` with an `## Open` section naming what is left (for example, captured analyzer output for golden tests).

Done when the ticket file says exactly what was and was not delivered.
