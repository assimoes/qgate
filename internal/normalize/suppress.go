package normalize

import (
	"fmt"
	"slices"
	"time"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/model"
)

// Suppress marks tool findings that a config suppression covers as
// suppressed, with its reason as the status note (design §9 step 8). A
// suppression covers a finding when its rule glob matches the namespaced
// rule ID and, if it lists paths, one of them matches the finding's path.
// A suppression is valid through its expiry date (UTC); once expired it is
// ignored and reported in the returned warnings. today is the run's date,
// passed in so the step stays a pure function.
func Suppress(fs []model.Finding, sups []config.Suppression, today time.Time) ([]model.Finding, []string) {
	var active []config.Suppression
	var warnings []string
	day := today.UTC().Format(config.ExpiryLayout)
	for i, s := range sups {
		if s.Expires != "" && s.Expires < day { // ISO dates compare as strings
			warnings = append(warnings, fmt.Sprintf(
				"suppression %d (rule %q) expired on %s and is ignored: %s", i, s.Rule, s.Expires, s.Reason))
			continue
		}
		active = append(active, s)
	}

	out := slices.Clone(fs)
	for i := range out {
		f := &out[i]
		if f.Source.Kind != model.SourceTool || !suppressible(f.Status) {
			continue
		}
		for _, s := range active {
			if covers(s, *f) {
				f.Status, f.StatusNote = model.StatusSuppressed, s.Reason
				break
			}
		}
	}
	return out, warnings
}

// suppressible reports whether a status can still become suppressed:
// findings already set aside for another reason keep it.
func suppressible(s model.Status) bool {
	return s == model.StatusNew || s == model.StatusOutOfScope || s == model.StatusBaseline
}

func covers(s config.Suppression, f model.Finding) bool {
	if !glob(s.Rule, f.RuleID) {
		return false
	}
	return len(s.Paths) == 0 || slices.ContainsFunc(s.Paths, func(p string) bool { return glob(p, f.Location.Path) })
}
