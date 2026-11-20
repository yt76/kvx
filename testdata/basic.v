module basic #(
    parameter WIDTH = 8
) (
    input wire clk,
    input wire [WIDTH-1:0] din,
    output reg [WIDTH-1:0] dout
);
    always @(posedge clk) begin
        dout <= din;
    end
endmodule
