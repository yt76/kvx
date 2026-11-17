package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFileName is the project configuration file name searched by kvx.
const ConfigFileName = "kvx.json"

// EnvConfig overrides kvx.json discovery with an explicit config file path.
const EnvConfig = "KVX_CONFIG"

// Config holds project-level settings loaded from kvx.json.
type Config struct {
	Files       []string `json:"files"`
	IncludeDirs []string `json:"includeDirs"`
	Filelists   []string `json:"filelists"`
}

// LoadFile reads and parses a kvx.json file at path, resolving relative paths
// against the directory containing path (including any -I / +incdir+ and file
// entries from listed filelists).
func LoadFile(path string) (*Config, error) {
	raw, baseDir, err := loadRawFile(path)
	if err != nil {
		return nil, err
	}
	return resolveConfig(raw, baseDir)
}

func loadRawFile(path string) (Config, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, "", err
	}

	var raw Config
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, "", fmt.Errorf("parsing %s: %w", path, err)
	}

	baseDir := filepath.Dir(path)
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	return raw, baseDir, nil
}

func resolveConfig(raw Config, baseDir string) (*Config, error) {
	var resolvedIncludes []string
	seenInc := make(map[string]bool)
	addDir := func(dir string) {
		if dir == "" {
			return
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		dir = filepath.Clean(dir)
		if !seenInc[dir] {
			seenInc[dir] = true
			resolvedIncludes = append(resolvedIncludes, dir)
		}
	}

	for _, dir := range raw.IncludeDirs {
		addDir(dir)
	}

	var resolvedFiles []string
	seenFiles := make(map[string]bool)
	addFile := func(f string) {
		if f == "" {
			return
		}
		if !filepath.IsAbs(f) {
			f = filepath.Join(baseDir, f)
		}
		f = filepath.Clean(f)
		if strings.ContainsAny(f, "*?[") {
			if matches, globErr := filepath.Glob(f); globErr == nil {
				for _, m := range matches {
					m = filepath.Clean(m)
					if !seenFiles[m] {
						seenFiles[m] = true
						resolvedFiles = append(resolvedFiles, m)
					}
				}
			}
			return
		}
		if !seenFiles[f] {
			seenFiles[f] = true
			resolvedFiles = append(resolvedFiles, f)
		}
	}

	for _, f := range raw.Files {
		addFile(f)
	}

	var resolvedFilelists []string
	for _, fl := range raw.Filelists {
		if fl == "" {
			continue
		}
		flPath := fl
		if !filepath.IsAbs(flPath) {
			flPath = filepath.Join(baseDir, flPath)
		}
		flPath = filepath.Clean(flPath)
		resolvedFilelists = append(resolvedFilelists, flPath)

		dirs, files, err := ParseFilelist(flPath, baseDir)
		if err != nil {
			return nil, err
		}
		for _, d := range dirs {
			if !seenInc[d] {
				seenInc[d] = true
				resolvedIncludes = append(resolvedIncludes, d)
			}
		}
		for _, f := range files {
			if !seenFiles[f] {
				seenFiles[f] = true
				resolvedFiles = append(resolvedFiles, f)
			}
		}
	}

	return &Config{
		Files:       resolvedFiles,
		IncludeDirs: resolvedIncludes,
		Filelists:   resolvedFilelists,
	}, nil
}

// FindAndLoad searches for kvx.json (or loads KVX_CONFIG if set).
// If no config file exists, it returns (nil, "", nil).
func FindAndLoad(startDir string) (*Config, string, error) {
	var raw Config
	var foundPath string
	var baseDir string
	hasConfig := false

	if envCfg := os.Getenv(EnvConfig); envCfg != "" {
		absCfg := envCfg
		if a, err := filepath.Abs(envCfg); err == nil {
			absCfg = a
		}
		loaded, dir, err := loadRawFile(absCfg)
		if err != nil {
			return nil, absCfg, err
		}
		raw = loaded
		baseDir = dir
		foundPath = absCfg
		hasConfig = true
	} else if startDir != "" {
		dir, err := filepath.Abs(startDir)
		if err != nil {
			dir = filepath.Clean(startDir)
		}
		baseDir = dir

		for {
			candidate := filepath.Join(dir, ConfigFileName)
			info, statErr := os.Stat(candidate)
			if statErr == nil && !info.IsDir() {
				loaded, cfgDir, loadErr := loadRawFile(candidate)
				if loadErr != nil {
					return nil, candidate, loadErr
				}
				raw = loaded
				baseDir = cfgDir
				foundPath = candidate
				hasConfig = true
				break
			}
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return nil, candidate, statErr
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if !hasConfig {
		return nil, "", nil
	}

	cfg, err := resolveConfig(raw, baseDir)
	return cfg, foundPath, err
}

// ParseFilelist reads a simulator filelist (.f) and extracts include directories
// (-I, -incdir, +incdir+) as well as source file paths.
func ParseFilelist(path string, baseDir string) ([]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading filelist %s: %w", path, err)
	}
	defer f.Close()

	flDir := filepath.Dir(path)
	var dirs []string
	var files []string

	resolvePath := func(rawPath string, expectDir bool) string {
		if rawPath == "" {
			return ""
		}
		if filepath.IsAbs(rawPath) {
			return filepath.Clean(rawPath)
		}
		candBase := filepath.Clean(filepath.Join(baseDir, rawPath))
		if info, err := os.Stat(candBase); err == nil && info.IsDir() == expectDir {
			return candBase
		}
		candFL := filepath.Clean(filepath.Join(flDir, rawPath))
		if info, err := os.Stat(candFL); err == nil && info.IsDir() == expectDir {
			return candFL
		}
		return candBase
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		// Strip inline // or # comments if preceded by whitespace.
		if idx := strings.Index(line, " //"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		fields := strings.Fields(line)
		for i := 0; i < len(fields); i++ {
			tok := fields[i]
			switch {
			case tok == "-I" || tok == "-incdir":
				if i+1 < len(fields) {
					i++
					if d := resolvePath(fields[i], true); d != "" {
						dirs = append(dirs, d)
					}
				}
			case strings.HasPrefix(tok, "-I") && len(tok) > 2:
				if d := resolvePath(tok[2:], true); d != "" {
					dirs = append(dirs, d)
				}
			case strings.HasPrefix(tok, "+incdir+"):
				rest := strings.TrimPrefix(tok, "+incdir+")
				for _, part := range strings.Split(rest, "+") {
					if d := resolvePath(part, true); d != "" {
						dirs = append(dirs, d)
					}
				}
			case tok == "-D" || tok == "-y" || tok == "-v" || tok == "-f" || tok == "-F" || tok == "-o" || tok == "-s":
				if i+1 < len(fields) {
					i++
				}
			case strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "+"):
				// Ignore other simulator flags.
			default:
				if p := resolvePath(tok, false); p != "" {
					files = append(files, p)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scanning filelist %s: %w", path, err)
	}
	return dirs, files, nil
}
