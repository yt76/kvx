package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestWatchedFilesReindexAndDiagnostics(t *testing.T) {
	dir := t.TempDir()
	subPath := filepath.Join(dir, "sub.sv")
	topPath := filepath.Join(dir, "top.sv")

	// Initially sub.sv has only port `clk` (missing `din`), so top.sv gets an unknown port diagnostic.
	if err := os.WriteFile(subPath, []byte("module sub (input logic clk); endmodule\n"), 0644); err != nil {
		t.Fatalf("WriteFile sub.sv: %v", err)
	}
	topSrc := "module top (input logic clk, input logic din);\n  sub u_sub (.clk(clk), .din(din));\nendmodule\n"
	if err := os.WriteFile(topPath, []byte(topSrc), 0644); err != nil {
		t.Fatalf("WriteFile top.sv: %v", err)
	}

	store := NewDocumentStore()
	if _, err := store.SetRootURI(filePathToURI(dir)); err != nil {
		t.Fatalf("SetRootURI: %v", err)
	}

	topDoc := store.Open(TextDocumentItem{
		URI:        filePathToURI(topPath),
		LanguageID: "systemverilog",
		Version:    1,
		Text:       topSrc,
	})
	if len(topDoc.Analysis.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic before sub.sv update, got %+v", topDoc.Analysis.Diagnostics)
	}

	// 1. Externally update sub.sv to add port `din` and fire FileChangeChanged.
	if err := os.WriteFile(subPath, []byte("module sub (input logic clk, input logic din); endmodule\n"), 0644); err != nil {
		t.Fatalf("WriteFile sub.sv update: %v", err)
	}
	updated := store.HandleWatchedFileChanges([]FileEvent{{
		URI:  filePathToURI(subPath),
		Type: FileChangeChanged,
	}})
	if len(updated) != 1 {
		t.Fatalf("expected 1 refreshed open document, got %d", len(updated))
	}
	if len(updated[0].Analysis.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics after sub.sv update, got %+v", updated[0].Analysis.Diagnostics)
	}

	// 2. Externally create a new package file and verify it's indexed.
	pkgPath := filepath.Join(dir, "extra_pkg.sv")
	if err := os.WriteFile(pkgPath, []byte("package extra_pkg; localparam int K = 1; endpackage\n"), 0644); err != nil {
		t.Fatalf("WriteFile extra_pkg.sv: %v", err)
	}
	store.HandleWatchedFileChanges([]FileEvent{{
		URI:  filePathToURI(pkgPath),
		Type: FileChangeCreated,
	}})
	if _, ok := store.FindPackageURI("extra_pkg"); !ok {
		t.Errorf("expected extra_pkg to be indexed after FileChangeCreated")
	}

	// 3. Externally delete sub.sv and verify it's removed from workspace index.
	_ = os.Remove(subPath)
	store.HandleWatchedFileChanges([]FileEvent{{
		URI:  filePathToURI(subPath),
		Type: FileChangeDeleted,
	}})
	if _, ok := store.FindModuleScope("sub"); ok {
		t.Errorf("expected module sub to be removed after FileChangeDeleted")
	}
}

func TestWatchedFilesHeaderAndConfigRescan(t *testing.T) {
	dir := t.TempDir()
	hdrPath := filepath.Join(dir, "defs.vh")
	topPath := filepath.Join(dir, "top.sv")
	cfgPath := filepath.Join(dir, "kvx.json")

	if err := os.WriteFile(hdrPath, []byte("`define MY_W 8\n"), 0644); err != nil {
		t.Fatalf("WriteFile defs.vh: %v", err)
	}
	topSrc := "`include \"defs.vh\"\nmodule top;\n  localparam int W = `MY_W;\n`ifdef EXTRA_DEF\n  localparam int E = `EXTRA_DEF;\n`endif\nendmodule\n"
	if err := os.WriteFile(topPath, []byte(topSrc), 0644); err != nil {
		t.Fatalf("WriteFile top.sv: %v", err)
	}

	store := NewDocumentStore()
	if _, err := store.SetRootURI(filePathToURI(dir)); err != nil {
		t.Fatalf("SetRootURI: %v", err)
	}
	doc := store.Open(TextDocumentItem{
		URI:        filePathToURI(topPath),
		LanguageID: "systemverilog",
		Version:    1,
		Text:       topSrc,
	})
	if len(doc.Analysis.Diagnostics) != 0 {
		t.Fatalf("expected 0 initial diagnostics, got %+v", doc.Analysis.Diagnostics)
	}

	// Create kvx.json adding EXTRA_DEF and fire FileChangeCreated on kvx.json.
	if err := os.WriteFile(cfgPath, []byte(`{"defines":{"EXTRA_DEF":"16"}}`), 0644); err != nil {
		t.Fatalf("WriteFile kvx.json: %v", err)
	}
	updated := store.HandleWatchedFileChanges([]FileEvent{{
		URI:  filePathToURI(cfgPath),
		Type: FileChangeCreated,
	}})
	if len(updated) != 1 {
		t.Fatalf("expected 1 refreshed open document after kvx.json change, got %d", len(updated))
	}
	if _, exists := updated[0].Analysis.Modules["top"].Symbols["E"]; !exists {
		t.Errorf("expected localparam E to be active after kvx.json define update")
	}
}

func TestLSPWatchedFilesDynamicRegistrationAndNotification(t *testing.T) {
	dir := t.TempDir()
	subPath := filepath.Join(dir, "sub.sv")
	topPath := filepath.Join(dir, "top.sv")

	if err := os.WriteFile(subPath, []byte("module sub (input logic clk); endmodule\n"), 0644); err != nil {
		t.Fatalf("WriteFile sub.sv: %v", err)
	}
	topSrc := "module top (input logic clk, input logic din);\n  sub u_sub (.clk(clk), .din(din));\nendmodule\n"
	if err := os.WriteFile(topPath, []byte(topSrc), 0644); err != nil {
		t.Fatalf("WriteFile top.sv: %v", err)
	}

	var in bytes.Buffer
	writeMsg := func(v any) {
		if err := WriteMessage(&in, v); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}

	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(dir),
			"capabilities": map[string]any{
				"workspace": map[string]any{
					"didChangeWatchedFiles": map[string]any{
						"dynamicRegistration": true,
					},
				},
			},
		},
	})
	writeMsg(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	// Simulate client response to client/registerCapability
	writeMsg(map[string]any{"jsonrpc": "2.0", "id": "kvx-watch-files", "result": nil})
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        filePathToURI(topPath),
				"languageId": "systemverilog",
				"version":    1,
				"text":       topSrc,
			},
		},
	})

	// Update sub.sv on disk before server processes didChangeWatchedFiles notification
	if err := os.WriteFile(subPath, []byte("module sub (input logic clk, input logic din); endmodule\n"), 0644); err != nil {
		t.Fatalf("WriteFile sub.sv update: %v", err)
	}
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWatchedFiles",
		"params": map[string]any{
			"changes": []map[string]any{
				{"uri": filePathToURI(subPath), "type": FileChangeChanged},
			},
		},
	})
	writeMsg(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown", "params": nil})
	writeMsg(map[string]any{"jsonrpc": "2.0", "method": "exit", "params": nil})

	var out bytes.Buffer
	srv := NewServer(&in, &out, log.New(io.Discard, "", 0))
	if err := srv.Run(); err != nil {
		t.Fatalf("srv.Run: %v", err)
	}

	sawRegister := false
	diagCounts := []int{}

	reader := bufio.NewReader(&out)
	for {
		raw, err := ReadMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		var env struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		switch env.Method {
		case "client/registerCapability":
			sawRegister = true
		case "textDocument/publishDiagnostics":
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(env.Params, &p); err != nil {
				t.Fatalf("Unmarshal publishDiagnostics: %v", err)
			}
			diagCounts = append(diagCounts, len(p.Diagnostics))
		}
	}

	if !sawRegister {
		t.Errorf("expected server to send client/registerCapability for workspace/didChangeWatchedFiles")
	}
	if len(diagCounts) < 2 {
		t.Fatalf("expected at least 2 publishDiagnostics notifications (didOpen + didChangeWatchedFiles), got %v", diagCounts)
	}
	if diagCounts[len(diagCounts)-1] != 0 {
		t.Errorf("expected 0 diagnostics after didChangeWatchedFiles, got %v", diagCounts)
	}
}
