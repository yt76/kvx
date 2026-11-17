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

// Config holds project-level settings loaded from kvx.json.
type Config struct {
	IncludeDirs []string `json:"includeDirs"`
	Filelists   []string `json:"filelists"`
}

// LoadFile reads and parses a kvx.json file at path, resolving relative paths
// against the directory containing path (including any -I / +incdir+ entries
// from listed filelists).
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw Config
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	baseDir := filepath.Dir(path)
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}

	var resolvedIncludes []string
	seen := make(map[string]bool)
	addDir := func(dir string) {
		if dir == "" {
			return
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			resolvedIncludes = append(resolvedIncludes, dir)
		}
	}

	for _, dir := range raw.IncludeDirs {
		addDir(dir)
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

		dirs, err := ParseFilelist(flPath, baseDir)
		if err != nil {
			return nil, err
		}
		for _, d := range dirs {
			if !seen[d] {
				seen[d] = true
				resolvedIncludes = append(resolvedIncludes, d)
			}
		}
	}

	return &Config{
		IncludeDirs: resolvedIncludes,
		Filelists:   resolvedFilelists,
	}, nil
}

// FindAndLoad searches for kvx.json starting at startDir and walking up parent
// directories until one is found or the filesystem root is reached.
// If no kvx.json exists, it returns (nil, "", nil).
func FindAndLoad(startDir string) (*Config, string, error) {
	if startDir == "" {
		return nil, "", nil
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		dir = filepath.Clean(startDir)
	}

	for {
		candidate := filepath.Join(dir, ConfigFileName)
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			cfg, loadErr := LoadFile(candidate)
			return cfg, candidate, loadErr
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

	return nil, "", nil
}

// ParseFilelist reads a simulator filelist (.f) and extracts include directories
// specified via -I <dir>, -I<dir>, or +incdir+<dir1>[+<dir2>...].
func ParseFilelist(path string, baseDir string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading filelist %s: %w", path, err)
	}
	defer f.Close()

	flDir := filepath.Dir(path)
	var dirs []string

	resolve := func(rawDir string) string {
		if rawDir == "" {
			return ""
		}
		if filepath.IsAbs(rawDir) {
			return filepath.Clean(rawDir)
		}
		// Prefer baseDir (project root) if that directory exists; otherwise try filelist's own directory.
		candBase := filepath.Clean(filepath.Join(baseDir, rawDir))
		if info, err := os.Stat(candBase); err == nil && info.IsDir() {
			return candBase
		}
		candFL := filepath.Clean(filepath.Join(flDir, rawDir))
		if info, err := os.Stat(candFL); err == nil && info.IsDir() {
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
					if d := resolve(fields[i]); d != "" {
						dirs = append(dirs, d)
					}
				}
			case strings.HasPrefix(tok, "-I") && len(tok) > 2:
				if d := resolve(tok[2:]); d != "" {
					dirs = append(dirs, d)
				}
			case strings.HasPrefix(tok, "+incdir+"):
				rest := strings.TrimPrefix(tok, "+incdir+")
				for _, part := range strings.Split(rest, "+") {
					if d := resolve(part); d != "" {
						dirs = append(dirs, d)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning filelist %s: %w", path, err)
	}
	return dirs, nil
}
