// Package config loads, layers, validates and resolves qgate's configuration
// (design §5). Layers are plain documents until the very end, when the
// resolved config is decoded into typed structs.
package config

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed builtin.yaml
var builtinYAML []byte

const builtinSource = "builtin defaults"

// repoConfigNames are the repo config files looked up in the repo root, in order.
var repoConfigNames = [...]string{".qgate.yaml", ".qgate.yml"}

// Options say where to find configuration and how to resolve it.
type Options struct {
	Dir        string            // repo root; relative paths resolve against it
	Env        map[string]string // for ${VAR} interpolation
	OrgConfig  string            // org config path or HTTPS URL; empty for none
	ConfigPath string            // explicit repo config (--config or QGATE_CONFIG); empty to discover
	Stage      string
	Sets       []string // --set overrides, "dotted.path=value"
	HTTP       HTTPDoer // fetches an org config URL
}

// Resolved is the configuration for one run.
type Resolved struct {
	// RepoConfig is the repo config file used, or "" when there is none.
	RepoConfig string
	// Source names the repo config in messages (relative to the repo root),
	// or "builtin defaults" when there is no repo config.
	Source string
	// OrgConfig names the org config in messages, or "" when there is none.
	OrgConfig string
	// Locked are the keys the org config locks, with their values.
	Locked []LockedKey
	// Document is the resolved config as plain values, as printed by
	// `qgate validate` and stored in the report.
	Document map[string]any
	Config   Config
}

// Load resolves the configuration for opts.Stage: builtin defaults, then the
// org config, then the repo config, then the stage's profile, then --set.
// Problems with the configuration are returned as Errors; anything else is
// an infrastructure failure.
func Load(ctx context.Context, opts Options) (*Resolved, error) {
	sch, err := loadSchemas()
	if err != nil {
		return nil, err
	}
	builtin, err := parseYAML(builtinYAML)
	if err != nil {
		return nil, fmt.Errorf("parse builtin defaults: %w", err)
	}
	ls := layers{{source: builtinSource, doc: builtin}}

	var org, repo *layer
	var orgData []byte
	if opts.OrgConfig != "" {
		data, source, err := readOrgConfig(ctx, opts)
		if err != nil {
			return nil, err
		}
		doc, err := parseLayer(sch, data, source, opts.Env)
		if err != nil {
			return nil, err
		}
		org, orgData = &layer{source: source, doc: doc}, data
	}
	repoPath, err := discover(ctx, opts)
	if err != nil {
		return nil, err
	}
	if repoPath != "" {
		source := displayPath(opts.Dir, repoPath)
		doc, err := readLayer(ctx, sch, repoPath, source, opts.Env)
		if err != nil {
			return nil, err
		}
		repo = &layer{source: source, doc: doc}
	}

	errs := checkPlacement(org, repo)
	var locked []string
	if org != nil {
		ls = append(ls, *org)
		keys, lerrs := lockedKeys(sch, *org)
		locked, errs = keys, append(errs, lerrs...)
		if repo != nil {
			errs = append(errs, checkPin(orgData, org.source, *repo)...)
		}
	}
	if repo != nil {
		ls = append(ls, *repo)
	}
	if len(errs) > 0 {
		return nil, errs
	}

	merged := ls.merge()
	delete(merged, "locked")
	delete(merged, "org_config_sha256")
	blame := ls.blame(merged)

	// Each layer was checked on its own; required fields can only be
	// checked once the layers are merged.
	if errs, err := validateSchema(sch.layer, merged, complete, blame); err != nil || len(errs) > 0 {
		return nil, firstErr(err, errs)
	}

	profiles, _ := merged["profiles"].(map[string]any)
	if errs := checkProfiles(profiles, blame); len(errs) > 0 {
		return nil, errs
	}
	// Check every profile, not only the selected one, and report paths as
	// they are written in the layer that set them.
	if errs := checkSemantics(merged, blame); len(errs) > 0 {
		return nil, errs
	}
	profile, errs := resolveProfile(profiles, opts.Stage, blame)
	if len(errs) > 0 {
		return nil, errs
	}
	doc := resolvedDoc(merged, opts.Stage, profile)

	var lockedOut []LockedKey
	if len(locked) > 0 {
		if repo != nil {
			if errs := checkRepoLocks(locked, ls, *repo); len(errs) > 0 {
				return nil, errs
			}
		}
		lockedOut = describeLocked(locked, doc, *org)
	}

	// Only --set can make an already-checked document invalid, so the
	// resolved document is checked again only when it was used.
	if len(opts.Sets) > 0 {
		before := deepCopy(doc).(map[string]any)
		if errs := applySets(doc, opts.Sets); len(errs) > 0 {
			return nil, errs
		}
		if errs, err := validateSchema(sch.resolved, doc, complete, blameSource(setSource)); err != nil || len(errs) > 0 {
			return nil, firstErr(err, errs)
		}
		if errs := checkSemantics(doc, blameSource(setSource)); len(errs) > 0 {
			return nil, errs
		}
		if errs := checkSetLocks(locked, before, doc); len(errs) > 0 {
			return nil, errs
		}
	}

	cfg, err := decode(doc)
	if err != nil {
		return nil, err
	}
	res := &Resolved{RepoConfig: repoPath, Source: builtinSource, Locked: lockedOut, Document: doc, Config: cfg}
	if repo != nil {
		res.Source = repo.source
	}
	if org != nil {
		res.OrgConfig = org.source
	}
	return res, nil
}

// resolvedDoc replaces a merged document's profiles with the selected stage
// and its resolved profile.
func resolvedDoc(merged map[string]any, stage string, profile map[string]any) map[string]any {
	doc := deepCopy(merged).(map[string]any)
	delete(doc, "profiles")
	doc["stage"] = stage
	doc["profile"] = profile
	return doc
}

// discover finds the repo config: an explicit path, else the first of
// repoConfigNames in the repo root, else none.
func discover(ctx context.Context, opts Options) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if opts.ConfigPath != "" {
		p := opts.ConfigPath
		if !filepath.IsAbs(p) {
			p = filepath.Join(opts.Dir, p)
		}
		if _, err := os.Stat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", Errors{errorf(displayPath(opts.Dir, p), "", "repo config not found")}
			}
			return "", fmt.Errorf("repo config %s: %w", p, err)
		}
		return p, nil
	}
	for _, name := range repoConfigNames {
		p := filepath.Join(opts.Dir, name)
		_, err := os.Stat(p)
		switch {
		case err == nil:
			return p, nil
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("look up repo config: %w", err)
		}
	}
	return "", nil
}

// readLayer reads, validates and interpolates one config file.
func readLayer(ctx context.Context, sch *compiledSchemas, p, source string, env map[string]string) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	return parseLayer(sch, data, source, env)
}

// parseLayer parses, validates and interpolates one config layer.
func parseLayer(sch *compiledSchemas, data []byte, source string, env map[string]string) (map[string]any, error) {
	layer, err := parseYAML(data)
	if err != nil {
		return nil, Errors{errorf(source, "", "invalid YAML: %v", err)}
	}
	errs, err := validateSchema(sch.layer, layer, partial, blameSource(source))
	if err != nil {
		return nil, err
	}
	errs = append(errs, checkDuplicates(layer, source)...)
	errs = append(errs, interpolate(sch, layer, env, source)...)
	if len(errs) > 0 {
		return nil, errs
	}
	return layer, nil
}

// decode turns the validated, resolved document into typed structs. A
// failure here means the schema and the structs disagree: a qgate bug.
func decode(doc map[string]any) (Config, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return Config{}, fmt.Errorf("encode resolved config: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode resolved config (schema and types disagree): %w", err)
	}
	return cfg, nil
}

// displayPath shows p relative to the repo root when it lies inside it.
func displayPath(dir, p string) string {
	if rel, err := filepath.Rel(dir, p); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return p
}

func firstErr(err error, errs Errors) error {
	if err != nil {
		return err
	}
	return errs.orNil()
}
