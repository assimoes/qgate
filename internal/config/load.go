// Package config loads, layers, validates and resolves qgate's configuration
// (design §5). Layers are plain documents until the very end, when the
// resolved config is decoded into typed structs.
package config

import (
	"bytes"
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

// RepoConfigNames are the repo config files looked up in the repo root, in order.
var RepoConfigNames = []string{".qgate.yaml", ".qgate.yml"}

// Options say where to find configuration and how to resolve it.
type Options struct {
	Dir        string            // repo root; relative paths resolve against it
	Env        map[string]string // for ${VAR} interpolation
	ConfigPath string            // explicit repo config (--config or QGATE_CONFIG); empty to discover
	Stage      string
	Sets       []string // --set overrides, "dotted.path=value"
}

// Resolved is the configuration for one run.
type Resolved struct {
	// RepoConfig is the repo config file used, or "" when only builtin
	// defaults apply.
	RepoConfig string
	// Document is the resolved config as plain values, as printed by
	// `qgate validate` and stored in the report.
	Document map[string]any
	Config   Config
}

// Load resolves the configuration for opts.Stage. Problems with the
// configuration are returned as Errors; anything else is an I/O failure.
func Load(opts Options) (*Resolved, error) {
	sch, err := loadSchemas()
	if err != nil {
		return nil, err
	}

	builtin, err := parseYAML(builtinYAML)
	if err != nil {
		return nil, fmt.Errorf("parse builtin defaults: %w", err)
	}

	merged := builtin
	repoPath, err := discover(opts)
	if err != nil {
		return nil, err
	}
	source := builtinSource
	if repoPath != "" {
		source = displayPath(opts.Dir, repoPath)
		layer, err := readLayer(sch, repoPath, source, opts.Env)
		if err != nil {
			return nil, err
		}
		merged = mergeLayers(merged, layer)
	}

	profiles, _ := merged["profiles"].(map[string]any)
	if errs := checkProfiles(profiles, source); len(errs) > 0 {
		return nil, errs
	}
	// Check every profile, not only the selected one, and report paths as
	// they are written in the config file.
	if errs := checkSemantics(merged, source); len(errs) > 0 {
		return nil, errs
	}
	profile, errs := resolveProfile(profiles, opts.Stage, source)
	if len(errs) > 0 {
		return nil, errs
	}

	doc := merged
	delete(doc, "profiles")
	doc["stage"] = opts.Stage
	doc["profile"] = profile

	// Only --set can make an already-checked document invalid, so the
	// resolved document is checked again only when it was used.
	if len(opts.Sets) > 0 {
		if errs := applySets(doc, opts.Sets); len(errs) > 0 {
			return nil, errs
		}
		if errs, err := validateSchema(sch.resolved, doc, setSource); err != nil || len(errs) > 0 {
			return nil, firstErr(err, errs)
		}
		if errs := checkSemantics(doc, setSource); len(errs) > 0 {
			return nil, errs
		}
	}

	cfg, err := decode(doc)
	if err != nil {
		return nil, err
	}
	res := &Resolved{Document: doc, Config: cfg}
	if repoPath != "" {
		res.RepoConfig = repoPath
	}
	return res, nil
}

// discover finds the repo config: an explicit path, else the first of
// RepoConfigNames in the repo root, else none.
func discover(opts Options) (string, error) {
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
	for _, name := range RepoConfigNames {
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
func readLayer(sch *compiledSchemas, p, source string, env map[string]string) (map[string]any, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	layer, err := parseYAML(data)
	if err != nil {
		return nil, Errors{errorf(source, "", "invalid YAML: %v", err)}
	}
	errs, err := validateSchema(sch.layer, layer, source)
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
