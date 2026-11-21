`define WIDTH 8
`define BUS(w) [w-1:0]
`define ADD(a, b) ((a) + (b))
`define ADD3(a, b, c) `ADD(`ADD(a, b), c)
`define INC(x, step=1) ((x) + (step))
`define MUX(sel, t, f) ((sel) ? (t) : (f))
`define FF_ASSIGN(clk, q, d) \
    always @(posedge clk) begin \
        q <= d; \
    end

module func_macro (
    input  wire              clk,
    input  wire              sel,
    input  wire `BUS(`WIDTH) a,
    input  wire `BUS(`WIDTH) b,
    input  wire `BUS(`WIDTH) c,
    output wire `BUS(`WIDTH) sum,
    output wire [3:0]        nib,
    output reg  `BUS(`WIDTH) q
);
    assign sum = `ADD3(a, b, `INC(c));
    assign nib = `MUX(sel, {a[1:0], b[1:0]}, c[3:0]);
    `FF_ASSIGN(clk, q, `INC(sum, 2))
endmodule
