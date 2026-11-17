package main

import (
	"errors"
	"io"
	"log"
	"os"

	"kvx/internal/lsp"
)

const envLogFile = "KVX_LOG"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && !isLSPModeArg(args[0]) {
		if args[0] == "init" {
			return runInit(args[1:], os.Stderr)
		}
		if args[0] == "check" {
			args = args[1:]
		} else if args[0] == "help" {
			args = []string{"-h"}
		}
		return runCheck(args, os.Stderr)
	}

	logger, cleanup, err := setupLogger(os.Getenv(envLogFile), os.Stderr)
	defer cleanup()
	if err != nil {
		logger.Printf("warning: failed to open %s=%q: %v (logging to stderr only)", envLogFile, os.Getenv(envLogFile), err)
	}

	server := lsp.NewServer(os.Stdin, os.Stdout, logger)
	if err := server.Run(); err != nil {
		if errors.Is(err, lsp.ErrExitWithoutShutdown) {
			logger.Println(err)
			return 1
		}
		logger.Printf("server error: %v", err)
		return 1
	}
	return 0
}

// setupLogger creates a logger writing to fallback (stderr), and additionally to logPath
// when logPath is non-empty. Never writes to stdout.
func setupLogger(logPath string, fallback io.Writer) (*log.Logger, func(), error) {
	if fallback == nil {
		fallback = io.Discard
	}
	cleanup := func() {}
	out := fallback

	var openErr error
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			openErr = err
		} else {
			out = io.MultiWriter(fallback, f)
			cleanup = func() { _ = f.Close() }
		}
	}

	logger := log.New(out, "[kvx] ", log.LstdFlags|log.Lshortfile)
	return logger, cleanup, openErr
}

func isLSPModeArg(arg string) bool {
	return arg == "-" || arg == "--stdio" || arg == "-stdio" || arg == "lsp"
}
