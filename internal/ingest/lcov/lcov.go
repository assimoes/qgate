// Package lcov ingests LCOV tracefiles into coverage metrics (design §8.2).
package lcov

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/ingest/coverage"
)

// Ingestor parses LCOV tracefiles.
type Ingestor struct{}

// New returns the LCOV ingestor.
func New() Ingestor { return Ingestor{} }

// Format is the format name inputs use.
func (Ingestor) Format() string { return "lcov" }

// ProducesFindings is false: LCOV yields metrics only.
func (Ingestor) ProducesFindings() bool { return false }

// Parse reads SF/DA/BRDA records; the summary lines (LF, LH, BRF, BRH) are
// ignored in favour of the per-line data they summarize.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, gc *gitctx.Context) (ingest.Result, error) {
	byPath := map[string]*coverage.File{}
	var cur *coverage.File
	var res ingest.Result
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		text := strings.TrimSpace(sc.Text())
		key, val, _ := strings.Cut(text, ":")
		switch key {
		case "SF":
			if f, ok := byPath[val]; ok {
				cur = f
			} else {
				f := coverage.NewFile(val)
				byPath[val] = &f
				cur = &f
			}
		case "end_of_record":
			cur = nil
		case "DA", "BRDA":
			if cur == nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("line %d: %s outside a file record", n, key))
				continue
			}
			fields := strings.Split(val, ",")
			lineNo, err := strconv.Atoi(fields[0])
			if err != nil || lineNo <= 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("line %d: invalid %s record %q", n, key, val))
				continue
			}
			if key == "DA" && len(fields) >= 2 {
				hits, _ := strconv.Atoi(fields[1])
				cur.Lines[lineNo] += hits
			}
			if key == "BRDA" && len(fields) == 4 {
				b := cur.Branches[lineNo]
				b.Total++
				if taken, err := strconv.Atoi(fields[3]); err == nil && taken > 0 {
					b.Covered++
				}
				cur.Branches[lineNo] = b
			}
		}
	}
	if err := sc.Err(); err != nil {
		return ingest.Result{}, fmt.Errorf("read lcov: %w", err)
	}
	files := make([]coverage.File, 0, len(byPath))
	for _, p := range slices.Sorted(maps.Keys(byPath)) {
		files = append(files, *byPath[p])
	}
	res.Metrics = coverage.Metrics(files, gc)
	return res, nil
}
