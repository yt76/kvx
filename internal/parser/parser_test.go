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

	// Verify expanded identifiers (a, b, c, sel, clk, q, sum) appear in module or block Refs with real source positions.
	refNames := make(map[string]bool)
	for _, r := range mod.Refs {
		refNames[r.Name] = true
	}
	for _, item := range mod.Items {
		if blk, ok := item.(*ast.ProceduralBlock); ok {
			for _, r := range blk.Refs {
				refNames[r.Name] = true
			}
		}
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

func TestParseAttributes(t *testing.T) {
	src := `
(* keep_hierarchy = "yes" *)
module attr_demo #(
    (* fsm_encoding = "one_hot" *) parameter WIDTH = 8
) (
    (* mark_debug = "true" *) input wire clk,
    (* mark_debug = "true", keep = "true" *) input wire [WIDTH-1:0] din,
    (* dont_touch = "yes" *) output reg [WIDTH-1:0] dout
);
    (* ram_style = "block" *) reg [WIDTH-1:0] mem [0:15];
    (* keep = "true" *) (* ASYNC_REG = "TRUE" *) wire [WIDTH-1:0] stage;

    (* keep = "true" *) assign stage = (* no_opt *) din + 1'b1;

    (* dont_touch = "true" *)
    sub_mod u_sub (
        (* keep *) .a(stage),
        .y()
    );

    // Ensure always @(*) is still parsed as wildcard sensitivity list, not an attribute!
    reg [WIDTH-1:0] comb;
    always @(*) begin
        (* full_case, parallel_case *)
        case (din[1:0])
            2'b00: comb = stage;
            default: comb = din;
        endcase
    end

    always @(posedge clk) begin
        mem[0] <= comb;
        dout <= mem[0];
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
	if mod.Name != "attr_demo" {
		t.Errorf("module name = %q, want %q", mod.Name, "attr_demo")
	}
	// Ensure attribute names like mark_debug, full_case, no_opt were NOT recorded as signal references.
	for _, ref := range mod.Refs {
		switch ref.Name {
		case "keep_hierarchy", "fsm_encoding", "mark_debug", "keep", "dont_touch",
			"ram_style", "ASYNC_REG", "no_opt", "full_case", "parallel_case":
			t.Errorf("attribute name %q should not be recorded as a signal reference", ref.Name)
		}
	}

	// Verify unclosed attribute reports an error and recovers.
	badSrc := `
module m(input wire a, output wire y);
    (* mark_debug = "true" wire w;
    assign y = a;
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "unclosed attribute") {
		t.Errorf("expected unclosed attribute error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseTaskDecl(t *testing.T) {
	src := `
module task_demo (
    input  wire       clk,
    input  wire [7:0] din,
    output reg  [7:0] dout
);
    // ANSI style automatic task
    task automatic drive_out (
        input  [7:0] val,
        output [7:0] res
    );
        reg [7:0] tmp;
        begin
            tmp = val + 8'd1;
            res = tmp;
        end
    endtask

    // Non-ANSI style task with no arguments
    task clear_out;
        begin
            dout <= 8'h00;
        end
    endtask

    // Non-ANSI style task with body port declarations
    task add_pair;
        input  [7:0] a;
        input  [7:0] b;
        output [7:0] y;
        begin
            y = a + b;
        end
    endtask

    reg [7:0] next_val;
    always @(posedge clk) begin
        if (din == 8'h00) begin
            clear_out;
        end else begin
            drive_out(din, next_val);
            add_pair(next_val, din, dout);
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

	var tasks []*ast.TaskDecl
	for _, item := range file.Modules[0].Items {
		if tk, ok := item.(*ast.TaskDecl); ok {
			tasks = append(tasks, tk)
		}
	}
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(tasks))
	}
	if tasks[0].Name != "drive_out" || !tasks[0].Automatic || len(tasks[0].Params) != 2 || len(tasks[0].Locals) != 1 {
		t.Errorf("unexpected tasks[0]: %+v", tasks[0])
	}
	if tasks[1].Name != "clear_out" || tasks[1].Automatic || len(tasks[1].Params) != 0 {
		t.Errorf("unexpected tasks[1]: %+v", tasks[1])
	}
	if tasks[2].Name != "add_pair" || len(tasks[2].Params) != 3 {
		t.Errorf("unexpected tasks[2]: %+v", tasks[2])
	}

	// Verify error recovery when endtask is missing
	badSrc := `
module m;
    task t(input a);
        $display("%b", a);
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected 'endtask'") {
		t.Errorf("expected missing endtask error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 partial module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseProceduralBlockLocals(t *testing.T) {
	src := `
module blk_local_demo (
    input  wire       clk,
    input  wire [7:0] din,
    output reg  [7:0] dout,
    output reg  [7:0] comb
);
    always @(*) begin : blk_comb
        integer i;
        reg [7:0] tmp;
        localparam MASK = 8'hFF;
        tmp = 8'h00;
        for (i = 0; i < 8; i = i + 1) begin
            tmp[i] = din[i] & MASK[i];
        end
        comb = tmp;
    end

    always @(posedge clk) begin : blk_seq
        integer i;
        reg [7:0] tmp;
        tmp = comb;
        for (i = 0; i < 8; i = i + 1) begin
            dout[i] <= tmp[i];
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
	var blocks []*ast.ProceduralBlock
	for _, item := range file.Modules[0].Items {
		if blk, ok := item.(*ast.ProceduralBlock); ok {
			blocks = append(blocks, blk)
		}
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d procedural blocks, want 2", len(blocks))
	}
	if blocks[0].Label != "blk_comb" || len(blocks[0].Locals) != 3 {
		t.Errorf("blocks[0] label=%q locals=%d, want blk_comb and 3", blocks[0].Label, len(blocks[0].Locals))
	}
	if blocks[1].Label != "blk_seq" || len(blocks[1].Locals) != 2 {
		t.Errorf("blocks[1] label=%q locals=%d, want blk_seq and 2", blocks[1].Label, len(blocks[1].Locals))
	}
}

func TestParseDefparam(t *testing.T) {
	src := `
module sub #(parameter WIDTH = 8, parameter DEPTH = 16) (
    input  wire [WIDTH-1:0] a,
    output wire [WIDTH-1:0] y
);
    assign y = a;
endmodule

module top #(parameter TOP_W = 16) (
    input  wire [TOP_W-1:0] din,
    output wire [TOP_W-1:0] dout
);
    sub u_sub (.a(din), .y(dout));
    defparam u_sub.WIDTH = TOP_W;
    defparam u_sub.DEPTH = 32, u_sub.WIDTH = TOP_W;
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(file.Modules))
	}
	var defparams []*ast.DefparamStmt
	for _, item := range file.Modules[1].Items {
		if dp, ok := item.(*ast.DefparamStmt); ok {
			defparams = append(defparams, dp)
		}
	}
	if len(defparams) != 2 {
		t.Fatalf("got %d defparam statements, want 2", len(defparams))
	}

	// Missing '=' error and recovery
	badSrc := `
module m;
    defparam u_sub.WIDTH;
    wire x;
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected '=' in defparam statement") {
		t.Errorf("expected missing '=' error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParsePortInitAndAssignVariants(t *testing.T) {
	src := `
module ansi_init #(
    parameter INIT_VAL = 8'h00
) (
    input  wire       clk,
    input  wire [7:0] a,
    input  wire [7:0] b,
    output reg  [7:0] q = INIT_VAL,
    output wire [7:0] y1,
    output wire [7:0] y2,
    output wire [7:0] y3
);
    wire #1 w_dly = a;
    assign y1 = a, y2 = b;
    assign #2 {y3} = {w_dly};
    always @(posedge clk) begin
        q <= y1 + y2;
    end
endmodule

module non_ansi_init (clk, a, b, q, y1, y2);
    input  wire       clk;
    input  wire [7:0] a, b;
    output reg  [7:0] q = 8'hFF;
    output wire [3:0] y1;
    output wire [3:0] y2;

    assign #(1, 2) {y1, y2} = {a[3:0], b[3:0]};
    always @(posedge clk) begin
        q <= a;
    end
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(file.Modules))
	}
	if len(file.Modules[0].Ports) != 7 {
		t.Errorf("ansi_init ports = %d, want 7", len(file.Modules[0].Ports))
	}
	if len(file.Modules[1].Ports) != 6 {
		t.Errorf("non_ansi_init ports = %d, want 6", len(file.Modules[1].Ports))
	}
}

func TestParseNetVarTypesAndEvent(t *testing.T) {
	src := `
module net_var_demo (
    input  wire       clk,
    inout  tri  [7:0] io_bus,
    output wor        irq_n
);
    supply0 gnd;
    supply1 vdd;
    tri     tri_sig;
    wand    and_bus;
    wor     or_bus;

    real     gain;
    time     ts;
    realtime rts;
    event    ev_done;

    assign and_bus = vdd;
    assign and_bus = ~gnd;
    assign or_bus  = gnd;
    assign irq_n   = or_bus;

    function real scale_val(input real x);
        begin
            scale_val = x;
        end
    endfunction

    always @(posedge clk) begin : blk
        real local_r;
        time local_t;
        event local_ev;
        local_r = scale_val(gain);
        local_t = $time;
        ts <= local_t;
        rts <= local_r;
        -> ev_done;
        -> local_ev;
    end

    always @(ev_done) begin
        tri_sig <= io_bus[0];
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

	// Missing identifier after '->' reports an error and recovers.
	badSrc := `
module m;
    initial begin
        -> ;
    end
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected event identifier after '->'") {
		t.Errorf("expected missing event identifier error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseProceduralLoopsAndWait(t *testing.T) {
	src := `
module tb_ctrl;
    reg clk;
    reg ready;
    reg done;

    initial begin
        clk = 1'b0;
        forever #5 clk = ~clk;
    end

    initial begin : main_seq
        ready = 1'b0;
        done  = 1'b0;
        repeat (10) @(posedge clk);
        ready = 1'b1;
        wait (ready);

        fork : par_blk
            begin : branch_a
                #20;
                disable branch_b;
            end
            begin : branch_b
                forever #10 done = ~done;
            end
        join
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

	// Unclosed fork reports missing join and recovers
	badSrc := `
module m;
    initial begin
        fork
            #10;
    end
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 || !strings.Contains(badErrs[0].Message, "expected 'join'") {
		t.Errorf("expected missing join error, got %v", badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseGatePrimitives(t *testing.T) {
	src := `
module gate_demo (
    input  wire a,
    input  wire b,
    input  wire oe,
    output wire y_and,
    output wire y_or,
    output wire y_not,
    output wire y_nand,
    output wire y_xor,
    inout  wire io,
    output wire p_up
);
    and (y_and, a, b);
    or (y_or, a, b);
    not (y_not, a);
    bufif1 (io, a, oe);
    nand #1 (y_nand, a, b);
    xor u_xor (y_xor, a, b);
    pullup (p_up);
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}

	// Non-gate identifier followed by '(' in module body must still report an error.
	badSrc := `
module m(input wire a, output wire y);
    unknown_gate (y, a);
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) == 0 {
		t.Errorf("expected error for non-gate identifier without instance name")
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseSpecifyAndPrimitiveUDP(t *testing.T) {
	src := `
primitive udp_dff (q, clk, d);
    output q;
    input clk, d;
    reg q;
    initial q = 1'b0;
    table
        // clk  d  : q : q+
           (01) 0  : ? : 0;
           (01) 1  : ? : 1;
           (0?) ?  : ? : -;
           ?   (??): ? : -;
    endtable
endprimitive

module dff_cell (
    input  wire clk,
    input  wire d,
    output wire q
);
    reg notifier;
    udp_dff u_dff (q, clk, d);

    specify
        specparam tSU = 0.5, tHD = 0.2;
        (posedge clk => (q +: d)) = (1.0, 1.2);
        $setuphold(posedge clk, posedge d, tSU, tHD, notifier);
    endspecify
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(file.Modules))
	}
	if file.Modules[0].Name != "dff_cell" {
		t.Errorf("module name = %q, want %q", file.Modules[0].Name, "dff_cell")
	}

	// Missing endspecify and endprimitive report errors and recover
	badSrc := `
primitive udp_bad (y, a);
    output y;
    input a;
    table
        0 : 1;
    endtable
module m(input wire a, output wire y);
    specify
        (a => y) = 1.0;
endmodule
`
	badFile, badErrs := Parse(badSrc)
	if len(badErrs) < 2 {
		t.Fatalf("expected at least 2 errors (missing endprimitive and endspecify), got %d: %v", len(badErrs), badErrs)
	}
	if len(badFile.Modules) != 1 {
		t.Errorf("expected 1 module on recovery, got %d", len(badFile.Modules))
	}
}

func TestParseVerilogExtras(t *testing.T) {
	src := "`begin_keywords \"1364-2005\"\n" +
		"`unconnected_drive pull0\n" +
		"`define MAKE_NET(prefix, idx) prefix``_net_``idx\n" +
		"\n" +
		"primitive udp_buf (y, a);\n" +
		"    output y;\n" +
		"    input a;\n" +
		"    table\n" +
		"        0 : 0;\n" +
		"        1 : 1;\n" +
		"    endtable\n" +
		"endprimitive\n" +
		"\n" +
		"macromodule cell_inv (y, a, );\n" +
		"    input a;\n" +
		"    reg y;\n" +
		"    output y;\n" +
		"    always @(*) y = ~a;\n" +
		"endmodule\n" +
		"\n" +
		"module extras_demo #(parameter W = 4) (\n" +
		"    input  wire            clk,\n" +
		"    input  wire            rst_n,\n" +
		"    input  wire            oe,\n" +
		"    input  uwire [W-1:0]   a,\n" +
		"    input  uwire [W-1:0]   b,\n" +
		"    inout  tri   [W-1:0]   io_pin,\n" +
		"    output reg   [W-1:0]   q,\n" +
		"    output wire  [W-1:0]   y_xnor,\n" +
		"    output wire            y_udp\n" +
		");\n" +
		"    localparam DEPTH = 2 ** W;\n" +
		"    tri0   t0_sig;\n" +
		"    tri1   t1_sig;\n" +
		"    triand tand_sig;\n" +
		"    trior  tor_sig;\n" +
		"    trireg (large) cap_node;\n" +
		"    wire vectored [W-1:0] `MAKE_NET(my, 0);\n" +
		"    wire (weak1, weak0) [W-1:0] pull_bus = a;\n" +
		"\n" +
		"    assign (strong1, pull0) #1 `MAKE_NET(my, 0) = (a ~^ b) & (a ^~ b);\n" +
		"    assign y_xnor = `MAKE_NET(my, 0) ^ pull_bus;\n" +
		"\n" +
		"    bufif1 (strong1, strong0) #1 (io_pin[0], a[0], oe);\n" +
		"    udp_buf (y_udp, a[0]);\n" +
		"\n" +
		"    always @(rst_n) begin\n" +
		"        if (!rst_n)\n" +
		"            assign q = {W{1'b0}};\n" +
		"        else\n" +
		"            deassign q;\n" +
		"    end\n" +
		"\n" +
		"    initial begin\n" +
		"        force y_xnor = {W{1'b0}};\n" +
		"        #10 release y_xnor;\n" +
		"    end\n" +
		"endmodule\n" +
		"`nounconnected_drive\n" +
		"`end_keywords\n"

	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 2 {
		t.Fatalf("expected 2 modules (macromodule + module), got %d", len(file.Modules))
	}
}

func TestParseCornerCases(t *testing.T) {
	src := "`define EMPTY_MARKER\n" +
		"`define MULTI_COMMENT_ADD(a, b) /* line1\n" +
		"   line2 */ ((a) + \\\n" +
		"   (b))\n" +
		"`define LOG_VAL(x) $display(`\"value (`\\`\"x`\\`\") = %h`\", x)\n" +
		"\n" +
		"config demo_cfg;\n" +
		"    design rtlLib.corner_demo;\n" +
		"    default liblist rtlLib;\n" +
		"endconfig : demo_cfg\n" +
		"\n" +
		"module corner_demo (\n" +
		"    input  wire [3:0] a,\n" +
		"    input  wire [3:0] b,\n" +
		"    output wire [3:0] y_gate,\n" +
		"    output reg  [3:0] y_comb\n" +
		");\n" +
		"    `EMPTY_MARKER\n" +
		"\n" +
		"    // Unnamed gate primitive array\n" +
		"    and [3:0] (y_gate, a, b);\n" +
		"\n" +
		"    function [3:0] fn_id(input [3:0] v);\n" +
		"        begin : fn_blk\n" +
		"            fn_id = v;\n" +
		"        end : fn_blk\n" +
		"    endfunction : fn_id\n" +
		"\n" +
		"    task tk_log(input [3:0] v);\n" +
		"        begin : tk_blk\n" +
		"            `LOG_VAL(v);\n" +
		"        end : tk_blk\n" +
		"    endtask : tk_log\n" +
		"\n" +
		"    // Wildcard sensitivity list with space before ')'\n" +
		"    always @(* ) begin : comb_blk\n" +
		"        `EMPTY_MARKER\n" +
		"        y_comb = `MULTI_COMMENT_ADD(fn_id(a), b);\n" +
		"    end : comb_blk\n" +
		"\n" +
		"    initial begin : init_blk\n" +
		"        fork : fk_blk\n" +
		"            tk_log(a);\n" +
		"        join : fk_blk\n" +
		"    end : init_blk\n" +
		"endmodule : corner_demo\n"

	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 1 || file.Modules[0].Name != "corner_demo" {
		t.Fatalf("expected 1 module 'corner_demo', got %+v", file.Modules)
	}
}

func TestParseSystemVerilogStep1(t *testing.T) {
	src := `
module sv_sub (
    input  logic       clk,
    input  logic [7:0] din,
    output logic [7:0] dout
);
    assign dout = din;
endmodule

module sv_top #(
    parameter int WIDTH = 8
) (
    input  logic              clk,
    input  logic              rst_n,
    input  logic [1:0]        sel,
    input  logic [WIDTH-1:0]  din,
    output logic [WIDTH-1:0]  dout,
    output logic [WIDTH-1:0]  comb_out,
    output logic [WIDTH-1:0]  latch_out
);
    localparam logic [WIDTH-1:0] ZERO = '0;
    logic [3:0][WIDTH-1:0] pipe;
    logic [WIDTH-1:0]      sub_out;
    bit                    flag;

    // Implicit named port (.clk) and wildcard port (.*)
    sv_sub u_sub0 (.clk, .din(din), .dout(sub_out));
    sv_sub u_sub1 (.*, .dout());

    function automatic logic [WIDTH-1:0] sat_add(input logic [WIDTH-1:0] a, input int unsigned b);
        logic [WIDTH-1:0] res;
        res = a;
        res += b;
        return res;
    endfunction

    function void noop_fn(input bit en);
        if (!en) return;
    endfunction

    always_comb begin
        comb_out = ZERO;
        unique case (sel)
            2'b00: comb_out = din;
            2'b01: comb_out = sub_out;
            default: comb_out = sat_add(din, 1);
        endcase
        priority if (flag) begin
            for (int i = 0; i < 4; i++) begin
                if (i == 2) continue;
                comb_out ^= pipe[i];
                if (i == 3) break;
            end
        end
    end

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) begin
            dout <= ZERO;
            flag <= 1'b0;
        end else begin
            int cnt;
            cnt = 0;
            do begin
                cnt++;
            end while (cnt < 2);
            pipe[0] <= comb_out;
            dout    <= pipe[0];
            flag    <= 1'b1;
        end
    end

    always_latch begin
        if (flag)
            latch_out = comb_out;
    end
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(file.Modules))
	}
	// Verify `.clk` implicit named port connection recorded a local ref to `clk` in sv_top.
	foundClkRef := false
	for _, r := range file.Modules[1].Refs {
		if r.Name == "clk" {
			foundClkRef = true
			break
		}
	}
	if !foundClkRef {
		t.Errorf("expected implicit named port `.clk` to record local ref to `clk`")
	}
}

func TestParseSystemVerilogUserDefinedTypes(t *testing.T) {
	src := `
typedef logic [7:0] byte_t;

typedef enum logic [1:0] {
    ST_IDLE = 2'd0,
    ST_RUN  = 2'd1,
    ST_DONE = 2'd2
} state_t;

typedef struct packed {
    byte_t addr;
    logic  valid;
} pkt_t;

typedef union packed {
    logic [7:0] raw;
    struct packed {
        logic [3:0] hi;
        logic [3:0] lo;
    } nib;
} word_u;

module sv_types_demo #(
    parameter type T = byte_t,
    parameter state_t INIT_ST = ST_IDLE
) (
    input  logic   clk,
    input  logic   rst_n,
    input  state_t state_in,
    input  pkt_t   pkt_in,
    input  T       data_in,
    output state_t state_out,
    output pkt_t   pkt_out,
    output T       data_out
);
    enum logic { MODE_A, MODE_B } mode_q;
    state_t cur_state, next_state;
    pkt_t   pkt_q;
    word_u  u_val;
    T       buf_val;

    function automatic state_t advance_state(input state_t s);
        if (s == ST_IDLE)
            return ST_RUN;
        return ST_DONE;
    endfunction

    always_comb begin
        next_state = advance_state(cur_state);
        pkt_out    = '{addr: data_in, valid: pkt_in.valid};
        u_val      = word_u'(data_in);
        data_out   = T'(u_val.raw);
        state_out  = state_t'(next_state);
    end

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) begin
            cur_state <= INIT_ST;
            pkt_q     <= '{default: '0};
            mode_q    <= MODE_A;
            buf_val   <= '0;
        end else begin
            cur_state <= state_in;
            pkt_q     <= pkt_in;
            mode_q    <= MODE_B;
            buf_val   <= data_in;
        end
    end
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Typedefs) != 4 {
		t.Fatalf("got %d top-level typedefs, want 4", len(file.Typedefs))
	}
	if file.Typedefs[1].Name != "state_t" || len(file.Typedefs[1].EnumMembers) != 3 {
		t.Errorf("unexpected enum typedef: %+v", file.Typedefs[1])
	}
	if len(file.Modules) != 1 || file.Modules[0].Name != "sv_types_demo" {
		t.Fatalf("expected 1 module 'sv_types_demo', got %+v", file.Modules)
	}
}

func TestParseSystemVerilogPackagesAndImports(t *testing.T) {
	src := `
package common_pkg;
    localparam int WIDTH = 16;
    typedef enum logic [1:0] {
        ST_IDLE = 2'd0,
        ST_BUSY = 2'd1
    } state_t;

    function automatic logic [WIDTH-1:0] inc_val(input logic [WIDTH-1:0] v);
        return v + 1;
    endfunction
endpackage : common_pkg

module pkg_user
    import common_pkg::WIDTH;
#(
    parameter int W = WIDTH
) (
    input  logic              clk,
    input  common_pkg::state_t st_in,
    input  logic [W-1:0]      din,
    output logic [W-1:0]      dout,
    output common_pkg::state_t st_out
);
    import common_pkg::*;
    state_t cur_st;

    always_ff @(posedge clk) begin
        if (st_in == common_pkg::ST_IDLE) begin
            cur_st <= ST_BUSY;
            dout   <= common_pkg::inc_val(din);
        end else begin
            cur_st <= st_in;
            dout   <= inc_val(din);
        end
    end
    assign st_out = cur_st;
endmodule
`
	file, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatalf("expected 0 parse errors, got %d: %v", len(errs), errs)
	}
	if len(file.Packages) != 1 || file.Packages[0].Name != "common_pkg" {
		t.Fatalf("expected 1 package 'common_pkg', got %+v", file.Packages)
	}
	if len(file.Modules) != 1 || len(file.Modules[0].Imports) != 2 {
		t.Fatalf("expected 1 module with 2 imports, got %+v", file.Modules)
	}
}
