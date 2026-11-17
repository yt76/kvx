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
