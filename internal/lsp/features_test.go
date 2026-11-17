package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLSPCoreFeatures(t *testing.T) {
	var in, out bytes.Buffer
	subURI := "file:///workspace/sub_alu.v"
	topURI := "file:///workspace/top.v"

	subSrc := "module sub_alu(\n" +
		"  input wire [7:0] a,\n" +
		"  output wire [7:0] y\n" +
		");\n" +
		"  assign y = a;\n" +
		"endmodule\n"

	topSrc := "module top #(parameter WIDTH = 8) (\n" +
		"  input wire clk,\n" +
		"  input wire [WIDTH-1:0] din,\n" +
		"  output reg [WIDTH-1:0] dout\n" +
		");\n" +
		"  wire [WIDTH-1:0] internal_bus;\n" +
		"  sub_alu u_alu (.a(din), .y(internal_bus));\n" +
		"  always @(posedge clk) begin\n" +
		"    dout <= internal_bus;\n" +
		"  end\n" +
		"endmodule\n"

	// 1. initialize
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	// 2. open sub_alu.v
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        subURI,
				LanguageID: "verilog",
				Version:    1,
				Text:       subSrc,
			},
		},
	})
	// 3. open top.v
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        topURI,
				LanguageID: "verilog",
				Version:    1,
				Text:       topSrc,
			},
		},
	})
	// 4. documentSymbol on top.v
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/documentSymbol",
		"params": DocumentSymbolParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
		},
	})
	// 5. hover on `internal_bus` usage at line 8, col 14 (`dout <= internal_bus;`)
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/hover",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 8, Character: 14},
		},
	})
	// 6. definition on `internal_bus` usage at line 8, col 14 -> should jump to line 5
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 8, Character: 14},
		},
	})
	// 7. definition on `sub_alu` module instantiation at line 6, col 3 -> should jump to sub_alu.v line 0!
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 6, Character: 3},
		},
	})
	// 8. completion inside `top` module at line 8, col 4
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "textDocument/completion",
		"params": CompletionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 8, Character: 4},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)

	// Consume initialize response (id=1) and two publishDiagnostics notifications.
	_ = readFramedResponse(t, outReader)
	_, _ = readFramedNotification(t, outReader)
	_, _ = readFramedNotification(t, outReader)

	// Verify documentSymbol response (id=2)
	respSym := readFramedResponse(t, outReader)
	rawSym, _ := json.Marshal(respSym.Result)
	var docSymbols []DocumentSymbol
	if err := json.Unmarshal(rawSym, &docSymbols); err != nil {
		t.Fatalf("unmarshaling DocumentSymbol: %v", err)
	}
	if len(docSymbols) != 1 || docSymbols[0].Name != "top" {
		t.Fatalf("expected 1 top-level module DocumentSymbol 'top', got %+v", docSymbols)
	}
	if len(docSymbols[0].Children) != 6 { // WIDTH, clk, din, dout, internal_bus, u_alu
		t.Errorf("expected 6 children symbols under 'top', got %d: %+v", len(docSymbols[0].Children), docSymbols[0].Children)
	}

	// Verify hover response (id=3)
	respHover := readFramedResponse(t, outReader)
	rawHover, _ := json.Marshal(respHover.Result)
	var hover Hover
	if err := json.Unmarshal(rawHover, &hover); err != nil {
		t.Fatalf("unmarshaling Hover: %v", err)
	}
	wantHover := "wire [WIDTH-1:0] internal_bus"
	if !strings.Contains(hover.Contents.Value, wantHover) {
		t.Errorf("Hover value = %q, want substring %q", hover.Contents.Value, wantHover)
	}

	// Verify intra-file definition response (id=4)
	respDef1 := readFramedResponse(t, outReader)
	rawDef1, _ := json.Marshal(respDef1.Result)
	var loc1 Location
	if err := json.Unmarshal(rawDef1, &loc1); err != nil {
		t.Fatalf("unmarshaling Location 1: %v", err)
	}
	if loc1.URI != topURI || loc1.Range.Start.Line != 5 {
		t.Errorf("definition of internal_bus = %+v, want URI=%s Line=5", loc1, topURI)
	}

	// Verify cross-file module definition response (id=5)
	respDef2 := readFramedResponse(t, outReader)
	rawDef2, _ := json.Marshal(respDef2.Result)
	var loc2 Location
	if err := json.Unmarshal(rawDef2, &loc2); err != nil {
		t.Fatalf("unmarshaling Location 2: %v", err)
	}
	if loc2.URI != subURI || loc2.Range.Start.Line != 0 {
		t.Errorf("definition of sub_alu = %+v, want URI=%s Line=0", loc2, subURI)
	}

	// Verify completion response (id=6)
	respComp := readFramedResponse(t, outReader)
	rawComp, _ := json.Marshal(respComp.Result)
	var compItems []CompletionItem
	if err := json.Unmarshal(rawComp, &compItems); err != nil {
		t.Fatalf("unmarshaling CompletionItem slice: %v", err)
	}
	foundInternalBus := false
	foundAlwaysKw := false
	for _, item := range compItems {
		if item.Label == "internal_bus" && item.Kind == CompletionKindVariable {
			foundInternalBus = true
		}
		if item.Label == "always" && item.Kind == CompletionKindKeyword {
			foundAlwaysKw = true
		}
	}
	if !foundInternalBus || !foundAlwaysKw {
		t.Errorf("completion items missing expected entries: foundInternalBus=%v foundAlwaysKw=%v",
			foundInternalBus, foundAlwaysKw)
	}
}

func TestSemanticTokensFull(t *testing.T) {
	var in, out bytes.Buffer
	uri := "file:///workspace/sem.v"
	src := "module sem #(parameter W = 8) (input wire a, output wire y);\n" +
		"  wire tmp;\n" +
		"  assign tmp = a;\n" +
		"  assign y = tmp;\n" +
		"endmodule\n"

	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        uri,
				LanguageID: "verilog",
				Version:    1,
				Text:       src,
			},
		},
	})
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/semanticTokens/full",
		"params": SemanticTokensParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)

	// 1. Check initialize response advertises SemanticTokensProvider.
	initResp := readFramedResponse(t, outReader)
	initBytes, _ := json.Marshal(initResp.Result)
	var initRes InitializeResult
	if err := json.Unmarshal(initBytes, &initRes); err != nil {
		t.Fatalf("unmarshaling InitializeResult: %v", err)
	}
	if initRes.Capabilities.SemanticTokensProvider == nil || !initRes.Capabilities.SemanticTokensProvider.Full {
		t.Fatalf("expected SemanticTokensProvider.Full=true, got %+v", initRes.Capabilities.SemanticTokensProvider)
	}

	// Consume publishDiagnostics notification from didOpen.
	_, _ = readFramedNotification(t, outReader)

	// 2. Check semanticTokens/full response.
	semResp := readFramedResponse(t, outReader)
	semBytes, _ := json.Marshal(semResp.Result)
	var semTokens SemanticTokens
	if err := json.Unmarshal(semBytes, &semTokens); err != nil {
		t.Fatalf("unmarshaling SemanticTokens: %v", err)
	}
	if len(semTokens.Data) == 0 || len(semTokens.Data)%5 != 0 {
		t.Fatalf("invalid semantic token data length: %d", len(semTokens.Data))
	}

	type decodedToken struct {
		line     int
		col      int
		length   int
		tokType  uint32
		tokMods  uint32
	}

	var decoded []decodedToken
	line, col := 0, 0
	for i := 0; i < len(semTokens.Data); i += 5 {
		dLine := int(semTokens.Data[i])
		dStart := int(semTokens.Data[i+1])
		length := int(semTokens.Data[i+2])
		tokType := semTokens.Data[i+3]
		tokMods := semTokens.Data[i+4]

		if dLine > 0 {
			line += dLine
			col = dStart
		} else {
			col += dStart
		}
		decoded = append(decoded, decodedToken{
			line:    line,
			col:     col,
			length:  length,
			tokType: tokType,
			tokMods: tokMods,
		})
	}

	// Verify first few tokens on line 0:
	// "module" (0:0, len 6, keyword)
	// "sem"    (0:7, len 3, type, declaration)
	// "#"      (0:11, len 1, operator)
	// "parameter" (0:13, len 9, keyword)
	// "W"      (0:23, len 1, parameter, declaration|readonly)
	// "="      (0:25, len 1, operator)
	// "8"      (0:27, len 1, number)
	wantPrefix := []decodedToken{
		{line: 0, col: 0, length: 6, tokType: semTokenKeyword, tokMods: 0},
		{line: 0, col: 7, length: 3, tokType: semTokenType, tokMods: semModDeclaration},
		{line: 0, col: 11, length: 1, tokType: semTokenOperator, tokMods: 0},
		{line: 0, col: 13, length: 9, tokType: semTokenKeyword, tokMods: 0},
		{line: 0, col: 23, length: 1, tokType: semTokenParameter, tokMods: semModDeclaration | semModReadonly},
		{line: 0, col: 25, length: 1, tokType: semTokenOperator, tokMods: 0},
		{line: 0, col: 27, length: 1, tokType: semTokenNumber, tokMods: 0},
	}

	if len(decoded) < len(wantPrefix) {
		t.Fatalf("got %d decoded tokens, want at least %d", len(decoded), len(wantPrefix))
	}
	for i, w := range wantPrefix {
		if decoded[i] != w {
			t.Errorf("decoded[%d] = %+v, want %+v", i, decoded[i], w)
		}
	}
}

func TestSemanticTokensDirectives(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open(TextDocumentItem{
		URI:        "file:///workspace/dir.v",
		LanguageID: "verilog",
		Version:    1,
		Text: "`include \"defs.vh\"\n" +
			"`define WIDTH 8\n" +
			"`ifdef SYNTH\n" +
			"`endif\n",
	})

	tokens := computeSemanticTokens(doc)
	if len(tokens.Data) == 0 || len(tokens.Data)%5 != 0 {
		t.Fatalf("invalid semantic token data length: %d", len(tokens.Data))
	}

	type decodedToken struct {
		line    int
		col     int
		length  int
		tokType uint32
		tokMods uint32
	}

	var decoded []decodedToken
	line, col := 0, 0
	for i := 0; i < len(tokens.Data); i += 5 {
		dLine := int(tokens.Data[i])
		dStart := int(tokens.Data[i+1])
		if dLine > 0 {
			line += dLine
			col = dStart
		} else {
			col += dStart
		}
		decoded = append(decoded, decodedToken{
			line:    line,
			col:     col,
			length:  int(tokens.Data[i+2]),
			tokType: tokens.Data[i+3],
			tokMods: tokens.Data[i+4],
		})
	}

	want := []decodedToken{
		// `include "defs.vh"
		{line: 0, col: 0, length: 8, tokType: semTokenMacro, tokMods: 0},
		{line: 0, col: 9, length: 9, tokType: semTokenString, tokMods: 0},
		// `define WIDTH 8
		{line: 1, col: 0, length: 7, tokType: semTokenMacro, tokMods: 0},
		{line: 1, col: 8, length: 5, tokType: semTokenMacro, tokMods: semModDeclaration},
		{line: 1, col: 14, length: 1, tokType: semTokenNumber, tokMods: 0},
		// `ifdef SYNTH
		{line: 2, col: 0, length: 6, tokType: semTokenMacro, tokMods: 0},
		{line: 2, col: 7, length: 5, tokType: semTokenMacro, tokMods: 0},
		// `endif
		{line: 3, col: 0, length: 6, tokType: semTokenMacro, tokMods: 0},
	}

	if len(decoded) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(decoded), len(want), decoded)
	}
	for i, w := range want {
		if decoded[i] != w {
			t.Errorf("decoded[%d] = %+v, want %+v", i, decoded[i], w)
		}
	}
}
