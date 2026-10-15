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
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// maxOrgConfigBytes bounds what qgate reads from an org config URL.
	maxOrgConfigBytes = 1 << 20
	// orgConfigTimeout bounds fetching an org config when the caller did not
	// supply its own HTTP client.
	orgConfigTimeout = 30 * time.Second
)

// readOrgConfig returns the org config's bytes and how to name it in
// messages. A URL must use HTTPS. Failing to reach a server is an
// infrastructure failure; a missing file or a refused URL is a config error.
func readOrgConfig(ctx context.Context, opts Options) ([]byte, string, error) {
	ref := opts.OrgConfig
	switch {
	case strings.HasPrefix(ref, "https://"):
		data, err := fetch(ctx, opts.HTTPClient, ref)
		return data, ref, err
	case strings.Contains(ref, "://"):
		return nil, ref, Errors{errorf(ref, "", "org config URL must use HTTPS")}
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

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: orgConfigTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch org config %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch org config %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch org config %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOrgConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch org config %s: %w", url, err)
	}
	if len(data) > maxOrgConfigBytes {
		return nil, fmt.Errorf("fetch org config %s: larger than %d bytes", url, maxOrgConfigBytes)
	}
	return data, nil
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
