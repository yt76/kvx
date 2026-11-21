package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestLSPIncludeHoverAndDefinition(t *testing.T) {
	tmpDir := t.TempDir()
	defsPath := filepath.Join(tmpDir, "defs.vh")
	defsContent := "`define BUS_W 32\n" +
		"localparam INIT_SEED = 32'h1234;\n"
	if err := os.WriteFile(defsPath, []byte(defsContent), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(tmpDir, "top.v")
	topURI := filePathToURI(topPath)
	defsURI := filePathToURI(defsPath)
	topSrc := "`include \"defs.vh\"\n" +
		"module top(\n" +
		"  input wire [`BUS_W-1:0] a,\n" +
		"  output wire [`BUS_W-1:0] y\n" +
		");\n" +
		"  assign y = a ^ INIT_SEED;\n" +
		"endmodule\n"

	var in, out bytes.Buffer

	// 1. initialize
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(tmpDir),
		},
	})
	// 2. didOpen top.v
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
	// 3. definition on `include "defs.vh" at line 0, col 10 -> should jump to defs.vh line 0
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 0, Character: 10},
		},
	})
	// 4. definition on `BUS_W at line 2, col 16 -> should jump to defs.vh line 0
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 2, Character: 16},
		},
	})
	// 5. definition on INIT_SEED at line 5, col 18 -> should jump to defs.vh line 1
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 5, Character: 18},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	// Check publishDiagnostics has 0 diagnostics (INIT_SEED and `BUS_W resolved via defs.vh).
	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics on top.v with valid include, got %+v", diagParams.Diagnostics)
	}

	// Definition on `include "defs.vh" (id=2)
	respInc := readFramedResponse(t, outReader)
	rawInc, _ := json.Marshal(respInc.Result)
	var locInc Location
	if err := json.Unmarshal(rawInc, &locInc); err != nil {
		t.Fatalf("unmarshaling Location for include: %v", err)
	}
	if locInc.URI != defsURI || locInc.Range.Start.Line != 0 {
		t.Errorf("include definition = %+v, want URI=%s Line=0", locInc, defsURI)
	}

	// Definition on `BUS_W (id=3)
	respMacro := readFramedResponse(t, outReader)
	rawMacro, _ := json.Marshal(respMacro.Result)
	var locMacro Location
	if err := json.Unmarshal(rawMacro, &locMacro); err != nil {
		t.Fatalf("unmarshaling Location for `BUS_W: %v", err)
	}
	if locMacro.URI != defsURI || locMacro.Range.Start.Line != 0 {
		t.Errorf("`BUS_W definition = %+v, want URI=%s Line=0", locMacro, defsURI)
	}

	// Definition on INIT_SEED (id=4)
	respParam := readFramedResponse(t, outReader)
	rawParam, _ := json.Marshal(respParam.Result)
	var locParam Location
	if err := json.Unmarshal(rawParam, &locParam); err != nil {
		t.Fatalf("unmarshaling Location for INIT_SEED: %v", err)
	}
	if locParam.URI != defsURI || locParam.Range.Start.Line != 1 {
		t.Errorf("INIT_SEED definition = %+v, want URI=%s Line=1", locParam, defsURI)
	}
}

func TestLSPConfigIncludeDirs(t *testing.T) {
	root := t.TempDir()
	incDir := filepath.Join(root, "inc")
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(incDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	headerPath := filepath.Join(incDir, "params.vh")
	if err := os.WriteFile(headerPath, []byte("`define WIDTH 16\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, "kvx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"includeDirs": ["inc"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(rtlDir, "top.v")
	topURI := filePathToURI(topPath)
	headerURI := filePathToURI(headerPath)
	topSrc := "`include \"params.vh\"\n" +
		"module top(input wire [`WIDTH-1:0] din, output wire [`WIDTH-1:0] dout);\n" +
		"  assign dout = din;\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
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
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 0, Character: 10},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics when inc is configured in kvx.json, got %+v", diagParams.Diagnostics)
	}

	respDef := readFramedResponse(t, outReader)
	rawDef, _ := json.Marshal(respDef.Result)
	var loc Location
	if err := json.Unmarshal(rawDef, &loc); err != nil {
		t.Fatalf("unmarshaling Location: %v", err)
	}
	if loc.URI != headerURI {
		t.Errorf("include definition URI = %q, want %q", loc.URI, headerURI)
	}
}

func TestSemanticTokensComments(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open(TextDocumentItem{
		URI:        "file:///workspace/comments.v",
		LanguageID: "verilog",
		Version:    1,
		Text: "// header comment\n" +
			"`define /* inline */ W 8\n" +
			"/* multi\n" +
			"   line */\n",
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
		// // header comment
		{line: 0, col: 0, length: 17, tokType: semTokenComment, tokMods: 0},
		// `define /* inline */ W 8
		{line: 1, col: 0, length: 7, tokType: semTokenMacro, tokMods: 0},
		{line: 1, col: 8, length: 12, tokType: semTokenComment, tokMods: 0},
		{line: 1, col: 21, length: 1, tokType: semTokenMacro, tokMods: semModDeclaration},
		{line: 1, col: 23, length: 1, tokType: semTokenNumber, tokMods: 0},
		// /* multi
		{line: 2, col: 0, length: 8, tokType: semTokenComment, tokMods: 0},
		//    line */
		{line: 3, col: 0, length: 10, tokType: semTokenComment, tokMods: 0},
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

func TestLSPWorkspaceFilesIndex(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	subPath := filepath.Join(rtlDir, "sub_mod.v")
	subSrc := "module sub_mod(input wire a, output wire y);\n" +
		"  assign y = ~a;\n" +
		"endmodule\n"
	if err := os.WriteFile(subPath, []byte(subSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// kvx.json lists rtl/sub_mod.v; note that we will NOT call didOpen on sub_mod.v.
	cfgPath := filepath.Join(root, "kvx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"files": ["rtl/*.v"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(rtlDir, "top.v")
	topURI := filePathToURI(topPath)
	subURI := filePathToURI(subPath)
	topSrc := "module top(input wire din, output wire dout);\n" +
		"  sub_mod u_sub (.a(din), .y(dout));\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
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
	// Hover on `sub_mod` at line 1, col 3 -> should show sub_mod signature from indexed file.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/hover",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 1, Character: 3},
		},
	})
	// Definition on `sub_mod` at line 1, col 3 -> should jump to sub_mod.v line 0.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 1, Character: 3},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader)     // initialize
	_, _ = readFramedNotification(t, outReader) // publishDiagnostics

	respHover := readFramedResponse(t, outReader)
	rawHover, _ := json.Marshal(respHover.Result)
	var hover Hover
	if err := json.Unmarshal(rawHover, &hover); err != nil {
		t.Fatalf("unmarshaling Hover: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "module sub_mod (input wire a, output wire y)") {
		t.Errorf("Hover value = %q, want sub_mod signature", hover.Contents.Value)
	}

	respDef := readFramedResponse(t, outReader)
	rawDef, _ := json.Marshal(respDef.Result)
	var loc Location
	if err := json.Unmarshal(rawDef, &loc); err != nil {
		t.Fatalf("unmarshaling Location: %v", err)
	}
	if loc.URI != subURI || loc.Range.Start.Line != 0 {
		t.Errorf("definition = %+v, want URI=%s Line=0", loc, subURI)
	}
}

func TestLSPConfigDefines(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "kvx.json")
	cfgJSON := `{
  "defines": {
    "USE_PORT_B": "",
    "DATA_W": "16"
  }
}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(root, "top.v")
	topURI := filePathToURI(topPath)
	topSrc := "module top(\n" +
		"  input wire [`DATA_W-1:0] a,\n" +
		"`ifdef USE_PORT_B\n" +
		"  input wire [`DATA_W-1:0] b,\n" +
		"`endif\n" +
		"  output wire [`DATA_W-1:0] y\n" +
		");\n" +
		"  assign y = a & b;\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
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
	// Hover on port `b` in `assign y = a & b;` (line 7, col 17) -> should resolve `[16-1:0]`!
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/hover",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 7, Character: 17},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics when defines are set in kvx.json, got %+v", diagParams.Diagnostics)
	}

	respHover := readFramedResponse(t, outReader)
	rawHover, _ := json.Marshal(respHover.Result)
	var hover Hover
	if err := json.Unmarshal(rawHover, &hover); err != nil {
		t.Fatalf("unmarshaling Hover: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "input wire [16-1:0] b") {
		t.Errorf("Hover value = %q, want expanded [16-1:0] for port b", hover.Contents.Value)
	}
}

func TestLSPZeroConfigAutoDiscovery(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	incDir := filepath.Join(root, "inc")
	for _, d := range []string{rtlDir, incDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// No kvx.json is created!
	_ = os.WriteFile(filepath.Join(incDir, "defs.vh"), []byte("`define WIDTH 8\n"), 0644)
	subPath := filepath.Join(rtlDir, "sub_unit.v")
	_ = os.WriteFile(subPath, []byte("module sub_unit(input wire a, output wire y); assign y = a; endmodule\n"), 0644)

	topPath := filepath.Join(rtlDir, "top.v")
	topURI := filePathToURI(topPath)
	subURI := filePathToURI(subPath)
	topSrc := "`include \"defs.vh\"\n" +
		"module top(input wire [`WIDTH-1:0] din, output wire dout);\n" +
		"  sub_unit u_sub (.a(din[0]), .y(dout));\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
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
	// Definition on `sub_unit` at line 2, col 3 -> should resolve via zero-config auto-discovery.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 2, Character: 3},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics in zero-config mode, got %+v", diagParams.Diagnostics)
	}

	respDef := readFramedResponse(t, outReader)
	rawDef, _ := json.Marshal(respDef.Result)
	var loc Location
	if err := json.Unmarshal(rawDef, &loc); err != nil {
		t.Fatalf("unmarshaling Location: %v", err)
	}
	if loc.URI != subURI || loc.Range.Start.Line != 0 {
		t.Errorf("definition = %+v, want URI=%s Line=0", loc, subURI)
	}
}

func TestLSPStartupScanWithConfigWithoutFilesAndInstanceJump(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	incDir := filepath.Join(root, "inc")
	for _, d := range []string{rtlDir, incDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// kvx.json specifies ONLY includeDirs (no "files" field).
	// Startup scan must still discover and index all .v files in the workspace.
	_ = os.WriteFile(filepath.Join(root, "kvx.json"), []byte(`{"includeDirs": ["inc"]}`), 0644)
	_ = os.WriteFile(filepath.Join(incDir, "defs.vh"), []byte("`define WIDTH 8\n"), 0644)
	subPath := filepath.Join(rtlDir, "sub_unit.v")
	_ = os.WriteFile(subPath, []byte("module sub_unit(input wire a, output wire y);\n  assign y = a;\nendmodule\n"), 0644)

	topPath := filepath.Join(rtlDir, "top.v")
	topURI := filePathToURI(topPath)
	subURI := filePathToURI(subPath)
	topSrc := "`include \"defs.vh\"\n" +
		"module top(input wire [`WIDTH-1:0] din, output wire dout);\n" +
		"  sub_unit u_sub (.a(din[0]), .y(dout));\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	// Pass workspaceFolders (and leave rootUri empty) to verify modern client initialization.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"workspaceFolders": []map[string]any{
				{"uri": filePathToURI(root), "name": "test"},
			},
		},
	})
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
	// 1. Definition on module type name `sub_unit` (line 2, col 3)
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 2, Character: 3},
		},
	})
	// 2. Definition on instance name `u_sub` (line 2, col 12) -> should also jump to sub_unit.v!
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 2, Character: 12},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader)        // initialize
	_, _ = readFramedNotification(t, outReader) // publishDiagnostics

	for _, id := range []string{"2", "3"} {
		respDef := readFramedResponse(t, outReader)
		if string(respDef.ID) != id {
			t.Fatalf("got response ID=%s, want %s", string(respDef.ID), id)
		}
		rawDef, _ := json.Marshal(respDef.Result)
		var loc Location
		if err := json.Unmarshal(rawDef, &loc); err != nil {
			t.Fatalf("unmarshaling Location (id=%s): %v", id, err)
		}
		if loc.URI != subURI || loc.Range.Start.Line != 0 {
			t.Errorf("definition (id=%s) = %+v, want URI=%s Line=0", id, loc, subURI)
		}
	}
}

func TestLSPModuleReferences(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	subPath := filepath.Join(rtlDir, "sub_unit.v")
	subSrc := "module sub_unit(input wire a, output wire y);\n" +
		"  assign y = a;\n" +
		"endmodule\n"
	if err := os.WriteFile(subPath, []byte(subSrc), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(rtlDir, "top.v")
	topSrc := "module top(input wire [1:0] din, output wire [1:0] dout);\n" +
		"  sub_unit u_sub0 (.a(din[0]), .y(dout[0]));\n" +
		"  generate\n" +
		"    if (1) begin : g_sub\n" +
		"      sub_unit u_sub1 (.a(din[1]), .y(dout[1]));\n" +
		"    end\n" +
		"  endgenerate\n" +
		"endmodule\n"
	if err := os.WriteFile(topPath, []byte(topSrc), 0644); err != nil {
		t.Fatal(err)
	}

	otherPath := filepath.Join(rtlDir, "other.v")
	otherSrc := "module other(input wire x, output wire z);\n" +
		"  sub_unit u_other (.a(x), .y(z));\n" +
		"endmodule\n"
	if err := os.WriteFile(otherPath, []byte(otherSrc), 0644); err != nil {
		t.Fatal(err)
	}

	subURI := filePathToURI(subPath)
	topURI := filePathToURI(topPath)
	otherURI := filePathToURI(otherPath)

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
	// Open only sub_unit.v; top.v and other.v remain indexed workspace files.
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
	// Request references on `sub_unit` in `module sub_unit(...)` (line 0, col 9) without declaration.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/references",
		"params": ReferenceParams{
			TextDocument: TextDocumentIdentifier{URI: subURI},
			Position:     Position{Line: 0, Character: 9},
			Context:      ReferenceContext{IncludeDeclaration: false},
		},
	})
	// Request references with includeDeclaration: true.
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/references",
		"params": ReferenceParams{
			TextDocument: TextDocumentIdentifier{URI: subURI},
			Position:     Position{Line: 0, Character: 9},
			Context:      ReferenceContext{IncludeDeclaration: true},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	respInit := readFramedResponse(t, outReader)
	rawInit, _ := json.Marshal(respInit.Result)
	var initRes InitializeResult
	if err := json.Unmarshal(rawInit, &initRes); err != nil {
		t.Fatalf("unmarshaling InitializeResult: %v", err)
	}
	if !initRes.Capabilities.ReferencesProvider {
		t.Fatalf("expected ReferencesProvider capability to be true")
	}

	_, _ = readFramedNotification(t, outReader) // publishDiagnostics

	// Response id=2 (includeDeclaration: false -> 3 instantiation sites: other.v:1, top.v:1, top.v:4)
	respRef2 := readFramedResponse(t, outReader)
	rawRef2, _ := json.Marshal(respRef2.Result)
	var locs2 []Location
	if err := json.Unmarshal(rawRef2, &locs2); err != nil {
		t.Fatalf("unmarshaling references (id=2): %v", err)
	}
	if len(locs2) != 3 {
		t.Fatalf("id=2: got %d references, want 3: %+v", len(locs2), locs2)
	}
	wantLocs2 := []struct {
		uri  string
		line int
	}{
		{otherURI, 1},
		{topURI, 1},
		{topURI, 4},
	}
	for i, want := range wantLocs2 {
		if locs2[i].URI != want.uri || locs2[i].Range.Start.Line != want.line {
			t.Errorf("id=2 ref[%d] = (%s, line %d), want (%s, line %d)",
				i, locs2[i].URI, locs2[i].Range.Start.Line, want.uri, want.line)
		}
	}

	// Response id=3 (includeDeclaration: true -> 4 locations including sub_unit.v:0)
	respRef3 := readFramedResponse(t, outReader)
	rawRef3, _ := json.Marshal(respRef3.Result)
	var locs3 []Location
	if err := json.Unmarshal(rawRef3, &locs3); err != nil {
		t.Fatalf("unmarshaling references (id=3): %v", err)
	}
	if len(locs3) != 4 {
		t.Fatalf("id=3: got %d references, want 4: %+v", len(locs3), locs3)
	}
	foundDecl := false
	for _, loc := range locs3 {
		if loc.URI == subURI && loc.Range.Start.Line == 0 {
			foundDecl = true
		}
	}
	if !foundDecl {
		t.Errorf("id=3: expected declaration in %s line 0, got %+v", subURI, locs3)
	}
}

func TestLSPDocumentSymbolSkipsUnnamedIncompleteModule(t *testing.T) {
	uri := "file:///tmp/incomplete.v"
	// First module is incomplete without a name (`module (`), second module is valid.
	src := "module (\n" +
		"  input wire a\n" +
		");\n" +
		"endmodule\n" +
		"module valid_mod(input wire x, output wire y);\n" +
		"  assign y = x;\n" +
		"endmodule\n"

	var in, out bytes.Buffer
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
		"method":  "textDocument/documentSymbol",
		"params": DocumentSymbolParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader)        // initialize
	_, _ = readFramedNotification(t, outReader) // publishDiagnostics

	respSym := readFramedResponse(t, outReader)
	rawSym, _ := json.Marshal(respSym.Result)
	var syms []DocumentSymbol
	if err := json.Unmarshal(rawSym, &syms); err != nil {
		t.Fatalf("unmarshaling DocumentSymbol[]: %v", err)
	}
	if len(syms) != 1 || syms[0].Name != "valid_mod" {
		t.Fatalf("expected only valid_mod in documentSymbol, got %+v", syms)
	}
}

func TestLSPCrossFilePackageResolution(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	pkgPath := filepath.Join(rtlDir, "defs_pkg.sv")
	pkgSrc := "package defs_pkg;\n" +
		"  localparam int DATA_W = 16;\n" +
		"  typedef enum logic { IDLE, BUSY } state_t;\n" +
		"endpackage\n"
	if err := os.WriteFile(pkgPath, []byte(pkgSrc), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(rtlDir, "top.sv")
	topSrc := "module top import defs_pkg::*; (\n" +
		"  input  logic [DATA_W-1:0] din,\n" +
		"  output logic [defs_pkg::DATA_W-1:0] dout,\n" +
		"  output state_t st\n" +
		");\n" +
		"  assign dout = din;\n" +
		"  assign st   = IDLE;\n" +
		"endmodule\n"
	if err := os.WriteFile(topPath, []byte(topSrc), 0644); err != nil {
		t.Fatal(err)
	}

	pkgURI := filePathToURI(pkgPath)
	topURI := filePathToURI(topPath)

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        topURI,
				LanguageID: "systemverilog",
				Version:    1,
				Text:       topSrc,
			},
		},
	})
	// 1. Go to definition on `defs_pkg::DATA_W` (line 2, col 27) -> defs_pkg.sv line 1
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 2, Character: 27},
		},
	})
	// 2. Go to definition on imported enum member `IDLE` (line 6, col 17) -> defs_pkg.sv line 2
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: topURI},
			Position:     Position{Line: 6, Character: 17},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for cross-file package import, got %+v", diagParams.Diagnostics)
	}

	// Check definition id=2 (DATA_W -> defs_pkg.sv line 1)
	respDef2 := readFramedResponse(t, outReader)
	raw2, _ := json.Marshal(respDef2.Result)
	var loc2 Location
	if err := json.Unmarshal(raw2, &loc2); err != nil {
		t.Fatalf("unmarshaling loc2: %v", err)
	}
	if loc2.URI != pkgURI || loc2.Range.Start.Line != 1 {
		t.Errorf("definition(DATA_W) = %+v, want URI=%s line=1", loc2, pkgURI)
	}

	// Check definition id=3 (IDLE -> defs_pkg.sv line 2)
	respDef3 := readFramedResponse(t, outReader)
	raw3, _ := json.Marshal(respDef3.Result)
	var loc3 Location
	if err := json.Unmarshal(raw3, &loc3); err != nil {
		t.Fatalf("unmarshaling loc3: %v", err)
	}
	if loc3.URI != pkgURI || loc3.Range.Start.Line != 2 {
		t.Errorf("definition(IDLE) = %+v, want URI=%s line=2", loc3, pkgURI)
	}
}

func TestLSPCrossFileInterfaceResolution(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	ifPath := filepath.Join(rtlDir, "bus_if.sv")
	ifSrc := "interface bus_if (input logic clk);\n" +
		"  logic valid;\n" +
		"  logic ready;\n" +
		"  modport master (input clk, ready, output valid);\n" +
		"endinterface\n"
	if err := os.WriteFile(ifPath, []byte(ifSrc), 0644); err != nil {
		t.Fatal(err)
	}

	prodPath := filepath.Join(rtlDir, "producer.sv")
	prodSrc := "module producer (bus_if.master bus, input logic en);\n" +
		"  assign bus.valid = en;\n" +
		"endmodule\n"
	if err := os.WriteFile(prodPath, []byte(prodSrc), 0644); err != nil {
		t.Fatal(err)
	}

	ifURI := filePathToURI(ifPath)
	prodURI := filePathToURI(prodPath)

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        prodURI,
				LanguageID: "systemverilog",
				Version:    1,
				Text:       prodSrc,
			},
		},
	})
	// 1. Go to definition on `bus_if` (line 0, col 18) -> bus_if.sv line 0
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: prodURI},
			Position:     Position{Line: 0, Character: 18},
		},
	})
	// 2. Go to definition on `master` (line 0, col 25) -> bus_if.sv line 3
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "textDocument/definition",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: prodURI},
			Position:     Position{Line: 0, Character: 25},
		},
	})
	// 3. Hover on `master` (line 0, col 25) -> shows modport signature
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "textDocument/hover",
		"params": TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: prodURI},
			Position:     Position{Line: 0, Character: 25},
		},
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	_ = readFramedResponse(t, outReader) // initialize

	_, diagParams := readFramedNotification(t, outReader)
	if len(diagParams.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for cross-file interface port, got %+v", diagParams.Diagnostics)
	}

	// Definition id=2: bus_if -> bus_if.sv line 0
	respDef2 := readFramedResponse(t, outReader)
	raw2, _ := json.Marshal(respDef2.Result)
	var loc2 Location
	if err := json.Unmarshal(raw2, &loc2); err != nil {
		t.Fatalf("unmarshaling loc2: %v", err)
	}
	if loc2.URI != ifURI || loc2.Range.Start.Line != 0 {
		t.Errorf("definition(bus_if) = %+v, want URI=%s line=0", loc2, ifURI)
	}

	// Definition id=3: master -> bus_if.sv line 3
	respDef3 := readFramedResponse(t, outReader)
	raw3, _ := json.Marshal(respDef3.Result)
	var loc3 Location
	if err := json.Unmarshal(raw3, &loc3); err != nil {
		t.Fatalf("unmarshaling loc3: %v", err)
	}
	if loc3.URI != ifURI || loc3.Range.Start.Line != 3 {
		t.Errorf("definition(master) = %+v, want URI=%s line=3", loc3, ifURI)
	}

	// Hover id=4: master -> contains "modport master"
	respHover := readFramedResponse(t, outReader)
	raw4, _ := json.Marshal(respHover.Result)
	var hover Hover
	if err := json.Unmarshal(raw4, &hover); err != nil {
		t.Fatalf("unmarshaling hover: %v", err)
	}
	if !strings.Contains(hover.Contents.Value, "modport master") {
		t.Errorf("hover(master) = %q, want to contain 'modport master'", hover.Contents.Value)
	}
}

func TestLSPContextAwareCompletion(t *testing.T) {
	root := t.TempDir()
	rtlDir := filepath.Join(root, "rtl")
	if err := os.MkdirAll(rtlDir, 0755); err != nil {
		t.Fatal(err)
	}

	pkgPath := filepath.Join(rtlDir, "defs_pkg.sv")
	pkgSrc := "package defs_pkg;\n" +
		"  localparam int DATA_W = 16;\n" +
		"  typedef struct packed {\n" +
		"    logic [7:0] addr;\n" +
		"    logic       valid;\n" +
		"  } pkt_t;\n" +
		"endpackage\n"
	if err := os.WriteFile(pkgPath, []byte(pkgSrc), 0644); err != nil {
		t.Fatal(err)
	}

	ifPath := filepath.Join(rtlDir, "bus_if.sv")
	ifSrc := "interface bus_if (input logic clk);\n" +
		"  logic valid;\n" +
		"  logic ready;\n" +
		"  modport master (input clk, ready, output valid);\n" +
		"  modport slave  (input clk, valid, output ready);\n" +
		"endinterface\n"
	if err := os.WriteFile(ifPath, []byte(ifSrc), 0644); err != nil {
		t.Fatal(err)
	}

	subPath := filepath.Join(rtlDir, "sub_unit.sv")
	subSrc := "module sub_unit #(parameter int WIDTH = 8) (input logic a, output logic y);\n" +
		"  assign y = a;\n" +
		"endmodule\n"
	if err := os.WriteFile(subPath, []byte(subSrc), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(rtlDir, "top.sv")
	topURI := filePathToURI(topPath)
	// Lines:
	// 0: module top (bus_if.master bus, input logic clk);
	// 1:   defs_pkg::pkt_t pkt;
	// 2:   logic x;
	// 3:   assign x = defs_pkg::
	// 4:   assign x = bus.
	// 5:   assign x = pkt.
	// 6:   sub_unit #(.
	// 7:   ) u_sub (.
	// 8:   );
	// 9: endmodule
	topSrc := "module top (bus_if.master bus, input logic clk);\n" +
		"  defs_pkg::pkt_t pkt;\n" +
		"  logic x;\n" +
		"  assign x = defs_pkg::\n" +
		"  assign x = bus.\n" +
		"  assign x = pkt.\n" +
		"  sub_unit #(.\n" +
		"  ) u_sub (.\n" +
		"  );\n" +
		"endmodule\n"

	var in, out bytes.Buffer
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathToURI(root),
		},
	})
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        topURI,
				LanguageID: "systemverilog",
				Version:    1,
				Text:       topSrc,
			},
		},
	})

	requests := []struct {
		id  int
		pos Position
	}{
		{2, Position{Line: 3, Character: 23}}, // defs_pkg::
		{3, Position{Line: 0, Character: 19}}, // bus_if. (modport completion)
		{4, Position{Line: 4, Character: 17}}, // bus. (interface members)
		{5, Position{Line: 5, Character: 17}}, // pkt. (struct fields)
		{6, Position{Line: 6, Character: 14}}, // sub_unit #(. (param completion)
		{7, Position{Line: 7, Character: 12}}, // ) u_sub (. (port completion)
	}
	for _, r := range requests {
		writeFramedJSON(t, &in, map[string]any{
			"jsonrpc": "2.0",
			"id":      r.id,
			"method":  "textDocument/completion",
			"params": CompletionParams{
				TextDocument: TextDocumentIdentifier{URI: topURI},
				Position:     r.pos,
			},
		})
	}

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() error: %v", err)
	}

	outReader := bufio.NewReader(&out)
	respInit := readFramedResponse(t, outReader)
	rawInit, _ := json.Marshal(respInit.Result)
	var initRes InitializeResult
	if err := json.Unmarshal(rawInit, &initRes); err != nil {
		t.Fatalf("unmarshaling InitializeResult: %v", err)
	}
	if initRes.Capabilities.CompletionProvider == nil || len(initRes.Capabilities.CompletionProvider.TriggerCharacters) < 2 {
		t.Fatalf("expected triggerCharacters ['.', ':'], got %+v", initRes.Capabilities.CompletionProvider)
	}

	_, _ = readFramedNotification(t, outReader) // publishDiagnostics

	readLabels := func(wantID string) map[string]bool {
		resp := readFramedResponse(t, outReader)
		if string(resp.ID) != wantID {
			t.Fatalf("got response ID=%s, want %s", string(resp.ID), wantID)
		}
		raw, _ := json.Marshal(resp.Result)
		var items []CompletionItem
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatalf("unmarshaling completions (id=%s): %v", wantID, err)
		}
		m := make(map[string]bool, len(items))
		for _, it := range items {
			m[it.Label] = true
		}
		return m
	}

	// id=2: defs_pkg:: -> DATA_W, pkt_t (no keywords like "module")
	lbl2 := readLabels("2")
	if !lbl2["DATA_W"] || !lbl2["pkt_t"] || lbl2["module"] {
		t.Errorf("defs_pkg:: completions = %v, want DATA_W and pkt_t only", lbl2)
	}

	// id=3: bus_if. -> master, slave
	lbl3 := readLabels("3")
	if !lbl3["master"] || !lbl3["slave"] || lbl3["module"] {
		t.Errorf("bus_if. completions = %v, want master and slave", lbl3)
	}

	// id=4: bus. -> valid, ready, clk
	lbl4 := readLabels("4")
	if !lbl4["valid"] || !lbl4["ready"] || !lbl4["clk"] || lbl4["module"] {
		t.Errorf("bus. completions = %v, want interface members valid/ready/clk", lbl4)
	}

	// id=5: pkt. -> addr, valid
	lbl5 := readLabels("5")
	if !lbl5["addr"] || !lbl5["valid"] || lbl5["module"] {
		t.Errorf("pkt. completions = %v, want struct fields addr and valid", lbl5)
	}

	// id=6: sub_unit #(. -> WIDTH
	lbl6 := readLabels("6")
	if !lbl6["WIDTH"] || lbl6["a"] || lbl6["module"] {
		t.Errorf("sub_unit #(. completions = %v, want WIDTH", lbl6)
	}

	// id=7: u_sub (. -> a, y
	lbl7 := readLabels("7")
	if !lbl7["a"] || !lbl7["y"] || lbl7["WIDTH"] || lbl7["module"] {
		t.Errorf("u_sub (. completions = %v, want ports a and y", lbl7)
	}
}
