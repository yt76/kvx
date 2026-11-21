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

func TestConditionalCompilation(t *testing.T) {
	tmpDir := t.TempDir()

	// Header with include guard `ifndef / `define / `endif
	guardedHeader := filepath.Join(tmpDir, "guarded.vh")
	guardedContent := "`ifndef GUARDED_VH\n" +
		"`define GUARDED_VH\n" +
		"localparam SHARED_P = 42;\n" +
		"`endif\n"
	if err := os.WriteFile(guardedHeader, []byte(guardedContent), 0644); err != nil {
		t.Fatal(err)
	}

	topPath := filepath.Join(tmpDir, "top.v")
	src := "`include \"guarded.vh\"\n" +
		"`include \"guarded.vh\"\n" +
		"`define MODE_B\n" +
		"`define TEMP_FLAG\n" +
		"`undef TEMP_FLAG\n" +
		"module top(\n" +
		"`ifdef MODE_A\n" +
		"  input wire [3:0] a,\n" +
		"  `include \"nonexistent_in_inactive_branch.vh\"\n" +
		"`elsif MODE_B\n" +
		"  `ifdef TEMP_FLAG\n" +
		"    input wire [1:0] a,\n" +
		"  `else\n" +
		"    input wire [7:0] a,\n" +
		"  `endif\n" +
		"`else\n" +
		"  input wire [15:0] a,\n" +
		"`endif\n" +
		"  output wire [7:0] y\n" +
		");\n" +
		"  assign y = a + SHARED_P;\n" +
		"endmodule\n"

	file, errs := ParseWithOptions(src, Options{FilePath: topPath})
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Params) != 1 || file.Params[0].Name != "SHARED_P" {
		t.Errorf("expected exactly 1 SHARED_P param (include guard prevented second copy), got %+v", file.Params)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}
	mod := file.Modules[0]
	if len(mod.Ports) != 2 {
		t.Fatalf("expected 2 ports (a and y), got %d: %+v", len(mod.Ports), mod.Ports)
	}
	if mod.Ports[0].Width == nil || mod.Ports[0].Width.Raw != "[7:0]" {
		t.Errorf("port a width = %+v, want [7:0] from active `elsif MODE_B -> `else branch", mod.Ports[0].Width)
	}
}

func TestConditionalCompilationErrors(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantErrSub string
	}{
		{
			name:       "unterminated ifdef reports error",
			src:        "`ifdef FOO\nmodule m; endmodule\n",
			wantErrSub: "unterminated `ifdef or `ifndef",
		},
		{
			name:       "stray endif without ifdef",
			src:        "module m; endmodule\n`endif\n",
			wantErrSub: "unexpected `endif without matching `ifdef or `ifndef",
		},
		{
			name:       "stray else without ifdef",
			src:        "`else\nmodule m; endmodule\n",
			wantErrSub: "unexpected `else without matching `ifdef or `ifndef",
		},
		{
			name:       "duplicate else in same conditional block",
			src:        "`ifdef FOO\n`else\n`else\n`endif\nmodule m; endmodule\n",
			wantErrSub: "duplicate `else in conditional block",
		},
		{
			name:       "elsif after else is rejected",
			src:        "`ifdef FOO\n`else\n`elsif BAR\n`endif\nmodule m; endmodule\n",
			wantErrSub: "`elsif after `else is not allowed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse(tc.src)
			if len(errs) == 0 {
				t.Fatalf("expected error containing %q, got none", tc.wantErrSub)
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
		})
	}
}

func TestParseFunctions(t *testing.T) {
	src := `
module math_unit(
    input wire [7:0] x,
    input wire [7:0] y,
    output wire [7:0] z
);
    // ANSI-style automatic function with bit-width return type
    function automatic signed [7:0] add_sat(
        input [7:0] a,
        input [7:0] b
    );
        reg [8:0] sum;
        begin
            sum = a + b;
            if (sum > 9'd255)
                add_sat = 8'hFF;
            else
                add_sat = sum[7:0];
        end
    endfunction

    // Non-ANSI style integer function
    function integer clog2;
        input [31:0] value;
        integer i;
        begin
            value = value - 1;
            for (i = 0; value > 0; i = i + 1)
                value = value >> 1;
            clog2 = i;
        end
    endfunction

    assign z = add_sat(x, y);
endmodule
`

	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}

	var funcs []*ast.FunctionDecl
	for _, item := range file.Modules[0].Items {
		if fn, ok := item.(*ast.FunctionDecl); ok {
			funcs = append(funcs, fn)
		}
	}
	if len(funcs) != 2 {
		t.Fatalf("got %d functions, want 2", len(funcs))
	}

	f0 := funcs[0]
	if f0.Name != "add_sat" || !f0.Automatic || !f0.Signed || f0.Width == nil || f0.Width.Raw != "[7:0]" {
		t.Errorf("unexpected f0 header: %+v", f0)
	}
	if len(f0.Params) != 2 || len(f0.Locals) != 1 {
		t.Errorf("f0 params=%d locals=%d, want 2 and 1", len(f0.Params), len(f0.Locals))
	}

	f1 := funcs[1]
	if f1.Name != "clog2" || f1.ReturnType != "integer" {
		t.Errorf("unexpected f1 header: %+v", f1)
	}
	if len(f1.Params) != 1 || len(f1.Locals) != 1 {
		t.Errorf("f1 params=%d locals=%d, want 1 and 1", len(f1.Params), len(f1.Locals))
	}

	// Verify error recovery when endfunction is missing
	badSrc := `
module m;
    function [3:0] f(input [3:0] a);
        f = a;
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected 'endfunction'") {
		t.Errorf("expected missing endfunction error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 partial module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseGenerateConstructs(t *testing.T) {
	src := `
module gen_demo #(
    parameter WIDTH = 4,
    parameter MODE  = 1
) (
    input  wire [WIDTH-1:0] din,
    output wire [WIDTH-1:0] dout
);
    genvar i;
    generate
        for (i = 0; i < WIDTH; i = i + 1) begin : g_bit
            wire inv_b;
            assign inv_b = ~din[i];
            if (MODE == 1) begin : g_inv
                assign dout[i] = inv_b;
            end else begin : g_pass
                assign dout[i] = din[i];
            end
        end

        case (MODE)
            0: begin : g_m0
                wire m0_flag = 1'b0;
            end
            default: begin : g_mdef
                wire m0_flag = 1'b1;
            end
        endcase
    endgenerate

    // Verilog-2005 generate-for without explicit generate/endgenerate and with inline genvar
    for (genvar k = 0; k < WIDTH; k = k + 1) begin : g_extra
        sub_buf u_buf (.a(din[k]), .y());
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

	var genBlocks []*ast.GenerateBlock
	for _, item := range file.Modules[0].Items {
		if g, ok := item.(*ast.GenerateBlock); ok {
			genBlocks = append(genBlocks, g)
		}
	}
	if len(genBlocks) != 2 { // 1 `generate...endgenerate` block + 1 standalone `for` generate block
		t.Fatalf("got %d top-level GenerateBlocks, want 2", len(genBlocks))
	}

	// Verify missing endgenerate reports syntax error and recovers
	badSrc := `
module m(input a, output b);
    generate
        assign b = a;
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected 'endgenerate'") {
		t.Errorf("expected missing endgenerate error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 partial module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseFunctionLikeMacros(t *testing.T) {
	src := "`define WIDTH 8\n" +
		"`define BUS(w) [w-1:0]\n" +
		"`define ADD(a, b) ((a) + (b))\n" +
		"`define ADD3(a, b, c) `ADD(`ADD(a, b), c)\n" +
		"`define INC(x, step=1) ((x) + (step))\n" +
		"`define ZERO() 1'b0\n" +
		"`define MUX(sel, t, f) ((sel) ? (t) : (f))\n" +
		"`define FF_ASSIGN(clk, q, d) \\\n" +
		"  always @(posedge clk) begin \\\n" +
		"    q <= d; \\\n" +
		"  end\n" +
		"\n" +
		"module func_macro_demo (\n" +
		"  input  wire clk,\n" +
		"  input  wire sel,\n" +
		"  input  wire `BUS(`WIDTH) a,\n" +
		"  input  wire `BUS(`WIDTH) b,\n" +
		"  input  wire `BUS(`WIDTH) c,\n" +
		"  output wire `BUS(`WIDTH) sum,\n" +
		"  output wire [3:0]        nib,\n" +
		"  output reg  `BUS(`WIDTH) q,\n" +
		"  output wire              z\n" +
		");\n" +
		"  assign sum = `ADD3(a, b, `INC(c));\n" +
		"  assign nib = `MUX(sel, {a[1:0], b[1:0]}, c[3:0]);\n" +
		"  assign z   = `ZERO();\n" +
		"  `FF_ASSIGN(clk, q, `INC(sum, 2))\n" +
		"endmodule\n"

	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}
	mod := file.Modules[0]
	if mod.Name != "func_macro_demo" {
		t.Errorf("module name = %q, want %q", mod.Name, "func_macro_demo")
	}

	// Verify expanded identifiers (a, b, c, sel, clk, q, sum) appear in module Refs with real source positions.
	refNames := make(map[string]bool)
	for _, r := range mod.Refs {
		refNames[r.Name] = true
	}
	for _, want := range []string{"a", "b", "c", "sel", "clk", "q", "sum"} {
		if !refNames[want] {
			t.Errorf("expected expanded argument identifier %q in module Refs, got %+v", want, refNames)
		}
	}

	// Verify argument count mismatch errors and recovery
	badArgsSrc := "`define PAIR(x, y) ((x) + (y))\n" +
		"module m(input wire a, input wire b, input wire c, output wire y1, output wire y2);\n" +
		"  assign y1 = `PAIR(a);\n" +
		"  assign y2 = `PAIR(a, b, c);\n" +
		"endmodule\n"
	badFile, badErrs := Parse(badArgsSrc)
	if len(badErrs) < 2 {
		t.Fatalf("expected at least 2 errors for too few / too many args, got %d: %v", len(badErrs), badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}

	// Verify self-recursive function-like macro does not hang
	recSrc := "`define SELF(x) `SELF(x)\n" +
		"module m(input wire a, output wire y);\n" +
		"  assign y = `SELF(a);\n" +
		"endmodule\n"
	_, _ = Parse(recSrc)
}
