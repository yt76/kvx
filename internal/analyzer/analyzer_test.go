package analyzer

import (
	"testing"

	"kvx/internal/ast"
	"kvx/internal/parser"
)

func TestAnalyzeAndSymbolResolution(t *testing.T) {
	src := "module sub_alu(input wire [7:0] a, output wire [7:0] y);\n" +
		"  assign y = a;\n" +
		"endmodule\n" +
		"module top #(parameter W = 8) (\n" +
		"  input wire clk,\n" +
		"  input wire [W-1:0] din,\n" +
		"  output reg [W-1:0] dout\n" +
		");\n" +
		"  wire [W-1:0] alu_out;\n" +
		"  sub_alu u_alu (.a(din), .y(alu_out));\n" +
		"  always @(posedge clk) begin\n" +
		"    dout <= alu_out;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Errorf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}
	if len(an.ModuleOrder) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(an.ModuleOrder))
	}

	tests := []struct {
		name     string
		pos      ast.Position
		wantName string
		wantKind SymbolKind
		wantLine int // declaration line
	}{
		{
			name:     "reference to din inside module instantiation (.a(din))",
			pos:      ast.Position{Line: 9, Column: 21},
			wantName: "din",
			wantKind: KindPort,
			wantLine: 5,
		},
		{
			name:     "reference to alu_out in procedural assignment (dout <= alu_out)",
			pos:      ast.Position{Line: 11, Column: 14},
			wantName: "alu_out",
			wantKind: KindNet,
			wantLine: 8,
		},
		{
			name:     "reference to sub_alu module type in instantiation",
			pos:      ast.Position{Line: 9, Column: 4},
			wantName: "sub_alu",
			wantKind: KindModule,
			wantLine: 0,
		},
		{
			name:     "reference to W parameter in [W-1:0]",
			pos:      ast.Position{Line: 5, Column: 14},
			wantName: "W",
			wantKind: KindParam,
			wantLine: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sym := an.SymbolAt(tc.pos)
			if sym == nil {
				t.Fatalf("SymbolAt(%+v) returned nil, want %q", tc.pos, tc.wantName)
			}
			if sym.Name != tc.wantName || sym.Kind != tc.wantKind {
				t.Errorf("got sym=(%s, %s), want (%s, %s)", sym.Name, sym.Kind, tc.wantName, tc.wantKind)
			}
			if sym.SelectionRange.Start.Line != tc.wantLine {
				t.Errorf("declaration line = %d, want %d", sym.SelectionRange.Start.Line, tc.wantLine)
			}
		})
	}
}

func TestAnalyzeSemanticDiagnostics(t *testing.T) {
	src := "module m(input wire a, output wire b);\n" +
		"  wire a;\n" +
		"  assign b = unknown_sig;\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	an := Analyze(file)
	if len(an.Diagnostics) != 2 {
		t.Fatalf("expected 2 semantic diagnostics (duplicate 'a' and undeclared 'unknown_sig'), got %d: %+v",
			len(an.Diagnostics), an.Diagnostics)
	}
}

func TestAnalyzeFunctionScope(t *testing.T) {
	src := "module m(input wire [7:0] din, output wire [7:0] dout);\n" +
		"  function [7:0] invert(input [7:0] v);\n" +
		"    reg [7:0] tmp;\n" +
		"    begin\n" +
		"      tmp = ~v;\n" +
		"      invert = tmp;\n" +
		"    end\n" +
		"  endfunction\n" +
		"  assign dout = invert(din);\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// Hover/definition on `v` in `tmp = ~v;` (line 4, col 13) -> resolves to param at line 1
	symArg := an.SymbolAt(ast.Position{Line: 4, Column: 13})
	if symArg == nil || symArg.Name != "v" || symArg.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(v) = %+v, want 'v' declared at line 1", symArg)
	}

	// Hover/definition on `invert` in `assign dout = invert(din);` (line 8, col 16) -> resolves to function at line 1
	symFn := an.SymbolAt(ast.Position{Line: 8, Column: 16})
	if symFn == nil || symFn.Name != "invert" || symFn.Kind != KindFunction || symFn.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(invert) = %+v, want KindFunction at line 1", symFn)
	}
}

func TestAnalyzeGenerateScope(t *testing.T) {
	src := "module m #(parameter USE_FAST = 1) (\n" +
		"  input wire [3:0] a,\n" +
		"  output wire [3:0] y\n" +
		");\n" +
		"  genvar i;\n" +
		"  generate\n" +
		"    for (i = 0; i < 4; i = i + 1) begin : g_loop\n" +
		"      if (USE_FAST) begin : g_fast\n" +
		"        wire bit_w;\n" +
		"        assign bit_w = a[i];\n" +
		"        assign y[i] = bit_w;\n" +
		"      end else begin : g_slow\n" +
		"        wire bit_w;\n" +
		"        assign bit_w = ~a[i];\n" +
		"        assign y[i] = bit_w;\n" +
		"      end\n" +
		"    end\n" +
		"  endgenerate\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for generate block, got %+v", an.Diagnostics)
	}

	// Verify genvar `i` in `assign bit_w = a[i];` (line 9, col 25) resolves to `genvar i` at line 4
	symGenvar := an.SymbolAt(ast.Position{Line: 9, Column: 25})
	if symGenvar == nil || symGenvar.Name != "i" || symGenvar.TypeDetail != "genvar" || symGenvar.SelectionRange.Start.Line != 4 {
		t.Errorf("SymbolAt(i) = %+v, want genvar 'i' at line 4", symGenvar)
	}
}

func TestFindModuleInstantiations(t *testing.T) {
	src := "module child(input wire a, output wire y);\n" +
		"  assign y = a;\n" +
		"endmodule\n" +
		"module top(input wire [1:0] a, output wire [1:0] y);\n" +
		"  child u0 (.a(a[0]), .y(y[0]));\n" +
		"  generate\n" +
		"    if (1) begin : g_inst\n" +
		"      child u1 (.a(a[1]), .y(y[1]));\n" +
		"    end\n" +
		"  endgenerate\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	ranges := an.FindModuleInstantiations("child")
	if len(ranges) != 2 {
		t.Fatalf("got %d instantiations, want 2: %+v", len(ranges), ranges)
	}
	if ranges[0].Start.Line != 4 || ranges[0].Start.Column != 2 {
		t.Errorf("first instantiation = %+v, want line 4 col 2", ranges[0])
	}
	if ranges[1].Start.Line != 7 || ranges[1].Start.Column != 6 {
		t.Errorf("second instantiation = %+v, want line 7 col 6", ranges[1])
	}
	if got := an.FindModuleInstantiations("nonexistent"); len(got) != 0 {
		t.Errorf("expected 0 instantiations for nonexistent, got %d", len(got))
	}
}

func TestAnalyzeTaskScope(t *testing.T) {
	src := "module m(input wire clk, input wire [7:0] a, output reg [7:0] y);\n" +
		"  task automatic compute(input [7:0] in_v, output [7:0] out_v);\n" +
		"    reg [7:0] tmp;\n" +
		"    begin\n" +
		"      tmp = in_v + 8'd1;\n" +
		"      out_v = tmp;\n" +
		"    end\n" +
		"  endtask\n" +
		"  always @(posedge clk) begin\n" +
		"    compute(a, y);\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// Local variable `tmp` inside task (line 5, col 14) -> resolves to `reg [7:0] tmp` at line 2
	symTmp := an.SymbolAt(ast.Position{Line: 5, Column: 14})
	if symTmp == nil || symTmp.Name != "tmp" || symTmp.Kind != KindNet || symTmp.SelectionRange.Start.Line != 2 {
		t.Errorf("SymbolAt(tmp) = %+v, want local net 'tmp' at line 2", symTmp)
	}

	// Task call `compute` at line 9, col 5 -> resolves to KindTask at line 1
	symTask := an.SymbolAt(ast.Position{Line: 9, Column: 5})
	if symTask == nil || symTask.Name != "compute" || symTask.Kind != KindTask || symTask.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(compute) = %+v, want KindTask 'compute' at line 1", symTask)
	}
}

func TestAnalyzeHierarchicalReferences(t *testing.T) {
	src := "module leaf(input wire a, output wire y);\n" +
		"  wire deep_sig = a;\n" +
		"  assign y = deep_sig;\n" +
		"endmodule\n" +
		"module sub(input wire a, output wire y);\n" +
		"  wire internal_sig;\n" +
		"  leaf u_leaf(.a(a), .y(internal_sig));\n" +
		"  assign y = internal_sig;\n" +
		"endmodule\n" +
		"module top(input wire din, output wire y1, output wire y2, output wire y3);\n" +
		"  sub u_sub(.a(din), .y(y1));\n" +
		"  genvar i;\n" +
		"  generate\n" +
		"    for (i = 0; i < 1; i = i + 1) begin : g_blk\n" +
		"      sub u_g(.a(din), .y());\n" +
		"    end\n" +
		"  endgenerate\n" +
		"  assign y2 = u_sub.internal_sig & u_sub.u_leaf.deep_sig;\n" +
		"  assign y3 = g_blk[0].u_g.internal_sig;\n" +
		"  initial begin\n" +
		"    u_sub.reset_task(1'b1);\n" +
		"    u_sub.internal_sig = 1'b0;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for hierarchical references, got %+v", an.Diagnostics)
	}

	// Root instance `u_sub` in `assign y2 = u_sub.internal_sig` (line 17, col 14) still resolves to u_sub.
	symRoot := an.SymbolAt(ast.Position{Line: 17, Column: 14})
	if symRoot == nil || symRoot.Name != "u_sub" || symRoot.Kind != KindInstance {
		t.Errorf("SymbolAt(u_sub) = %+v, want KindInstance 'u_sub'", symRoot)
	}

	// Unknown root identifier in hierarchical path must still be reported as undeclared.
	badSrc := "module top(output wire y);\n" +
		"  assign y = unknown_inst.sig;\n" +
		"endmodule\n"
	badFile, badErrs := parser.Parse(badSrc)
	if len(badErrs) != 0 {
		t.Fatalf("unexpected parse errors: %v", badErrs)
	}
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || badAn.Diagnostics[0].Message != `undeclared identifier "unknown_inst"` {
		t.Errorf("expected 1 undeclared diagnostic for unknown_inst, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeProceduralBlockLocals(t *testing.T) {
	src := "module m(input wire clk, input wire [7:0] din, output reg [7:0] y1, output reg [7:0] y2);\n" +
		"  always @(*) begin : blk1\n" +
		"    integer i;\n" +
		"    reg [7:0] tmp;\n" +
		"    localparam MASK = 8'hFF;\n" +
		"    tmp = din & MASK;\n" +
		"    for (i = 0; i < 8; i = i + 1) begin\n" +
		"      y1[i] = tmp[i];\n" +
		"    end\n" +
		"  end\n" +
		"  always @(posedge clk) begin : blk2\n" +
		"    integer i;\n" +
		"    reg [7:0] tmp;\n" +
		"    tmp = y1;\n" +
		"    for (i = 0; i < 8; i = i + 1) begin\n" +
		"      y2[i] <= tmp[i];\n" +
		"    end\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for block-local declarations, got %+v", an.Diagnostics)
	}

	// `tmp` in blk1 (line 7, col 14) resolves to blk1's `reg [7:0] tmp` at line 3.
	symTmp1 := an.SymbolAt(ast.Position{Line: 7, Column: 14})
	if symTmp1 == nil || symTmp1.Name != "tmp" || symTmp1.SelectionRange.Start.Line != 3 {
		t.Errorf("SymbolAt(tmp in blk1) = %+v, want line 3", symTmp1)
	}

	// `tmp` in blk2 (line 15, col 15) resolves to blk2's `reg [7:0] tmp` at line 12.
	symTmp2 := an.SymbolAt(ast.Position{Line: 15, Column: 15})
	if symTmp2 == nil || symTmp2.Name != "tmp" || symTmp2.SelectionRange.Start.Line != 12 {
		t.Errorf("SymbolAt(tmp in blk2) = %+v, want line 12", symTmp2)
	}

	// Undeclared identifier inside procedural block must still be reported.
	badSrc := "module m(output reg y);\n" +
		"  always @(*) begin : blk\n" +
		"    reg tmp;\n" +
		"    tmp = missing_sig;\n" +
		"    y = tmp;\n" +
		"  end\n" +
		"endmodule\n"
	badFile, badErrs := parser.Parse(badSrc)
	if len(badErrs) != 0 {
		t.Fatalf("unexpected parse errors: %v", badErrs)
	}
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || badAn.Diagnostics[0].Message != `undeclared identifier "missing_sig"` {
		t.Errorf("expected 1 undeclared diagnostic for missing_sig, got %+v", badAn.Diagnostics)
	}
}
