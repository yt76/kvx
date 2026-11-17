package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func readFramedNotification(t *testing.T, r *bufio.Reader) (string, PublishDiagnosticsParams) {
	t.Helper()
	payload, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("failed to read notification message: %v", err)
	}
	var msg IncomingMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("failed to unmarshal PublishDiagnosticsParams: %v", err)
	}
	return msg.Method, params
}

func TestPublishDiagnosticsOnOpenAndChange(t *testing.T) {
	var in, out bytes.Buffer
	uri := "file:///workspace/broken.v"

	// 1. initialize
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	// 2. didOpen with a syntax error (missing semicolon after wire a)
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        uri,
				LanguageID: "verilog",
				Version:    1,
				Text:       "module m;\n  wire a\nendmodule\n",
			},
		},
	})
	// 3. didChange fixing the syntax error
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": DidChangeTextDocumentParams{
			TextDocument: VersionedTextDocumentIdentifier{
				URI:     uri,
				Version: 2,
			},
			ContentChanges: []TextDocumentContentChangeEvent{
				{Text: "module m;\n  wire a;\nendmodule\n"},
			},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)

	// Read initialize response first.
	_ = readFramedResponse(t, outReader)

	// Read diagnostics from didOpen (should have 1 error).
	method1, diag1 := readFramedNotification(t, outReader)
	if method1 != "textDocument/publishDiagnostics" {
		t.Fatalf("method1 = %q, want textDocument/publishDiagnostics", method1)
	}
	if diag1.URI != uri {
		t.Errorf("diag1.URI = %q, want %q", diag1.URI, uri)
	}
	if len(diag1.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic on broken open, got %d: %+v", len(diag1.Diagnostics), diag1.Diagnostics)
	}
	if !strings.Contains(diag1.Diagnostics[0].Message, "expected ';'") {
		t.Errorf("diagnostic message = %q, want substring \"expected ';'\"", diag1.Diagnostics[0].Message)
	}
	if diag1.Diagnostics[0].Range.Start.Line != 1 {
		t.Errorf("diagnostic line = %d, want 1", diag1.Diagnostics[0].Range.Start.Line)
	}

	// Read diagnostics from didChange (should be empty slice, clearing errors).
	method2, diag2 := readFramedNotification(t, outReader)
	if method2 != "textDocument/publishDiagnostics" {
		t.Fatalf("method2 = %q, want textDocument/publishDiagnostics", method2)
	}
	if len(diag2.Diagnostics) != 0 {
		t.Errorf("expected 0 diagnostics after fix, got %d: %+v", len(diag2.Diagnostics), diag2.Diagnostics)
	}
}
