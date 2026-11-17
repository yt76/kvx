package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kvx/internal/ast"
)

func TestParseValidVerilog(t *testing.T) {
	src := `
module top #(
    parameter WIDTH = 8
) (
    input wire clk,
    input wire rst_n,
    input wire [WIDTH-1:0] din,
    output reg [WIDTH-1:0] dout
);
    localparam ZERO = 8'h00;
    wire [WIDTH-1:0] next_val;

    assign next_val = din + 1'b1;

    sub_alu #(
        .W(WIDTH)
    ) u_alu (
        .a(din),
        .y(next_val)
    );

    always @(posedge clk or negedge rst_n) begin : seq_proc
        if (!rst_n) begin
            dout <= ZERO;
        end else begin
            case (din)
                8'h00: dout <= ZERO;
                default: dout <= next_val;
            endcase
        end
    end
endmodule
`

	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}

	mod := file.Modules[0]
	if mod.Name != "top" {
		t.Errorf("module name = %q, want %q", mod.Name, "top")
	}
	if len(mod.Params) != 2 { // WIDTH and ZERO
		t.Errorf("got %d params, want 2", len(mod.Params))
	}
	if len(mod.Ports) != 4 {
		t.Fatalf("got %d ports, want 4", len(mod.Ports))
	}
	if mod.Ports[2].Width == nil || mod.Ports[2].Width.Raw != "[WIDTH-1:0]" {
		t.Errorf("din port width = %+v, want [WIDTH-1:0]", mod.Ports[2].Width)
	}

	// Verify items inside module body
	var foundNet, foundAssign, foundInst, foundAlways bool
	for _, item := range mod.Items {
		switch v := item.(type) {
		case *ast.NetDecl:
			if v.Name == "next_val" && v.Kind == "wire" {
				foundNet = true
			}
		case *ast.AssignStmt:
			foundAssign = true
		case *ast.ModuleInst:
			if v.ModuleName == "sub_alu" && v.InstanceName == "u_alu" {
				foundInst = true
			}
		case *ast.ProceduralBlock:
			if v.Kind == "always" && v.Label == "seq_proc" {
				foundAlways = true
			}
		}
	}

	if !foundNet || !foundAssign || !foundInst || !foundAlways {
		t.Errorf("missing expected AST items: net=%v assign=%v inst=%v always=%v",
			foundNet, foundAssign, foundInst, foundAlways)
	}
}

func TestParseNonANSIModule(t *testing.T) {
	src := `
module adder(a, b, sum);
    input [3:0] a, b;
    output [3:0] sum;
    assign sum = a + b;
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}
	mod := file.Modules[0]
	if len(mod.Ports) != 3 {
		t.Errorf("got %d ports, want 3 (a, b, sum)", len(mod.Ports))
	}
}

func TestParseErrorRecovery(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		wantErrSub  string
		wantModules int
	}{
		{
			name: "missing semicolon after wire declaration recovers for next item",
			src: `
module m(input clk);
    wire a
    reg b;
endmodule
`,
			wantErrSub:  "expected ';'",
			wantModules: 1,
		},
		{
			name: "missing endmodule still returns partial module AST",
			src: `
module incomplete(input a, output b);
    assign b = a;
`,
			wantErrSub:  "expected 'endmodule'",
			wantModules: 1,
		},
		{
			name: "missing end in begin block",
			src: `
module m(input clk, output reg q);
    always @(posedge clk) begin
        q <= 1'b1;
endmodule
`,
			wantErrSub:  "expected 'end'",
			wantModules: 1,
		},
		{
			name: "missing rhs in assign statement",
			src: `
module m(output y);
    assign y = ;
endmodule
`,
			wantErrSub:  "expected expression after '='",
			wantModules: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse(tc.src)
			if len(errs) == 0 {
				t.Fatalf("expected parse error containing %q, got none", tc.wantErrSub)
			}
			matched := false
			for _, e := range errs {
				if strings.Contains(e.Message, tc.wantErrSub) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("errors %v did not contain %q", errs, tc.wantErrSub)
			}
			if len(file.Modules) != tc.wantModules {
				t.Errorf("got %d modules, want %d", len(file.Modules), tc.wantModules)
			}
		})
	}
}

func TestParseIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	defsHeader := filepath.Join(tmpDir, "defs.vh")
	defsContent := "`define DATA_W 16\n" +
		"`define WORD_RANGE [15:0]\n" +
		"localparam GLOBAL_MASK = 16'hFFFF;\n"
	if err := os.WriteFile(defsHeader, []byte(defsContent), 0644); err != nil {
		t.Fatal(err)
	}

	bodyHeader := filepath.Join(tmpDir, "body_signals.vh")
	bodyContent := "wire [15:0] masked_in;\n" +
		"assign masked_in = din & GLOBAL_MASK;\n"
	if err := os.WriteFile(bodyHeader, []byte(bodyContent), 0644); err != nil {
		t.Fatal(err)
	}

	topFile := filepath.Join(tmpDir, "top.v")
	topSrc := "`include \"defs.vh\"\n" +
		"module top(\n" +
		"  input wire [`DATA_W-1:0] din,\n" +
		"  output wire `WORD_RANGE dout\n" +
		");\n" +
		"  `include \"body_signals.vh\"\n" +
		"  assign dout = masked_in;\n" +
		"endmodule\n"

	file, errs := ParseWithOptions(topSrc, Options{FilePath: topFile})
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Includes) != 2 {
		t.Errorf("got %d includes, want 2", len(file.Includes))
	}
	if len(file.Macros) != 2 {
		t.Errorf("got %d macros, want 2", len(file.Macros))
	}
	if len(file.Params) != 1 || file.Params[0].Name != "GLOBAL_MASK" {
		t.Errorf("got top-level params %+v, want GLOBAL_MASK", file.Params)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}
	mod := file.Modules[0]
	if len(mod.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(mod.Ports))
	}
	if mod.Ports[1].Width == nil || mod.Ports[1].Width.Raw != "[15:0]" {
		t.Errorf("dout port width = %+v, want [15:0] expanded from `WORD_RANGE", mod.Ports[1].Width)
	}
}

func TestParseIncludeErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// Circular include: a.vh -> b.vh -> a.vh
	aPath := filepath.Join(tmpDir, "a.vh")
	bPath := filepath.Join(tmpDir, "b.vh")
	_ = os.WriteFile(aPath, []byte("`include \"b.vh\"\n"), 0644)
	_ = os.WriteFile(bPath, []byte("`include \"a.vh\"\n"), 0644)

	// Syntax error inside included file
	badHeader := filepath.Join(tmpDir, "bad.vh")
	_ = os.WriteFile(badHeader, []byte("wire broken_sig\n"), 0644)

	topPath := filepath.Join(tmpDir, "top.v")

	tests := []struct {
		name       string
		src        string
		wantErrSub string
		wantLine   int
	}{
		{
			name:       "missing include file reports error on include line",
			src:        "`include \"nonexistent.vh\"\nmodule m; endmodule\n",
			wantErrSub: `cannot open include file "nonexistent.vh"`,
			wantLine:   0,
		},
		{
			name:       "circular include reports cycle error",
			src:        "`include \"a.vh\"\nmodule m; endmodule\n",
			wantErrSub: "circular include detected",
			wantLine:   0,
		},
		{
			name:       "syntax error inside included file maps back to include directive line",
			src:        "module m;\n  `include \"bad.vh\"\nendmodule\n",
			wantErrSub: `in included file "bad.vh"`,
			wantLine:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := ParseWithOptions(tc.src, Options{FilePath: topPath})
			if len(errs) == 0 {
				t.Fatalf("expected error containing %q, got none", tc.wantErrSub)
			}
			if !strings.Contains(errs[0].Message, tc.wantErrSub) {
				t.Errorf("error message = %q, want substring %q", errs[0].Message, tc.wantErrSub)
			}
			if errs[0].Range.Start.Line != tc.wantLine {
				t.Errorf("error line = %d, want %d", errs[0].Range.Start.Line, tc.wantLine)
			}
		})
	}
}
