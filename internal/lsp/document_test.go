package lsp

import (
	"bytes"
	"testing"
)

func TestDocumentStore(t *testing.T) {
	store := NewDocumentStore()
	uri := "file:///workspace/counter.v"

	// 1. Open
	store.Open(TextDocumentItem{
		URI:        uri,
		LanguageID: "verilog",
		Version:    1,
		Text:       "module counter;\nendmodule\n",
	})

	doc, ok := store.Get(uri)
	if !ok {
		t.Fatalf("expected document %q to exist after Open", uri)
	}
	if doc.Version != 1 || doc.Text != "module counter;\nendmodule\n" {
		t.Errorf("got doc=%+v", doc)
	}

	// 2. Change (Full sync)
	_, err := store.Change(
		VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		[]TextDocumentContentChangeEvent{
			{Text: "module counter(input clk);\nendmodule\n"},
		},
	)
	if err != nil {
		t.Fatalf("Change (full) failed: %v", err)
	}

	doc, _ = store.Get(uri)
	if doc.Version != 2 || doc.Text != "module counter(input clk);\nendmodule\n" {
		t.Errorf("after full change, got doc=%+v", doc)
	}

	// 3. Change (Incremental range sync)
	// Replace "counter" on line 0, cols 7..14 with "timer"
	_, err = store.Change(
		VersionedTextDocumentIdentifier{URI: uri, Version: 3},
		[]TextDocumentContentChangeEvent{
			{
				Range: &Range{
					Start: Position{Line: 0, Character: 7},
					End:   Position{Line: 0, Character: 14},
				},
				Text: "timer",
			},
		},
	)
	if err != nil {
		t.Fatalf("Change (incremental) failed: %v", err)
	}

	doc, _ = store.Get(uri)
	wantText := "module timer(input clk);\nendmodule\n"
	if doc.Version != 3 || doc.Text != wantText {
		t.Errorf("after incremental change, got version=%d text=%q, want version=3 text=%q", doc.Version, doc.Text, wantText)
	}

	// 4. Close
	store.Close(uri)
	if _, ok := store.Get(uri); ok {
		t.Errorf("expected document %q to be removed after Close", uri)
	}
}

func TestDocumentStoreErrors(t *testing.T) {
	store := NewDocumentStore()

	// Changing a document that isn't open should fail.
	_, err := store.Change(
		VersionedTextDocumentIdentifier{URI: "file:///missing.v", Version: 1},
		[]TextDocumentContentChangeEvent{{Text: "module m; endmodule"}},
	)
	if err == nil {
		t.Errorf("expected error when changing non-existent document, got nil")
	}

	// Open a document and test invalid range.
	uri := "file:///test.v"
	store.Open(TextDocumentItem{URI: uri, Version: 1, Text: "module m;\nendmodule"})

	tests := []struct {
		name  string
		rng   Range
		 replacement string
	}{
		{
			name: "out of range line",
			rng: Range{
				Start: Position{Line: 10, Character: 0},
				End:   Position{Line: 10, Character: 1},
			},
			replacement: "x",
		},
		{
			name: "start after end",
			rng: Range{
				Start: Position{Line: 0, Character: 5},
				End:   Position{Line: 0, Character: 2},
			},
			replacement: "x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.Change(
				VersionedTextDocumentIdentifier{URI: uri, Version: 2},
				[]TextDocumentContentChangeEvent{{Range: &tc.rng, Text: tc.replacement}},
			)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestServerDocumentSync(t *testing.T) {
	var in, out bytes.Buffer
	uri := "file:///workspace/alu.v"

	// 1. initialize
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	// 2. didOpen
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        uri,
				LanguageID: "verilog",
				Version:    1,
				Text:       "module alu;\nendmodule\n",
			},
		},
	})
	// 3. didChange
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": DidChangeTextDocumentParams{
			TextDocument: VersionedTextDocumentIdentifier{
				URI:     uri,
				Version: 2,
			},
			ContentChanges: []TextDocumentContentChangeEvent{
				{Text: "module alu(input a, output b);\n  assign b = a;\nendmodule\n"},
			},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	doc, ok := srv.GetDocument(uri)
	if !ok {
		t.Fatalf("expected document %q to be in server store", uri)
	}
	want := "module alu(input a, output b);\n  assign b = a;\nendmodule\n"
	if doc.Version != 2 || doc.Text != want {
		t.Errorf("got version=%d text=%q, want version=2 text=%q", doc.Version, doc.Text, want)
	}

	// Now send didClose and verify removal.
	in.Reset()
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didClose",
		"params": DidCloseTextDocumentParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
		},
	})
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error on didClose: %v", err)
	}
	if _, ok := srv.GetDocument(uri); ok {
		t.Errorf("expected document %q to be closed", uri)
	}
}
