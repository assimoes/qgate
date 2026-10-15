package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// maxOrgConfigBytes bounds what qgate reads from an org config URL.
const maxOrgConfigBytes = 1 << 20

// HTTPDoer sends HTTP requests. *http.Client satisfies it; the caller
// chooses timeouts and transport.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// readOrgConfig returns the org config's bytes and how to name it in
// messages. A URL must use HTTPS. Failing to reach a server is an
// infrastructure failure; a missing file or a refused URL is a config error.
func readOrgConfig(ctx context.Context, opts Options) ([]byte, string, error) {
	ref := opts.OrgConfig
	switch {
	case strings.HasPrefix(ref, "https://"):
		source := redactURL(ref)
		data, err := fetch(ctx, opts.HTTP, ref, source)
		return data, source, err
	case strings.Contains(ref, "://"):
		source := redactURL(ref)
		return nil, source, Errors{errorf(source, "", "org config URL must use HTTPS")}
	}

	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(opts.Dir, p)
	}
	source := displayPath(opts.Dir, p)
	if err := ctx.Err(); err != nil {
		return nil, source, err
	}
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, source, Errors{errorf(source, "", "org config not found")}
	case err != nil:
		return nil, source, fmt.Errorf("read org config %s: %w", source, err)
	}
	return data, source, nil
}

// fetch GETs rawURL. Messages name it only by source, its redacted form,
// because the URL may carry credentials.
func fetch(ctx context.Context, client HTTPDoer, rawURL, source string) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("fetch org config %s: no HTTP client configured", source)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch org config %s: invalid URL", source)
	}
	resp, err := client.Do(req)
	if err != nil {
		// *url.Error repeats the full URL; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("fetch org config %s: %w", source, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch org config %s: %s", source, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOrgConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch org config %s: %w", source, err)
	}
	if len(data) > maxOrgConfigBytes {
		return nil, fmt.Errorf("fetch org config %s: larger than %d bytes", source, maxOrgConfigBytes)
	}
	return data, nil
}

// redactURL drops user info, query and fragment, which may carry
// credentials, so a URL can appear in messages and reports.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid org config URL>"
	}
	u.User, u.RawQuery, u.Fragment, u.ForceQuery = nil, "", "", false
	return u.String()
}

// checkPin verifies the org config against the repo config's
// org_config_sha256, when the repo config sets one.
func checkPin(org []byte, orgSource string, repo layer) Errors {
	want, ok := repo.doc["org_config_sha256"].(string)
	if !ok {
		return nil
	}
	sum := sha256.Sum256(org)
	if got := hex.EncodeToString(sum[:]); got != want {
		return Errors{errorf(repo.source, "org_config_sha256",
			"org config %s has SHA-256 %s, but the repo config pins %s", orgSource, got, want)}
	}
	return nil
}

// checkPlacement rejects keys that only make sense in the other kind of
// layer: only an org config may lock keys, and only a repo config may pin
// the org config.
func checkPlacement(org, repo *layer) Errors {
	var errs Errors
	if org != nil {
		if _, ok := org.doc["org_config_sha256"]; ok {
			errs = append(errs, errorf(org.source, "org_config_sha256", "only a repo config may pin the org config"))
		}
	}
	if repo != nil {
		if _, ok := repo.doc["locked"]; ok {
			errs = append(errs, errorf(repo.source, "locked", "only the org config may lock keys"))
		}
	}
	return errs
}
