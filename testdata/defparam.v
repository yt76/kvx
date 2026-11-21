module sub_param #(
    parameter WIDTH = 8,
    parameter DEPTH = 16
) (
    input  wire [WIDTH-1:0] a,
    output wire [WIDTH-1:0] y
);
    assign y = a;
endmodule

module defparam_sample #(
    parameter TOP_W = 16
) (
    input  wire [TOP_W-1:0] din,
    output wire [TOP_W-1:0] dout
);
    sub_param u_sub (.a(din), .y(dout));

    defparam u_sub.WIDTH = TOP_W;
    defparam u_sub.DEPTH = 32, u_sub.WIDTH = TOP_W;
endmodule
