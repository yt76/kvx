package main

import (
	"errors"
	"log"
	"os"

	"kvx/internal/lsp"
)

func main() {
	args := os.Args[1:]

	if len(args) > 0 && !isLSPModeArg(args[0]) {
		if args[0] == "check" {
			args = args[1:]
		} else if args[0] == "help" {
			args = []string{"-h"}
		}
		os.Exit(runCheck(args, os.Stderr))
	}

	logger := log.New(os.Stderr, "[kvx] ", log.LstdFlags|log.Lshortfile)
	server := lsp.NewServer(os.Stdin, os.Stdout, logger)

	if err := server.Run(); err != nil {
		if errors.Is(err, lsp.ErrExitWithoutShutdown) {
			logger.Println(err)
			os.Exit(1)
		}
		logger.Fatalf("server error: %v", err)
	}
}

func isLSPModeArg(arg string) bool {
	return arg == "-" || arg == "--stdio" || arg == "-stdio" || arg == "lsp"
}
