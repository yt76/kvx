package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

func TestComputeSignatureHelp(t *testing.T) {
	src := "package math_pkg;\n" + // 0
		"  function automatic int clamp(input int val, input int lo, input int hi);\n" + // 1
		"    return val;\n" + // 2
		"  endfunction\n" + // 3
		"endpackage\n" + // 4
		"module sub #(parameter int WIDTH = 8, parameter int DEPTH = 16) (\n" + // 5
		"  input logic clk,\n" + // 6
		"  input logic [WIDTH-1:0] din,\n" + // 7
		"  output logic [WIDTH-1:0] dout\n" + // 8
		");\n" + // 9
		"  assign dout = din;\n" + // 10
		"endmodule\n" + // 11
		"module top (input logic clk, input logic [7:0] a, output logic [7:0] y);\n" + // 12
		"  let max2(x, z) = (x > z) ? x : z;\n" + // 13
		"  int q[$];\n" + // 14
		"  int r;\n" + // 15
		"  sub #(.DEPTH(32), .WIDTH(8)) u_sub (.clk(clk), .dout(y), .din(a));\n" + // 16
		"  initial begin\n" + // 17
		"    r = math_pkg::clamp(1, 2, 3);\n" + // 18
		"    r = max2(10, 20);\n" + // 19
		"    q.insert(0, 42);\n" + // 20
		"    r = $clog2(16);\n" + // 21
		"  end\n" + // 22
		"endmodule\n" // 23

	store := NewDocumentStore()
	doc := store.Open(TextDocumentItem{
		URI:        "file:///sig.sv",
		LanguageID: "systemverilog",
		Version:    1,
		Text:       src,
	})

	cases := []struct {
		name            string
		pos             Position
		wantLabelSubstr string
		wantParamCount  int
		wantActiveParam int
	}{
		{
			name:            "package-scoped function 2nd arg",
			pos:             Position{Line: 18, Character: 28}, // `math_pkg::clamp(1, |2, 3)`
			wantLabelSubstr: "clamp",
			wantParamCount:  3,
			wantActiveParam: 1,
		},
		{
			name:            "let call 2nd arg",
			pos:             Position{Line: 19, Character: 18}, // `max2(10, |20)`
			wantLabelSubstr: "let max2(x, z)",
			wantParamCount:  2,
			wantActiveParam: 1,
		},
		{
			name:            "queue built-in method insert 2nd arg",
			pos:             Position{Line: 20, Character: 16}, // `q.insert(0, |42)`
			wantLabelSubstr: "insert",
			wantParamCount:  2,
			wantActiveParam: 1,
		},
		{
			name:            "system function $clog2",
			pos:             Position{Line: 21, Character: 16}, // `$clog2(|16)`
			wantLabelSubstr: "$clog2",
			wantParamCount:  1,
			wantActiveParam: 0,
		},
		{
			name:            "instantiation named port .dout(y)",
			pos:             Position{Line: 16, Character: 56}, // `.dout(|y)` -> index 2 (dout)
			wantLabelSubstr: "module sub",
			wantParamCount:  3,
			wantActiveParam: 2,
		},
		{
			name:            "instantiation named param .DEPTH(32)",
			pos:             Position{Line: 16, Character: 16}, // `.DEPTH(|32)` -> index 1 (DEPTH)
			wantLabelSubstr: "sub #(",
			wantParamCount:  2,
			wantActiveParam: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := computeSignatureHelp(doc, store, tc.pos)
			if sh == nil || len(sh.Signatures) == 0 {
				t.Fatalf("computeSignatureHelp(%+v) returned nil", tc.pos)
			}
			sig := sh.Signatures[0]
			if !strings.Contains(sig.Label, tc.wantLabelSubstr) {
				t.Errorf("Signature.Label = %q, want substring %q", sig.Label, tc.wantLabelSubstr)
			}
			if len(sig.Parameters) != tc.wantParamCount {
				t.Errorf("len(Parameters) = %d (%+v), want %d", len(sig.Parameters), sig.Parameters, tc.wantParamCount)
			}
			if sh.ActiveParameter != tc.wantActiveParam {
				t.Errorf("ActiveParameter = %d, want %d (label=%q)", sh.ActiveParameter, tc.wantActiveParam, sig.Label)
			}
		})
	}
}

func TestLSPSignatureHelpRequest(t *testing.T) {
	var in bytes.Buffer
	mustWrite := func(payload string) {
		if err := WriteMessage(&in, json.RawMessage(payload)); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}

	mustWrite(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	mustWrite(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	mustWrite(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///sh.sv","languageId":"systemverilog","version":1,"text":"module m;\n  function int add(input int a, input int b); return a + b; endfunction\n  int r;\n  initial r = add(1, 2);\nendmodule\n"}}}`)
	mustWrite(`{"jsonrpc":"2.0","id":2,"method":"textDocument/signatureHelp","params":{"textDocument":{"uri":"file:///sh.sv"},"position":{"line":3,"character":21}}}`)
	mustWrite(`{"jsonrpc":"2.0","id":3,"method":"shutdown","params":null}`)
	mustWrite(`{"jsonrpc":"2.0","method":"exit","params":null}`)

	var out bytes.Buffer
	srv := NewServer(&in, &out, log.New(io.Discard, "", 0))
	if err := srv.Run(); err != nil {
		t.Fatalf("srv.Run: %v", err)
	}

	var initResp InitializeResult
	var sigHelp SignatureHelp
	foundSH := false

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
			t.Fatalf("Unmarshal envelope: %v", err)
		}
		switch string(env.ID) {
		case "1":
			if err := json.Unmarshal(env.Result, &initResp); err != nil {
				t.Fatalf("Unmarshal initialize: %v", err)
			}
		case "2":
			foundSH = true
			if err := json.Unmarshal(env.Result, &sigHelp); err != nil {
				t.Fatalf("Unmarshal signatureHelp: %v", err)
			}
		}
	}

	if initResp.Capabilities.SignatureHelpProvider == nil {
		t.Fatalf("expected SignatureHelpProvider to be advertised")
	}
	if !foundSH || len(sigHelp.Signatures) != 1 || sigHelp.ActiveParameter != 1 {
		t.Fatalf("unexpected signatureHelp = %+v", sigHelp)
	}
}
