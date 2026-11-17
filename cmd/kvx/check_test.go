package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheck(t *testing.T) {
	tmpDir := t.TempDir()

	validFile := filepath.Join(tmpDir, "valid.v")
	validSrc := "module counter(input wire clk, output reg [3:0] q);\n" +
		"  always @(posedge clk) begin\n" +
		"    q <= q + 4'd1;\n" +
		"  end\n" +
		"endmodule\n"
	if err := os.WriteFile(validFile, []byte(validSrc), 0644); err != nil {
		t.Fatal(err)
	}

	brokenFile := filepath.Join(tmpDir, "broken.v")
	brokenSrc := "module broken(input wire a, output wire b);\n" +
		"  wire x\n" +
		"  assign b = a;\n" +
		"endmodule\n"
	if err := os.WriteFile(brokenFile, []byte(brokenSrc), 0644); err != nil {
		t.Fatal(err)
	}

	semanticWarnFile := filepath.Join(tmpDir, "warn.v")
	semanticWarnSrc := "module warn_mod(output wire y);\n" +
		"  assign y = undeclared_sig;\n" +
		"endmodule\n"
	if err := os.WriteFile(semanticWarnFile, []byte(semanticWarnSrc), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantSubstr []string
	}{
		{
			name:     "valid file passes with exit code 0",
			args:     []string{validFile},
			wantCode: 0,
			wantSubstr: []string{
				"OK (0 syntax errors, 1 module(s))",
			},
		},
		{
			name:     "valid file with -ast and -tokens flags",
			args:     []string{"-tokens", "-ast", validFile},
			wantCode: 0,
			wantSubstr: []string{
				"=== Tokens:",
				"=== AST:",
				"Module counter",
				"Port:  input wire clk",
				"OK (0 syntax errors, 1 module(s))",
			},
		},
		{
			name:     "file with syntax error returns exit code 1 and reports line/col",
			args:     []string{brokenFile},
			wantCode: 1,
			wantSubstr: []string{
				"broken.v:2:8: syntax error: expected ';'",
				"FAILED (1 syntax error(s))",
			},
		},
		{
			name:     "semantic flag reports undeclared identifier warning",
			args:     []string{"-semantic", semanticWarnFile},
			wantCode: 0,
			wantSubstr: []string{
				"OK (0 syntax errors, 1 module(s))",
				`warning: undeclared identifier "undeclared_sig"`,
			},
		},
		{
			name:     "missing file returns exit code 1",
			args:     []string{filepath.Join(tmpDir, "nonexistent.v")},
			wantCode: 1,
			wantSubstr: []string{
				"error reading file",
			},
		},
		{
			name:     "no arguments returns exit code 2",
			args:     []string{},
			wantCode: 2,
			wantSubstr: []string{
				"no Verilog files specified",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := runCheck(tc.args, &out)
			if code != tc.wantCode {
				t.Errorf("runCheck(%v) code = %d, want %d\noutput:\n%s", tc.args, code, tc.wantCode, out.String())
			}
			gotOut := out.String()
			for _, sub := range tc.wantSubstr {
				if !strings.Contains(gotOut, sub) {
					t.Errorf("output missing %q\ngot:\n%s", sub, gotOut)
				}
			}
		})
	}
}
