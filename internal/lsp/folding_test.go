package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"testing"
)

func TestComputeFoldingRanges(t *testing.T) {
	src := "// Header line 1\n" + // 0
		"// Header line 2\n" + // 1
		"/* Block\n" + // 2
		"   comment */\n" + // 3
		"`ifdef SIM\n" + // 4
		"import \"DPI-C\" function int c_foo(int x);\n" + // 5
		"`else\n" + // 6
		"typedef class fwd_c;\n" + // 7
		"`endif\n" + // 8
		"module top (\n" + // 9
		"  input logic clk,\n" + // 10
		"  output logic y\n" + // 11
		");\n" + // 12
		"  typedef struct packed {\n" + // 13
		"    logic a;\n" + // 14
		"    logic b;\n" + // 15
		"  } s_t;\n" + // 16
		"  always_ff @(posedge clk) begin\n" + // 17
		"    case (y)\n" + // 18
		"      1'b0: y <= 1'b1;\n" + // 19
		"      default: y <= 1'b0;\n" + // 20
		"    endcase\n" + // 21
		"  end\n" + // 22
		"  assert property (@(posedge clk) y);\n" + // 23
		"endmodule\n" // 24

	got := computeFoldingRanges(src)
	has := func(start, end int, kind string) bool {
		for _, r := range got {
			if r.StartLine == start && r.EndLine == end && r.Kind == kind {
				return true
			}
		}
		return false
	}

	checks := []struct {
		name  string
		start int
		end   int
		kind  string
	}{
		{"line comment block", 0, 1, FoldingKindComment},
		{"block comment", 2, 3, FoldingKindComment},
		{"ifdef branch", 4, 5, FoldingKindRegion},
		{"else branch", 6, 8, FoldingKindRegion},
		{"port list parens", 9, 12, ""},
		{"module block", 9, 24, ""},
		{"struct braces", 13, 16, ""},
		{"begin..end", 17, 22, ""},
		{"case..endcase", 18, 21, ""},
	}
	for _, c := range checks {
		if !has(c.start, c.end, c.kind) {
			t.Errorf("missing folding range %s (%d..%d, kind=%q); got %+v", c.name, c.start, c.end, c.kind, got)
		}
	}
}

func TestLSPFoldingRangeRequest(t *testing.T) {
	var in bytes.Buffer
	mustWrite := func(payload string) {
		if err := WriteMessage(&in, json.RawMessage(payload)); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}

	mustWrite(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	mustWrite(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	mustWrite(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///fold.sv","languageId":"systemverilog","version":1,"text":"module m;\n  initial begin\n    $display(\"hi\");\n  end\nendmodule\n"}}}`)
	mustWrite(`{"jsonrpc":"2.0","id":2,"method":"textDocument/foldingRange","params":{"textDocument":{"uri":"file:///fold.sv"}}}`)
	mustWrite(`{"jsonrpc":"2.0","id":3,"method":"shutdown","params":null}`)
	mustWrite(`{"jsonrpc":"2.0","method":"exit","params":null}`)

	var out bytes.Buffer
	srv := NewServer(&in, &out, log.New(io.Discard, "", 0))
	if err := srv.Run(); err != nil {
		t.Fatalf("srv.Run: %v", err)
	}

	var initResp InitializeResult
	var foldRanges []FoldingRange
	foundFold := false

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
			foundFold = true
			if err := json.Unmarshal(env.Result, &foldRanges); err != nil {
				t.Fatalf("Unmarshal foldingRange: %v", err)
			}
		}
	}

	if !initResp.Capabilities.FoldingRangeProvider {
		t.Errorf("expected FoldingRangeProvider=true in ServerCapabilities")
	}
	if !foundFold {
		t.Fatalf("did not receive response for foldingRange request")
	}
	if len(foldRanges) != 2 || foldRanges[0].StartLine != 0 || foldRanges[0].EndLine != 4 ||
		foldRanges[1].StartLine != 1 || foldRanges[1].EndLine != 3 {
		t.Errorf("unexpected foldingRanges = %+v, want [0..4, 1..3]", foldRanges)
	}
}
