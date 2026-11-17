package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kvx/internal/config"
)

func runInit(args []string, out io.Writer) int {
	outPath := config.ConfigFileName
	force := false
	var simArgs []string

	// Parse init-specific flags (-o, -force, -h/--help); everything else (or anything after --)
	// is treated as simulator arguments (such as -I, +incdir+, -D, +define+, -f, *.v).
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			simArgs = append(simArgs, args[i+1:]...)
			break
		}
		switch {
		case a == "-h" || a == "--help" || a == "-help":
			printInitUsage(out)
			return 0
		case a == "-force" || a == "--force":
			force = true
		case a == "-o" || a == "--o" || a == "-out" || a == "--out":
			if i+1 >= len(args) {
				fmt.Fprintln(out, "error: -o requires an output file path")
				return 2
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "-o="):
			outPath = strings.TrimPrefix(a, "-o=")
		default:
			simArgs = append(simArgs, a)
		}
	}

	if !force {
		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(out, "error: %s already exists (use -force to overwrite)\n", outPath)
			return 1
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(out, "error checking %s: %v\n", outPath, err)
			return 1
		}
	}

	baseDir := filepath.Dir(outPath)
	if baseDir == "" {
		baseDir = "."
	}

	var cfg config.Config
	var modeDesc string
	if len(simArgs) > 0 {
		cfg = config.FromArgsRaw(simArgs, baseDir)
		modeDesc = "from simulator arguments"
	} else {
		cfg = config.DiscoverRaw(baseDir)
		modeDesc = "by scanning workspace"
	}

	if err := config.SaveFile(outPath, cfg); err != nil {
		fmt.Fprintf(out, "error writing %s: %v\n", outPath, err)
		return 1
	}

	fmt.Fprintf(out, "Created %s %s (%d file(s), %d includeDir(s), %d filelist(s), %d define(s))\n",
		outPath, modeDesc, len(cfg.Files), len(cfg.IncludeDirs), len(cfg.Filelists), len(cfg.Defines))
	return 0
}

func printInitUsage(out io.Writer) {
	fmt.Fprintf(out, "Usage:\n")
	fmt.Fprintf(out, "  kvx init [-o kvx.json] [-force] [-- <simulator-args...>]\n\n")
	fmt.Fprintf(out, "Examples:\n")
	fmt.Fprintf(out, "  kvx init                                     Auto-discover files, headers, and .f filelists\n")
	fmt.Fprintf(out, "  kvx init -- -I rtl/inc -D SIM -f sim.f       Generate kvx.json from simulator arguments\n\n")
	fmt.Fprintf(out, "Flags:\n")
	fmt.Fprintf(out, "  -o <path>   Output config path (default: kvx.json)\n")
	fmt.Fprintf(out, "  -force      Overwrite existing config file\n")
}
