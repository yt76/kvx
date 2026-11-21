module block_locals (
    input  wire       clk,
    input  wire [7:0] din,
    output reg  [7:0] comb,
    output reg  [7:0] dout
);
    always @(*) begin : blk_comb
        integer i;
        reg [7:0] tmp;
        localparam MASK = 8'hFF;
        tmp = din & MASK;
        for (i = 0; i < 8; i = i + 1) begin
            comb[i] = tmp[i];
        end
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
