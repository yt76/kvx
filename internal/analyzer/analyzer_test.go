package analyzer

import (
	"strings"
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

func TestAnalyzeSystemVerilogUserDefinedTypes(t *testing.T) {
	src := "typedef enum logic [1:0] { IDLE, RUN, DONE } state_t;\n" +
		"typedef struct packed { logic [7:0] addr; logic valid; } pkt_t;\n" +
		"module fsm(input logic clk, input pkt_t pin, output state_t sout, output pkt_t pout);\n" +
		"  state_t cur_st;\n" +
		"  always_ff @(posedge clk) begin\n" +
		"    if (cur_st == IDLE) cur_st <= RUN;\n" +
		"    else cur_st <= DONE;\n" +
		"  end\n" +
		"  assign sout = state_t'(cur_st);\n" +
		"  assign pout = '{addr: pin.addr, valid: pin.valid};\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for typedef/enum/struct, got %+v", an.Diagnostics)
	}

	// Enum member `IDLE` at line 5, col 18 resolves to GlobalSymbol `IDLE` (line 0).
	symIdle := an.SymbolAt(ast.Position{Line: 5, Column: 18})
	if symIdle == nil || symIdle.Name != "IDLE" || symIdle.Kind != KindParam || symIdle.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(IDLE) = %+v, want enum member IDLE at line 0", symIdle)
	}

	// Type name `state_t` in `state_t cur_st;` (line 3, col 3) resolves to KindType `state_t` (line 0).
	symType := an.SymbolAt(ast.Position{Line: 3, Column: 3})
	if symType == nil || symType.Name != "state_t" || symType.Kind != KindType || symType.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(state_t) = %+v, want KindType state_t at line 0", symType)
	}
}

func TestAnalyzeSystemVerilogPackages(t *testing.T) {
	src := "package my_pkg;\n" +
		"  localparam int W = 8;\n" +
		"  typedef enum logic { S0, S1 } st_t;\n" +
		"endpackage\n" +
		"module top import my_pkg::W; (input logic [W-1:0] a, output logic [W-1:0] y, output my_pkg::st_t s);\n" +
		"  import my_pkg::*;\n" +
		"  assign y = a + my_pkg::W;\n" +
		"  assign s = S1;\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// `my_pkg::W` at line 6, col 25 resolves to `W` at line 1 in my_pkg.
	symW := an.SymbolAt(ast.Position{Line: 6, Column: 25})
	if symW == nil || symW.Name != "W" || symW.PackageName != "my_pkg" || symW.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(my_pkg::W) = %+v, want W in my_pkg at line 1", symW)
	}

	// `S1` at line 7, col 13 resolves to enum member S1 at line 2 in my_pkg.
	symS1 := an.SymbolAt(ast.Position{Line: 7, Column: 13})
	if symS1 == nil || symS1.Name != "S1" || symS1.PackageName != "my_pkg" || symS1.SelectionRange.Start.Line != 2 {
		t.Errorf("SymbolAt(S1) = %+v, want S1 in my_pkg at line 2", symS1)
	}

	// Reference to nonexistent symbol inside a known package must be reported.
	badSrc := "package p; localparam int A = 1; endpackage\n" +
		"module m(output logic y); assign y = p::MISSING; endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || badAn.Diagnostics[0].Message != `undeclared identifier "p::MISSING"` {
		t.Errorf("expected 1 undeclared diagnostic for p::MISSING, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogInterfaces(t *testing.T) {
	src := "interface bus_if (input logic clk);\n" +
		"  logic valid;\n" +
		"  logic ready;\n" +
		"  modport master (input clk, ready, output valid);\n" +
		"  modport slave  (input clk, valid, output ready);\n" +
		"endinterface\n" +
		"module producer (bus_if.master bus, input logic en);\n" +
		"  assign bus.valid = en;\n" +
		"endmodule\n" +
		"module top (input logic clk, input logic en);\n" +
		"  bus_if u_bus (.clk(clk));\n" +
		"  producer u_prod (.bus(u_bus.master), .en(en));\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// `bus_if` in `bus_if.master bus` (line 6, col 18) resolves to KindInterface `bus_if` at line 0.
	symIf := an.SymbolAt(ast.Position{Line: 6, Column: 18})
	if symIf == nil || symIf.Kind != KindInterface || symIf.Name != "bus_if" || symIf.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(bus_if) = %+v, want KindInterface bus_if at line 0", symIf)
	}

	// `master` in `bus_if.master bus` (line 6, col 25) resolves to KindModport `master` at line 3.
	symMp := an.SymbolAt(ast.Position{Line: 6, Column: 25})
	if symMp == nil || symMp.Kind != KindModport || symMp.Name != "master" || symMp.SelectionRange.Start.Line != 3 {
		t.Errorf("SymbolAt(master) = %+v, want KindModport master at line 3", symMp)
	}

	// `bus_if` in `bus_if u_bus (.clk(clk));` (line 10, col 3) resolves to KindInterface `bus_if` at line 0.
	symInstIf := an.SymbolAt(ast.Position{Line: 10, Column: 3})
	if symInstIf == nil || symInstIf.Kind != KindInterface || symInstIf.Name != "bus_if" || symInstIf.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(inst bus_if) = %+v, want KindInterface bus_if at line 0", symInstIf)
	}
}

func TestAnalyzeSystemVerilogControlAndModifiers(t *testing.T) {
	src := "module m (input var logic clk, input var logic [7:0] x, output var logic [7:0] y);\n" +
		"  const int N = 4;\n" +
		"  static logic [7:0] arr [0:3];\n" +
		"  logic [7:0] q [$];\n" +
		"  always_ff @(posedge clk) begin\n" +
		"    foreach (arr[idx]) begin\n" +
		"      if (idx < N && (x inside {[0:7]}) && (x ==? 8'b0000_????))\n" +
		"        arr[idx] <= {<<{x}};\n" +
		"    end\n" +
		"  end\n" +
		"  assign y = arr[0] ^ q[0];\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// `idx` referenced on line 6, col 11 resolves to the foreach loop variable declared on line 5.
	symIdx := an.SymbolAt(ast.Position{Line: 6, Column: 11})
	if symIdx == nil || symIdx.Name != "idx" || symIdx.SelectionRange.Start.Line != 5 {
		t.Errorf("SymbolAt(idx) = %+v, want foreach loop variable 'idx' at line 5", symIdx)
	}
}

func TestAnalyzeStructAndInterfaceMemberResolution(t *testing.T) {
	src := "typedef struct packed {\n" +
		"  logic [7:0] addr;\n" +
		"  logic       valid;\n" +
		"} pkt_t;\n" +
		"interface bus_if (input logic clk);\n" +
		"  logic valid;\n" +
		"  modport master (input clk, output valid);\n" +
		"endinterface\n" +
		"module m (bus_if.master bus, input pkt_t pkt, output logic [7:0] a);\n" +
		"  assign bus.valid = pkt.valid;\n" +
		"  assign a = pkt.addr;\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics, got %+v", an.Diagnostics)
	}

	// `valid` in `bus.valid` (line 9, col 14) resolves to `logic valid` in bus_if (line 5).
	symBusValid := an.SymbolAt(ast.Position{Line: 9, Column: 14})
	if symBusValid == nil || symBusValid.Name != "valid" || symBusValid.InterfaceName != "bus_if" || symBusValid.SelectionRange.Start.Line != 5 {
		t.Errorf("SymbolAt(bus.valid) = %+v, want valid in bus_if at line 5", symBusValid)
	}

	// `addr` in `pkt.addr` (line 10, col 18) resolves to KindField `addr` in pkt_t (line 1).
	symAddr := an.SymbolAt(ast.Position{Line: 10, Column: 18})
	if symAddr == nil || symAddr.Kind != KindField || symAddr.Name != "addr" || symAddr.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(pkt.addr) = %+v, want KindField addr at line 1", symAddr)
	}

	// Invalid struct field and invalid interface member must produce warnings.
	badSrc := "typedef struct packed { logic a; } s_t;\n" +
		"interface bi; logic v; endinterface\n" +
		"module top (bi b, input s_t s, output logic y);\n" +
		"  assign y = s.nope & b.missing;\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 2 {
		t.Fatalf("expected 2 unknown member diagnostics, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeImplicitAndNamedPortConnections(t *testing.T) {
	// Valid `.sig` and `.*` connections
	validSrc := "module sub #(parameter int W = 8) (input logic clk, input logic [W-1:0] a, output logic [W-1:0] y);\n" +
		"  assign y = a;\n" +
		"endmodule\n" +
		"module top (input logic clk, input logic [7:0] a, output logic [7:0] y0, output logic [7:0] y1);\n" +
		"  sub #(.W(8)) u0 (.clk, .a, .y(y0));\n" +
		"  sub u1 (.*, .y(y1));\n" +
		"endmodule\n"
	file, errs := parser.Parse(validSrc)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for valid .sig / .*, got %+v", an.Diagnostics)
	}

	// SymbolAt on `.clk` (line 4, col 21) resolves to port `clk` of `sub` at line 0.
	symClk := an.SymbolAt(ast.Position{Line: 4, Column: 21})
	if symClk == nil || symClk.Kind != KindPort || symClk.Name != "clk" || symClk.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(.clk) = %+v, want KindPort clk at line 0", symClk)
	}

	// SymbolAt on `.*` (line 5, col 11) resolves to wildcard summary pointing to `sub` at line 0.
	symWild := an.SymbolAt(ast.Position{Line: 5, Column: 11})
	if symWild == nil || symWild.SelectionRange.Start.Line != 0 || !strings.Contains(symWild.Signature, "input logic clk") {
		t.Errorf("SymbolAt(.*) = %+v, want wildcard summary containing 'input logic clk'", symWild)
	}

	// Invalid cases:
	// 1. `.*` missing `rst_n` in parent module
	// 2. `.bad_port` unknown port on `sub`
	// 3. `#(.BAD_PARAM(1))` unknown parameter on `sub`
	// 4. duplicate `.clk` connection
	badSrc := "module sub #(parameter int W = 8) (input logic clk, input logic rst_n, output logic y);\n" +
		"  assign y = clk & rst_n;\n" +
		"endmodule\n" +
		"module top (input logic clk, output logic y);\n" +
		"  sub #(.BAD_PARAM(1)) u0 (.*, .bad_port(clk), .clk(clk), .clk(clk));\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 4 {
		t.Fatalf("expected 4 diagnostics (unknown param, unknown port, duplicate port, missing .* signal), got %d: %+v",
			len(badAn.Diagnostics), badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogAssertions(t *testing.T) {
	src := "module sva_m (input logic clk, input logic rst_n, input logic req, input logic ack);\n" +
		"  property p_hs(logic c, logic r, logic a);\n" +
		"    @(posedge c) disable iff (!rst_n) r |-> ##1 a;\n" +
		"  endproperty\n" +
		"  u_hs: assert property (p_hs(clk, req, ack));\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for SVA, got %+v", an.Diagnostics)
	}

	// `p_hs` in `assert property (p_hs(...))` (line 4, col 26) resolves to KindProperty `p_hs` at line 1.
	symProp := an.SymbolAt(ast.Position{Line: 4, Column: 26})
	if symProp == nil || symProp.Kind != KindProperty || symProp.Name != "p_hs" || symProp.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(p_hs) = %+v, want KindProperty p_hs at line 1", symProp)
	}

	// Formal parameter `r` inside `p_hs` body (line 2, col 38) resolves to formal port `r` at line 1.
	symR := an.SymbolAt(ast.Position{Line: 2, Column: 38})
	if symR == nil || symR.Name != "r" || symR.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(r in property) = %+v, want formal port 'r' at line 1", symR)
	}

	// Undeclared signal inside an assertion must be diagnosed.
	badSrc := "module m (input logic clk); assert property (@(posedge clk) missing_sig); endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"missing_sig"`) {
		t.Errorf("expected undeclared diagnostic for missing_sig in assertion, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogClasses(t *testing.T) {
	src := "class BasePkt;\n" +
		"  rand logic [7:0] addr;\n" +
		"  function new(input logic [7:0] a = 8'h00);\n" +
		"    this.addr = a;\n" +
		"  endfunction\n" +
		"endclass\n" +
		"class ExtPkt extends BasePkt;\n" +
		"  rand logic [7:0] data;\n" +
		"  constraint c_valid { addr != 8'hFF; data inside {[8'h00:8'h7F]}; }\n" +
		"  function logic [7:0] calc();\n" +
		"    return this.addr ^ this.data;\n" +
		"  endfunction\n" +
		"endclass\n" +
		"module tb(output logic [7:0] out_v);\n" +
		"  ExtPkt pkt;\n" +
		"  initial begin\n" +
		"    pkt = new();\n" +
		"    pkt.addr = 8'h10;\n" +
		"    out_v = pkt.calc();\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for classes, got %+v", an.Diagnostics)
	}

	// `this.addr` on line 10, col 17 resolves to inherited property `addr` in BasePkt (line 1).
	symThisAddr := an.SymbolAt(ast.Position{Line: 10, Column: 17})
	if symThisAddr == nil || symThisAddr.Name != "addr" || symThisAddr.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(this.addr) = %+v, want inherited 'addr' at line 1", symThisAddr)
	}

	// `pkt.calc()` on line 18, col 17 resolves to `calc` method in ExtPkt (line 9).
	symCalc := an.SymbolAt(ast.Position{Line: 18, Column: 17})
	if symCalc == nil || symCalc.Kind != KindFunction || symCalc.Name != "calc" || symCalc.SelectionRange.Start.Line != 9 {
		t.Errorf("SymbolAt(pkt.calc) = %+v, want KindFunction calc at line 9", symCalc)
	}

	// Member completions on `pkt.` at line 18 must include both inherited `addr` and own `data` / `calc`.
	members := an.MemberCompletions(ast.Position{Line: 18, Column: 16}, "pkt", nil)
	memberNames := make(map[string]bool, len(members))
	for _, m := range members {
		memberNames[m.Name] = true
	}
	if !memberNames["addr"] || !memberNames["data"] || !memberNames["calc"] {
		t.Errorf("MemberCompletions(pkt.) = %v, want addr, data, calc", memberNames)
	}
}

func TestAnalyzeLinterDiagnostics(t *testing.T) {
	src := "module lint_demo (\n" +
		"  input  logic clk,\n" +
		"  input  logic a,\n" +
		"  output logic y_comb,\n" +
		"  output logic y_ff,\n" +
		"  output logic multi\n" +
		");\n" +
		"  wire w;\n" +
		"  assign a = 1'b0;\n" +
		"  assign multi = a;\n" +
		"  always_comb begin\n" +
		"    w = a;\n" +
		"    y_comb <= a;\n" +
		"    multi = 1'b1;\n" +
		"  end\n" +
		"  always_ff @(posedge clk) begin\n" +
		"    int local_tmp;\n" +
		"    local_tmp = 1;\n" +
		"    y_ff = a;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 5 {
		t.Fatalf("expected 5 linter diagnostics, got %d: %+v", len(an.Diagnostics), an.Diagnostics)
	}

	wantSubstrings := []string{
		`cannot assign to input port "a"`,
		`cannot procedurally assign to net "w"`,
		`non-blocking assignment '<=' in always_comb`,
		`multiple drivers to "multi"`,
		`blocking assignment "=" to "y_ff" in always_ff`,
	}
	for _, want := range wantSubstrings {
		found := false
		for _, d := range an.Diagnostics {
			if strings.Contains(d.Message, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected diagnostic containing %q in %+v", want, an.Diagnostics)
		}
	}
}

func TestAnalyzeSystemVerilogClockingAndProgram(t *testing.T) {
	src := "program automatic my_prog (input logic clk, input logic ack, output logic req);\n" +
		"  clocking cb @(posedge clk);\n" +
		"    default input #1step output #1ns;\n" +
		"    input  ack;\n" +
		"    output req;\n" +
		"  endclocking : cb\n" +
		"  initial begin\n" +
		"    ##1;\n" +
		"    cb.req <= cb.ack;\n" +
		"  end\n" +
		"endprogram\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for clocking/program, got %+v", an.Diagnostics)
	}

	// `my_prog` signature should start with "program my_prog".
	progScope := an.Modules["my_prog"]
	if progScope == nil || !strings.HasPrefix(progScope.Symbol.Signature, "program my_prog") {
		t.Errorf("unexpected program symbol: %+v", progScope)
	}

	// `cb.req` on line 8, col 8 resolves to clocking signal `req` at line 4.
	symReq := an.SymbolAt(ast.Position{Line: 8, Column: 8})
	if symReq == nil || symReq.Name != "req" || symReq.SelectionRange.Start.Line != 4 {
		t.Errorf("SymbolAt(cb.req) = %+v, want clocking signal 'req' at line 4", symReq)
	}
}

func TestAnalyzeSystemVerilogCovergroup(t *testing.T) {
	src := "module cov_m (input logic clk, input logic [7:0] addr, input logic [1:0] cmd);\n" +
		"  covergroup cg @(posedge clk);\n" +
		"    option.per_instance = 1;\n" +
		"    cp_addr: coverpoint addr { bins b0 = {0}; }\n" +
		"    cp_cmd:  coverpoint cmd;\n" +
		"    cr_both: cross cp_addr, cp_cmd;\n" +
		"  endgroup\n" +
		"  cg u_cg;\n" +
		"  initial begin\n" +
		"    u_cg = new();\n" +
		"    u_cg.sample();\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for covergroup, got %+v", an.Diagnostics)
	}

	// `cp_addr` in `cross cp_addr, cp_cmd;` (line 5, col 21) resolves to coverpoint `cp_addr` at line 3.
	symCP := an.SymbolAt(ast.Position{Line: 5, Column: 21})
	if symCP == nil || symCP.Name != "cp_addr" || symCP.SelectionRange.Start.Line != 3 {
		t.Errorf("SymbolAt(cp_addr in cross) = %+v, want coverpoint cp_addr at line 3", symCP)
	}

	// `u_cg.` member completions include `sample`, `get_coverage`, `cp_addr`, and `cr_both`.
	members := an.MemberCompletions(ast.Position{Line: 10, Column: 9}, "u_cg", nil)
	names := make(map[string]bool, len(members))
	for _, m := range members {
		names[m.Name] = true
	}
	if !names["sample"] || !names["get_coverage"] || !names["cp_addr"] || !names["cr_both"] {
		t.Errorf("MemberCompletions(u_cg.) = %v, want sample, get_coverage, cp_addr, cr_both", names)
	}
}

func TestAnalyzeSystemVerilogBind(t *testing.T) {
	src := "module dut (input logic clk, input logic req);\n" +
		"  logic internal_sig;\n" +
		"  assign internal_sig = req;\n" +
		"endmodule\n" +
		"module chk (input logic clk, input logic req, input logic sig);\n" +
		"endmodule\n" +
		"bind dut chk u_chk (.*, .sig(internal_sig));\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for bind, got %+v", an.Diagnostics)
	}

	// `dut` in `bind dut chk u_chk` (line 6, col 6) resolves to module `dut` at line 0.
	symDut := an.SymbolAt(ast.Position{Line: 6, Column: 6})
	if symDut == nil || symDut.Kind != KindModule || symDut.Name != "dut" || symDut.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(bind target dut) = %+v, want KindModule dut at line 0", symDut)
	}

	// `chk` in `bind dut chk u_chk` (line 6, col 10) resolves to module `chk` at line 4.
	symChk := an.SymbolAt(ast.Position{Line: 6, Column: 10})
	if symChk == nil || symChk.Kind != KindModule || symChk.Name != "chk" || symChk.SelectionRange.Start.Line != 4 {
		t.Errorf("SymbolAt(bound module chk) = %+v, want KindModule chk at line 4", symChk)
	}

	// `internal_sig` in `.sig(internal_sig)` (line 6, col 32) resolves to `internal_sig` in `dut` (line 1).
	symSig := an.SymbolAt(ast.Position{Line: 6, Column: 32})
	if symSig == nil || symSig.Name != "internal_sig" || symSig.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(internal_sig in bind) = %+v, want internal_sig in dut at line 1", symSig)
	}

	// Invalid signal inside bind port connection must be diagnosed in target scope.
	badSrc := "module dut (input logic clk); endmodule\n" +
		"module chk (input logic clk, input logic x); endmodule\n" +
		"bind dut chk u_chk (.clk(clk), .x(missing_in_dut));\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"missing_in_dut"`) {
		t.Errorf("expected 1 diagnostic for missing_in_dut in bind, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogDPI(t *testing.T) {
	src := "import \"DPI-C\" pure function real c_sin(real x);\n" +
		"import \"DPI-C\" context c_init = function chandle sv_init(input string name);\n" +
		"import \"DPI-C\" context task c_wait(input int n, output shortreal v);\n" +
		"module tb;\n" +
		"  export \"DPI-C\" function sv_cb;\n" +
		"  function int sv_cb(int a);\n" +
		"    return a;\n" +
		"  endfunction\n" +
		"  chandle h;\n" +
		"  real r;\n" +
		"  shortreal sr;\n" +
		"  initial begin\n" +
		"    h = sv_init(\"ok\");\n" +
		"    r = c_sin(1.0);\n" +
		"    c_wait(5, sr);\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for DPI-C, got %+v", an.Diagnostics)
	}

	// `sv_init` call (line 12, col 10) resolves to top-level DPI function `sv_init` at line 1.
	symInit := an.SymbolAt(ast.Position{Line: 12, Column: 10})
	if symInit == nil || symInit.Kind != KindFunction || symInit.Name != "sv_init" || symInit.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(sv_init call) = %+v, want KindFunction sv_init at line 1", symInit)
	}

	// `sv_cb` in `export "DPI-C" function sv_cb;` (line 4, col 28) resolves to `sv_cb` at line 5.
	symExport := an.SymbolAt(ast.Position{Line: 4, Column: 28})
	if symExport == nil || symExport.Kind != KindFunction || symExport.Name != "sv_cb" || symExport.SelectionRange.Start.Line != 5 {
		t.Errorf("SymbolAt(sv_cb in export) = %+v, want KindFunction sv_cb at line 5", symExport)
	}
}

func TestAnalyzeSystemVerilogCheckerAndLet(t *testing.T) {
	src := "let TOP_EN = 1'b1;\n" +
		"checker my_chk (input logic clk, untyped req, ack = 1'b0);\n" +
		"  let active(r) = r && TOP_EN;\n" +
		"  property p_ok; @(posedge clk) active(req) |-> ack; endproperty\n" +
		"  assert property (p_ok);\n" +
		"endchecker\n" +
		"module top (input logic clk, input logic req, input logic ack);\n" +
		"  my_chk u_chk (.clk(clk), .req(req), .ack(ack));\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for checker/let, got %+v", an.Diagnostics)
	}

	// `my_chk` in instantiation (line 7, col 4) resolves to checker `my_chk` at line 1.
	symChk := an.SymbolAt(ast.Position{Line: 7, Column: 4})
	if symChk == nil || symChk.Kind != KindModule || symChk.Name != "my_chk" || symChk.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(my_chk inst) = %+v, want KindModule my_chk at line 1", symChk)
	}

	// `active` in property (line 3, col 34) resolves to `let active(r)` at line 2.
	symLet := an.SymbolAt(ast.Position{Line: 3, Column: 34})
	if symLet == nil || symLet.Kind != KindFunction || symLet.Name != "active" || symLet.SelectionRange.Start.Line != 2 {
		t.Errorf("SymbolAt(active let call) = %+v, want KindFunction active at line 2", symLet)
	}
	if symLet != nil && symLet.Signature != "let active(r)" {
		t.Errorf("let Signature = %q, want %q", symLet.Signature, "let active(r)")
	}
}

func TestAnalyzeSystemVerilogParameterizedClass(t *testing.T) {
	src := "class packet;\n" +
		"  logic [31:0] addr;\n" +
		"  logic [31:0] data;\n" +
		"  function void print();\n" +
		"  endfunction\n" +
		"endclass\n" +
		"class base_box #(type T = packet, int DEPTH = 8);\n" +
		"  T item;\n" +
		"endclass\n" +
		"class ext_box #(type U = packet) extends base_box #(U, 16);\n" +
		"  U extra;\n" +
		"endclass\n" +
		"typedef base_box #(packet, 16) pkt_box_t;\n" +
		"typedef ext_box #(packet) pkt_ext_t;\n" +
		"module tb;\n" +
		"  base_box #(packet) b1;\n" +
		"  base_box #(.T(packet)) b2;\n" +
		"  pkt_box_t b3;\n" +
		"  pkt_ext_t e1;\n" +
		"  logic [31:0] a;\n" +
		"  initial begin\n" +
		"    a = b1.item.addr;\n" +
		"    a = b2.item.data;\n" +
		"    a = b3.item.addr;\n" +
		"    a = e1.item.addr;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for parameterized classes, got %+v", an.Diagnostics)
	}

	// `addr` in `b1.item.addr` (line 21, col 17) resolves to `packet.addr` at line 1.
	symAddr := an.SymbolAt(ast.Position{Line: 21, Column: 17})
	if symAddr == nil || symAddr.Name != "addr" || symAddr.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(b1.item.addr) = %+v, want packet.addr at line 1", symAddr)
	}

	// `addr` in `e1.item.addr` (line 24, col 17) resolves through inheritance + typedef to `packet.addr` at line 1.
	symExtAddr := an.SymbolAt(ast.Position{Line: 24, Column: 17})
	if symExtAddr == nil || symExtAddr.Name != "addr" || symExtAddr.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(e1.item.addr) = %+v, want packet.addr at line 1", symExtAddr)
	}

	// Chained MemberCompletions on `b1.item.` and `e1.item.` return `addr`, `data`, `print`.
	for _, base := range []string{"b1.item", "b2.item", "b3.item", "e1.item"} {
		members := an.MemberCompletions(ast.Position{Line: 21, Column: 16}, base, nil)
		names := make(map[string]bool, len(members))
		for _, m := range members {
			names[m.Name] = true
		}
		if !names["addr"] || !names["data"] || !names["print"] {
			t.Errorf("MemberCompletions(%q) = %v, want addr, data, print", base, names)
		}
	}

	// Unknown member on parameterized member chain must be diagnosed.
	badSrc := "class packet; logic [31:0] addr; endclass\n" +
		"class box #(type T = int); T item; endclass\n" +
		"module tb;\n" +
		"  box #(packet) b;\n" +
		"  logic [31:0] a;\n" +
		"  initial begin a = b.item.no_such_field; end\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"no_such_field"`) {
		t.Errorf("expected 1 diagnostic for unknown field on parameterized class member, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogContainers(t *testing.T) {
	src := "typedef int int_q_t[$];\n" +
		"class pkt; logic [31:0] addr; endclass\n" +
		"module tb;\n" +
		"  int dyn[];\n" +
		"  int q[$];\n" +
		"  int map[string];\n" +
		"  string s;\n" +
		"  int_q_t tq;\n" +
		"  pkt pkts[$];\n" +
		"  int n;\n" +
		"  logic [31:0] a;\n" +
		"  initial begin\n" +
		"    n = dyn.size();\n" +
		"    q.push_back(1);\n" +
		"    n = map.exists(\"k\");\n" +
		"    n = s.len();\n" +
		"    tq.push_front(2);\n" +
		"    pkts.push_back(null);\n" +
		"    a = pkts[0].addr;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for containers, got %+v", an.Diagnostics)
	}

	// SymbolAt on `push_back` in `q.push_back(1);` (line 13, col 8) resolves to built-in method.
	symPush := an.SymbolAt(ast.Position{Line: 13, Column: 8})
	if symPush == nil || symPush.Kind != KindFunction || symPush.Name != "push_back" {
		t.Errorf("SymbolAt(q.push_back) = %+v, want built-in method push_back", symPush)
	}

	// SymbolAt on `len` in `s.len()` (line 15, col 11) resolves to string built-in method `len`.
	symLen := an.SymbolAt(ast.Position{Line: 15, Column: 11})
	if symLen == nil || symLen.Kind != KindFunction || symLen.Name != "len" {
		t.Errorf("SymbolAt(s.len) = %+v, want built-in method len", symLen)
	}

	// MemberCompletions check for each container kind.
	checkMembers := func(base string, want ...string) {
		members := an.MemberCompletions(ast.Position{Line: 13, Column: 6}, base, nil)
		got := make(map[string]bool, len(members))
		for _, m := range members {
			got[m.Name] = true
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("MemberCompletions(%q) missing %q (got %v)", base, w, got)
			}
		}
	}
	checkMembers("dyn", "size", "delete", "sort", "find")
	checkMembers("q", "push_back", "pop_front", "insert", "size", "find")
	checkMembers("tq", "push_back", "pop_front", "size")
	checkMembers("map", "num", "exists", "first", "next", "delete")
	checkMembers("s", "len", "substr", "toupper", "compare", "atoi")
	checkMembers("pkts", "push_back", "size", "addr")

	// Unknown method on queue/string must be diagnosed.
	badSrc := "module tb;\n" +
		"  int q[$];\n" +
		"  string s;\n" +
		"  int n;\n" +
		"  initial begin\n" +
		"    n = q.nonexistent();\n" +
		"    n = s.nonexistent();\n" +
		"  end\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 2 {
		t.Errorf("expected 2 diagnostics for nonexistent methods on queue and string, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogRandcaseAndRandsequence(t *testing.T) {
	src := "module tb;\n" +
		"  int x;\n" +
		"  int w;\n" +
		"  initial begin\n" +
		"    randcase\n" +
		"      w : x = 1;\n" +
		"      2 : x = 2;\n" +
		"    endcase\n" +
		"    randsequence (main)\n" +
		"      main : step(x) ;\n" +
		"      int step(int v) : { x = v + 1; } ;\n" +
		"    endsequence\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for randcase/randsequence, got %+v", an.Diagnostics)
	}

	// Undeclared signal inside randcase weight or randsequence code block must be diagnosed.
	badSrc := "module tb;\n" +
		"  int x;\n" +
		"  initial begin\n" +
		"    randcase\n" +
		"      missing_w : x = 1;\n" +
		"    endcase\n" +
		"    randsequence (main)\n" +
		"      main : { x = missing_v; } ;\n" +
		"    endsequence\n" +
		"  end\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 2 {
		t.Errorf("expected 2 diagnostics for missing_w and missing_v, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogExpect(t *testing.T) {
	src := "module tb (input logic clk, input logic req, input logic ack);\n" +
		"  int cnt;\n" +
		"  property p_hs; @(posedge clk) req |-> ##1 ack; endproperty\n" +
		"  initial begin\n" +
		"    expect (p_hs) cnt = cnt + 1; else $error(\"fail\");\n" +
		"    expect (@(posedge clk) req ##[1:3] ack);\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for expect, got %+v", an.Diagnostics)
	}

	// `p_hs` inside `expect (p_hs)` (line 4, col 13) resolves to property `p_hs` at line 2.
	symProp := an.SymbolAt(ast.Position{Line: 4, Column: 13})
	if symProp == nil || symProp.Kind != KindProperty || symProp.Name != "p_hs" || symProp.SelectionRange.Start.Line != 2 {
		t.Errorf("SymbolAt(p_hs in expect) = %+v, want KindProperty p_hs at line 2", symProp)
	}

	// Undeclared signal inside expect property expression must be diagnosed.
	badSrc := "module tb (input logic clk);\n" +
		"  initial begin\n" +
		"    expect (@(posedge clk) missing_sig);\n" +
		"  end\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"missing_sig"`) {
		t.Errorf("expected 1 diagnostic for missing_sig in expect, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeSystemVerilogNettypeAndInterconnect(t *testing.T) {
	src := "package np;\n" +
		"  typedef struct { real v; real i; } vi_s;\n" +
		"  function automatic real res_fn(input real d[]); return 0.0; endfunction\n" +
		"  nettype real wreal_t with res_fn;\n" +
		"  nettype vi_s vi_net_t;\n" +
		"endpackage\n" +
		"module top (interconnect a, interconnect [3:0] bus);\n" +
		"  import np::*;\n" +
		"  wreal_t n1;\n" +
		"  vi_net_t n2;\n" +
		"  interconnect w;\n" +
		"  assign n1 = 1.0;\n" +
		"  assign n2.v = n1;\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for nettype/interconnect, got %+v", an.Diagnostics)
	}

	// `res_fn` in `nettype real wreal_t with res_fn;` (line 3, col 29) resolves to `res_fn` at line 2.
	symRes := an.SymbolAt(ast.Position{Line: 3, Column: 29})
	if symRes == nil || symRes.Kind != KindFunction || symRes.Name != "res_fn" || symRes.SelectionRange.Start.Line != 2 {
		t.Errorf("SymbolAt(res_fn in nettype with) = %+v, want KindFunction res_fn at line 2", symRes)
	}

	// `v` in `assign n2.v = n1;` (line 12, col 12) resolves through `nettype vi_s vi_net_t` to `vi_s.v`.
	symField := an.SymbolAt(ast.Position{Line: 12, Column: 12})
	if symField == nil || symField.Name != "v" {
		t.Errorf("SymbolAt(n2.v) = %+v, want field v of vi_s", symField)
	}
}

func TestAnalyzeSystemVerilogPackageExport(t *testing.T) {
	src := "package base_pkg;\n" +
		"  typedef logic [7:0] byte_t;\n" +
		"  localparam int BASE_W = 8;\n" +
		"  localparam int SECRET_W = 4;\n" +
		"endpackage\n" +
		"package mid_pkg;\n" +
		"  import base_pkg::*;\n" +
		"  export base_pkg::byte_t;\n" +
		"  localparam int MID_W = BASE_W + 1;\n" +
		"endpackage\n" +
		"package all_pkg;\n" +
		"  import base_pkg::*;\n" +
		"  export *::*;\n" +
		"endpackage\n" +
		"module tb;\n" +
		"  import mid_pkg::*;\n" +
		"  import all_pkg::BASE_W;\n" +
		"  byte_t b;\n" +
		"  int v;\n" +
		"  initial begin\n" +
		"    b = 8'h55;\n" +
		"    v = BASE_W + MID_W;\n" +
		"  end\n" +
		"endmodule\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for package export, got %+v", an.Diagnostics)
	}

	// `byte_t` in `export base_pkg::byte_t;` (line 7, col 20) resolves to `base_pkg::byte_t` at line 1.
	symExp := an.SymbolAt(ast.Position{Line: 7, Column: 20})
	if symExp == nil || symExp.Kind != KindType || symExp.Name != "byte_t" || symExp.SelectionRange.Start.Line != 1 {
		t.Errorf("SymbolAt(byte_t in export) = %+v, want byte_t at line 1", symExp)
	}

	// `SECRET_W` was NOT re-exported by `mid_pkg`, so importing only `mid_pkg::*` must NOT expose `SECRET_W`.
	badSrc := "package base_pkg; localparam int SECRET_W = 4; endpackage\n" +
		"package mid_pkg; import base_pkg::*; endpackage\n" +
		"module tb; import mid_pkg::*; int v; initial v = SECRET_W; endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"SECRET_W"`) {
		t.Errorf("expected 1 diagnostic for non-re-exported SECRET_W, got %+v", badAn.Diagnostics)
	}
}

func TestAnalyzeConfigAndSpecify(t *testing.T) {
	src := "module leaf (input wire clk, input wire d, output wire q);\n" +
		"  reg notifier;\n" +
		"  assign q = d;\n" +
		"  specify\n" +
		"    specparam tSU = 0.5, tHD = 0.2;\n" +
		"    (posedge clk => (q +: d)) = (1.0, 1.2);\n" +
		"    $setuphold(posedge clk, posedge d, tSU, tHD, notifier);\n" +
		"  endspecify\n" +
		"endmodule\n" +
		"config my_cfg;\n" +
		"  design work.leaf;\n" +
		"  default liblist rtl_lib;\n" +
		"  cell leaf use gate_lib.leaf;\n" +
		"endconfig : my_cfg\n"

	file, errs := parser.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	an := Analyze(file)
	if len(an.Diagnostics) != 0 {
		t.Fatalf("expected 0 semantic diagnostics for config/specify, got %+v", an.Diagnostics)
	}

	// `tSU` in `$setuphold(..., tSU, ...)` (line 6, col 40) resolves to `specparam tSU` at line 4.
	symSpec := an.SymbolAt(ast.Position{Line: 6, Column: 40})
	if symSpec == nil || symSpec.Kind != KindParam || symSpec.Name != "tSU" || symSpec.SelectionRange.Start.Line != 4 {
		t.Errorf("SymbolAt(tSU in specify) = %+v, want KindParam tSU at line 4", symSpec)
	}

	// `leaf` in `design work.leaf;` (line 10, col 15) resolves to `module leaf` at line 0.
	symDesign := an.SymbolAt(ast.Position{Line: 10, Column: 15})
	if symDesign == nil || symDesign.Kind != KindModule || symDesign.Name != "leaf" || symDesign.SelectionRange.Start.Line != 0 {
		t.Errorf("SymbolAt(leaf in config design) = %+v, want KindModule leaf at line 0", symDesign)
	}

	// Undeclared signal in specify block must be diagnosed.
	badSrc := "module m (input wire clk, output wire q);\n" +
		"  assign q = clk;\n" +
		"  specify\n" +
		"    (clk => missing_out) = 1.0;\n" +
		"  endspecify\n" +
		"endmodule\n"
	badFile, _ := parser.Parse(badSrc)
	badAn := Analyze(badFile)
	if len(badAn.Diagnostics) != 1 || !strings.Contains(badAn.Diagnostics[0].Message, `"missing_out"`) {
		t.Errorf("expected 1 diagnostic for missing_out in specify, got %+v", badAn.Diagnostics)
	}
}
