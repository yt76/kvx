module number_literals (
    input  wire        clk,
    output reg  [31:0] data_out,
    output reg  [31:0] fill_out
);
    localparam real PERIOD = 1e-9;
    localparam real RATIO  = 1.5;
    localparam real FREQ   = 2.5E+6;

    real r_val;
    wire [31:0] h_val = 'hFF;
    wire [31:0] b_val = 'b1010_0101;
    wire [31:0] d_val = 'd123;
    wire signed [31:0] sd_val = 'sd42;

    always @(posedge clk) begin
        data_out <= h_val + b_val + d_val;
        fill_out <= '1;
        r_val    <= PERIOD * RATIO + FREQ;
    end
endmodule
