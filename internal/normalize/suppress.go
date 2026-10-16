package normalize

import (
	"fmt"
	"slices"
	"time"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/glob"
	"github.com/assimoes/qgate/internal/model"
)

// Suppress marks findings that a config suppression covers as suppressed,
// with its reason as the status note (design §9 step 8). A suppression
// covers a finding when its rule glob matches the namespaced rule ID and,
// if it lists paths, one of them matches the finding's path. It applies to
// tool and agent findings alike; findings already superseded or invalid
// keep that status. A suppression is valid through its expiry date (UTC);
// after it, it is ignored and reported in the returned warnings, which are
// deduplicated and sorted. now is the run's time, passed in so the step
// stays a pure function.
func Suppress(fs []model.Finding, sups []config.Suppression, now time.Time) ([]model.Finding, []string) {
	var active []config.Suppression
	var warnings []string
	for _, s := range sups {
		if s.Expires.ExpiredAt(now) {
			warnings = append(warnings, fmt.Sprintf("suppression for %q (%s) expired on %s and is ignored", s.Rule, s.Reason, s.Expires))
			continue
		}
		active = append(active, s)
	}

	out := slices.Clone(fs)
	for i := range out {
		f := &out[i]
		if !suppressible(f.Status) {
			continue
		}
		for _, s := range active {
			if covers(s, *f) {
				f.Status, f.StatusNote = model.StatusSuppressed, s.Reason
				break
			}
		}
	}
	slices.Sort(warnings)
	return out, slices.Compact(warnings)
}

// suppressible reports whether a status can become suppressed: superseded
// and invalid findings keep their status.
func suppressible(s model.Status) bool {
	return s == model.StatusNew || s == model.StatusOutOfScope || s == model.StatusBaseline
}

func covers(s config.Suppression, f model.Finding) bool {
	if !glob.Rule(s.Rule, f.RuleID) {
		return false
	}
	return len(s.Paths) == 0 || slices.ContainsFunc(s.Paths, func(p string) bool { return glob.Path(p, f.Location.Path) })
}
