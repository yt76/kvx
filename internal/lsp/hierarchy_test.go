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

func TestCallHierarchyCrossFile(t *testing.T) {
	dir := t.TempDir()
	leafPath := filepath.Join(dir, "leaf.sv")
	midPath := filepath.Join(dir, "mid.sv")
	topPath := filepath.Join(dir, "top.sv")

	leafSrc := "module leaf (input logic clk); endmodule\n"
	midSrc := "module mid (input logic clk);\n  leaf u_leaf0 (.clk(clk));\n  leaf u_leaf1 (.clk(clk));\nendmodule\n"
	topSrc := "module top (input logic clk);\n  mid u_mid (.clk(clk));\nendmodule\n"

	for p, s := range map[string]string{leafPath: leafSrc, midPath: midSrc, topPath: topSrc} {
		if err := os.WriteFile(p, []byte(s), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	store := NewDocumentStore()
	if _, err := store.SetRootURI(filePathToURI(dir)); err != nil {
		t.Fatalf("SetRootURI: %v", err)
	}
	midDoc := store.Open(TextDocumentItem{
		URI:        filePathToURI(midPath),
		LanguageID: "systemverilog",
		Version:    1,
		Text:       midSrc,
	})

	// 1. prepareCallHierarchy on `mid` declaration (line 0, col 8)
	items := prepareCallHierarchy(midDoc, store, Position{Line: 0, Character: 8})
	if len(items) != 1 || items[0].Name != "mid" {
		t.Fatalf("prepareCallHierarchy(mid) = %+v, want [mid]", items)
	}

	// 2. incomingCalls for `mid` -> instantiated by `top` (1 site)
	incoming := incomingHierarchyCalls(store, items[0])
	if len(incoming) != 1 || incoming[0].From.Name != "top" || len(incoming[0].FromRanges) != 1 {
		t.Fatalf("incomingHierarchyCalls(mid) = %+v, want [top (1 range)]", incoming)
	}

	// 3. outgoingCalls for `mid` -> instantiates `leaf` (2 sites)
	outgoing := outgoingHierarchyCalls(store, items[0])
	if len(outgoing) != 1 || outgoing[0].To.Name != "leaf" || len(outgoing[0].FromRanges) != 2 {
		t.Fatalf("outgoingHierarchyCalls(mid) = %+v, want [leaf (2 ranges)]", outgoing)
	}

	// 4. prepareCallHierarchy on `leaf` inside instantiation `leaf u_leaf0` (line 1, col 3)
	leafItems := prepareCallHierarchy(midDoc, store, Position{Line: 1, Character: 3})
	if len(leafItems) != 1 || leafItems[0].Name != "leaf" || leafItems[0].URI != filePathToURI(leafPath) {
		t.Fatalf("prepareCallHierarchy(leaf inst) = %+v, want leaf in leaf.sv", leafItems)
	}
}

func TestLSPCallHierarchyRequests(t *testing.T) {
	var in bytes.Buffer
	writeMsg := func(v any) {
		if err := WriteMessage(&in, v); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}

	src := "module leaf (input logic clk); endmodule\n" +
		"module top (input logic clk);\n" +
		"  leaf u_leaf (.clk(clk));\n" +
		"endmodule\n"

	writeMsg(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	writeMsg(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        "file:///hier.sv",
				"languageId": "systemverilog",
				"version":    1,
				"text":       src,
			},
		},
	})
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/prepareCallHierarchy",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///hier.sv"},
			"position":     map[string]any{"line": 1, "character": 8},
		},
	})
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "callHierarchy/outgoingCalls",
		"params": map[string]any{
			"item": map[string]any{
				"name":           "top",
				"kind":           LSPSymbolKindModule,
				"uri":            "file:///hier.sv",
				"range":          Range{},
				"selectionRange": Range{},
			},
		},
	})
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "callHierarchy/incomingCalls",
		"params": map[string]any{
			"item": map[string]any{
				"name":           "leaf",
				"kind":           LSPSymbolKindModule,
				"uri":            "file:///hier.sv",
				"range":          Range{},
				"selectionRange": Range{},
			},
		},
	})
	writeMsg(map[string]any{"jsonrpc": "2.0", "id": 5, "method": "shutdown", "params": nil})
	writeMsg(map[string]any{"jsonrpc": "2.0", "method": "exit", "params": nil})

	var out bytes.Buffer
	srv := NewServer(&in, &out, log.New(io.Discard, "", 0))
	if err := srv.Run(); err != nil {
		t.Fatalf("srv.Run: %v", err)
	}

	var initResp InitializeResult
	var prep []CallHierarchyItem
	var outgoing []CallHierarchyOutgoingCall
	var incoming []CallHierarchyIncomingCall

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
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		switch string(env.ID) {
		case "1":
			_ = json.Unmarshal(env.Result, &initResp)
		case "2":
			_ = json.Unmarshal(env.Result, &prep)
		case "3":
			_ = json.Unmarshal(env.Result, &outgoing)
		case "4":
			_ = json.Unmarshal(env.Result, &incoming)
		}
	}

	if !initResp.Capabilities.CallHierarchyProvider {
		t.Errorf("expected CallHierarchyProvider=true")
	}
	if len(prep) != 1 || prep[0].Name != "top" {
		t.Errorf("prepareCallHierarchy = %+v, want [top]", prep)
	}
	if len(outgoing) != 1 || outgoing[0].To.Name != "leaf" {
		t.Errorf("outgoingCalls = %+v, want [leaf]", outgoing)
	}
	if len(incoming) != 1 || incoming[0].From.Name != "top" {
		t.Errorf("incomingCalls = %+v, want [top]", incoming)
	}
}
