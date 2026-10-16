// Package buildinfo exposes the version metadata stamped into the binary at
// release time via -ldflags.
package buildinfo

import "runtime/debug"

// Set at build time, e.g.
// -ldflags "-X github.com/assimoes/qgate/internal/buildinfo.version=0.1.0 -X github.com/assimoes/qgate/internal/buildinfo.commit=abc123 -X github.com/assimoes/qgate/internal/buildinfo.date=2026-10-15T00:00:00Z".
// These are written only by the linker and never mutated at run time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

const unknown = "unknown"

// Info describes this build of qgate.
type Info struct {
	Version string
	Commit  string
	Date    string // build date; "unknown" unless stamped at build time
}

// Get returns the stamped build info. Without stamping, the commit falls
// back to the VCS revision the Go toolchain embeds; the build date has no
// honest fallback and reads "unknown".
func Get() Info {
	info := Info{Version: version, Commit: commit, Date: date}
	if info.Commit == "" {
		info.Commit = vcsRevision()
	}
	if info.Date == "" {
		info.Date = unknown
	}
	return info
}

func vcsRevision() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return unknown
}
