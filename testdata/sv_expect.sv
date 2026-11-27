// SystemVerilog expect property statement regression test

module sv_expect_tb (
    input logic clk,
    input logic rst_n,
    input logic req,
    input logic ack
);
    int pass_cnt;
    int fail_cnt;
    bit ok;

    property p_handshake;
        @(posedge clk) disable iff (!rst_n) req |-> ##[1:4] ack;
    endproperty

    initial begin
        pass_cnt = 0;
        fail_cnt = 0;
        ok = 1'b0;

        expect (p_handshake);

        expect (@(posedge clk) req ##[1:4] ack)
            pass_cnt = pass_cnt + 1;
        else
            fail_cnt = fail_cnt + 1;

        lbl_exp: expect (@(posedge clk) disable iff (!rst_n) req |-> ##1 ack) begin
            ok = 1'b1;
        end else begin
            ok = 1'b0;
        end

        expect (p_handshake) else $error("handshake failed");
    end
endmodule
