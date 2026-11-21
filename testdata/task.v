module task_sample (
    input  wire       clk,
    input  wire [7:0] din,
    output reg  [7:0] dout
);
    task automatic compute (
        input  [7:0] val,
        output [7:0] res
    );
        reg [7:0] tmp;
        begin
            tmp = val + 8'd1;
            res = tmp;
        end
    endtask

    task clear_out;
        begin
            dout <= 8'h00;
        end
    endtask

    reg [7:0] next_val;
    always @(posedge clk) begin
        if (din == 8'h00) begin
            clear_out;
        end else begin
            compute(din, next_val);
            dout <= next_val;
        end
    end
endmodule
