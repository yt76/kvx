package main

import (
	"errors"
	"log"
	"os"

	"kvx/internal/lsp"
)

func main() {
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
