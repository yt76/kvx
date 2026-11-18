package config

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxDiscoverEntries = 50000

var ignoredDirNames = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"obj_dir":      true,
	"work":         true,
	"simv.daidir":  true,
	"xsim.dir":     true,
	"build":        true,
	"dist":         true,
	"target":       true,
}

// MergeDiscovered returns base unchanged if base already specifies Files.
// Otherwise, it scans rootDir to discover Verilog source files (and includeDirs
// if base did not specify any).
func MergeDiscovered(base *Config, rootDir string) *Config {
	if base != nil && len(base.Files) > 0 {
		return base
	}
	discovered := Discover(rootDir)
	if base == nil {
		return discovered
	}
	if discovered == nil {
		return base
	}
	incDirs := base.IncludeDirs
	if len(incDirs) == 0 {
		incDirs = discovered.IncludeDirs
	}
	return &Config{
		Files:       discovered.Files,
		IncludeDirs: incDirs,
		Filelists:   append([]string(nil), base.Filelists...),
		Defines:     base.Defines,
	}
}

// Discover scans rootDir for Verilog sources (.v/.sv), header directories (.vh/.svh),
// and filelists (.f) to produce a zero-config Config.
func Discover(rootDir string) *Config {
	if rootDir == "" {
		return nil
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = filepath.Clean(rootDir)
	}

	raw := DiscoverRaw(absRoot)
	cfg, err := resolveConfig(raw, absRoot)
	if err != nil {
		// Fall back without filelists if a discovered .f file could not be parsed.
		raw.Filelists = nil
		cfg, _ = resolveConfig(raw, absRoot)
	}
	return cfg
}

// DiscoverRaw scans rootDir and returns an unresolved Config with paths relative to rootDir.
func DiscoverRaw(rootDir string) Config {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = filepath.Clean(rootDir)
	}

	var files []string
	var incDirs []string
	var filelists []string
	seenInc := make(map[string]bool)
	visited := 0

	_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != absRoot && (strings.HasPrefix(name, ".") || ignoredDirNames[name]) {
				return filepath.SkipDir
			}
			return nil
		}

		visited++
		if visited > maxDiscoverEntries {
			return fs.SkipAll
		}

		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".v", ".sv":
			files = append(files, rel)
		case ".vh", ".svh":
			incRel := filepath.ToSlash(filepath.Dir(rel))
			if !seenInc[incRel] {
				seenInc[incRel] = true
				incDirs = append(incDirs, incRel)
			}
		case ".f":
			// Only auto-adopt top-level .f filelists to avoid picking up unrelated nested test artifacts.
			if filepath.Dir(path) == absRoot {
				filelists = append(filelists, rel)
			}
		}
		return nil
	})

	sort.Strings(files)
	sort.Strings(incDirs)
	sort.Strings(filelists)

	return Config{
		Files:       files,
		IncludeDirs: incDirs,
		Filelists:   filelists,
	}
}

// FromArgsRaw converts simulator CLI arguments (e.g. -I, +incdir+, -D, +define+, -f, *.v)
// into a Config whose paths are normalized relative to baseDir when possible.
func FromArgsRaw(args []string, baseDir string) Config {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = filepath.Clean(baseDir)
	}

	var dirs []string
	var files []string
	var filelists []string
	defines := make(map[string]string)

	resolvePath := func(raw string, _ bool) string {
		if raw == "" {
			return ""
		}
		absPath := raw
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(absBase, absPath)
		}
		absPath = filepath.Clean(absPath)
		if rel, relErr := filepath.Rel(absBase, absPath); relErr == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return absPath
	}

	parseOptionTokens(args, resolvePath, &dirs, &files, &filelists, defines)

	cfg := Config{
		Files:       dedupeStrings(files),
		IncludeDirs: dedupeStrings(dirs),
		Filelists:   dedupeStrings(filelists),
	}
	if len(defines) > 0 {
		cfg.Defines = defines
	}
	return cfg
}

// SaveFile writes cfg formatted as JSON to path.
func SaveFile(path string, cfg Config) error {
	if parent := filepath.Dir(path); parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
