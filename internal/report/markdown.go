package report

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/assimoes/qgate/internal/model"
)

const (
	// markdownLimit caps the summary, as PR comments are capped (design
	// §14.3).
	markdownLimit = 60000
	// markdownTopFindings is how many findings the summary lists.
	markdownTopFindings = 15
)

// Markdown is the PR summary's content as a file: outcome, scores, failed
// gate checks, new-finding counts, top findings, warnings.
type Markdown struct {
	// FullReport names the JSON report the summary points to when it is
	// truncated.
	FullReport string
}

// Name is the format name outputs use.
func (Markdown) Name() string { return "markdown" }

// Render writes the summary, cut at markdownLimit characters on a line
// boundary with a pointer to the full report.
func (m Markdown) Render(r *model.Report) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## qgate: %s\n\n", strings.ToUpper(string(r.Gate.Outcome)))
	fmt.Fprintf(&b, "Stage **%s** · %s enforcement · gating scope %s\n\n", md(r.Run.Stage), r.Run.Enforcement, r.Run.GatingScope)
	if r.Gate.Degraded {
		b.WriteString("> **Degraded run:** an input or agent failed, so the result may be incomplete. See the report.\n\n")
	}

	b.WriteString("### Scores\n\n| Category | Score |\n|---|---|\n")
	fmt.Fprintf(&b, "| **overall** | %s |\n", mdPoints(r.Scores.Overall))
	for _, c := range slices.Sorted(maps.Keys(r.Scores.Categories)) {
		fmt.Fprintf(&b, "| %s | %s |\n", md(c), mdPoints(r.Scores.Categories[c]))
	}

	var failed []model.GateCheck
	for _, c := range r.Gate.Checks {
		if c.Result == model.CheckFail {
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		b.WriteString("\n### Failed gate checks\n\n| Check | Expected | Actual |\n|---|---|---|\n")
		for _, c := range failed {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.Check, md(c.Expected), md(actual(c.Actual)))
		}
	}

	counts := map[model.Severity]int{}
	for _, f := range r.Findings {
		if f.Status == model.StatusNew {
			counts[f.Severity]++
		}
	}
	b.WriteString("\n### New findings\n\n| Critical | High | Medium | Low | Info |\n|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %d |\n", counts[model.SeverityCritical], counts[model.SeverityHigh],
		counts[model.SeverityMedium], counts[model.SeverityLow], counts[model.SeverityInfo])

	if top := topFindings(r, markdownTopFindings); len(top) > 0 {
		b.WriteString("\n### Top findings\n\n| Severity | Rule | Location | Title |\n|---|---|---|---|\n")
		for _, f := range top {
			fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s |\n", f.Severity, code(f.RuleID), code(location(f)), md(f.Title))
		}
	}

	if len(r.Warnings) > 0 {
		b.WriteString("\n### Warnings\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", md(w))
		}
	}
	fmt.Fprintf(&b, "\n<sub>qgate %s · full report: `%s`</sub>\n", md(r.QgateVersion), code(m.FullReport))
	return []byte(capMarkdown(b.String(), m.FullReport)), nil
}

// capMarkdown cuts s to markdownLimit characters on a line boundary and
// says where the rest is.
func capMarkdown(s, fullReport string) string {
	if len([]rune(s)) <= markdownLimit {
		return s
	}
	note := fmt.Sprintf("\n\n_Summary truncated at %d characters; the full report is `%s`._\n", markdownLimit, code(fullReport))
	runes := []rune(s)[:markdownLimit-len([]rune(note))]
	cut := string(runes)
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return cut + note
}

func mdPoints(p *int) string {
	if p == nil {
		return "not assessed"
	}
	return fmt.Sprint(*p)
}

// mdEscaper keeps analyzer- and agent-written text inert in Markdown: no
// raw HTML (so no hidden comments or markers), no @mentions, no table
// breaks.
var mdEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", "@", "&#64;", "|", `\|`,
	"\r\n", " ", "\n", " ", "\r", " ",
	"*", `\*`, "_", `\_`, "`", "&#96;", "[", `\[`, "]", `\]`,
)

func md[S ~string](s S) string { return mdEscaper.Replace(string(s)) }

// codeEscaper keeps text inert inside a backtick code span.
var codeEscaper = strings.NewReplacer("`", "'", "|", `\|`, "\n", " ", "\r", " ")

func code(s string) string { return codeEscaper.Replace(s) }
