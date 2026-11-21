module decl_assign_ansi #(
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
    assign #2 y3 = w_dly;

    always @(posedge clk) begin
        q <= y1 + y2;
    end
endmodule

module decl_assign_non_ansi (clk, a, b, q, y1, y2);
    input  wire       clk;
    input  wire [7:0] a, b;
    output reg  [7:0] q = 8'hFF;
    output wire [3:0] y1;
    output wire [3:0] y2;

    assign #(1, 2) y1 = a[3:0], y2 = b[3:0];

    always @(posedge clk) begin
        q <= a;
    end
endmodule
