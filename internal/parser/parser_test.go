package parser

import (
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
