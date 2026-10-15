// Package buildinfo exposes the version metadata stamped into the binary at
// release time via -ldflags.
package buildinfo

import "runtime/debug"

// Set at build time, e.g.
// -ldflags "-X qgate/internal/buildinfo.version=0.1.0 -X qgate/internal/buildinfo.commit=abc123 -X qgate/internal/buildinfo.date=2026-10-15T00:00:00Z".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// Info describes this build of qgate.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Get returns the stamped build info, falling back to the VCS metadata the
// Go toolchain embeds when the binary was not built with -ldflags.
func Get() Info {
	info := Info{Version: version, Commit: commit, Date: date}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch {
			case s.Key == "vcs.revision" && info.Commit == "":
				info.Commit = s.Value
			case s.Key == "vcs.time" && info.Date == "":
				info.Date = s.Value
			}
		}
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.Date == "" {
		info.Date = "unknown"
	}
	return info
}
