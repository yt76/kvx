// Regression test for SystemVerilog Assertions (SVA): immediate, deferred, concurrent, property, and sequence.
interface sva_bus_if (input logic clk, input logic rst_n);
    logic req;
    logic ack;

    property p_bus_req_ack;
        @(posedge clk) disable iff (!rst_n)
        req |-> ##[1:3] ack;
    endproperty

    u_if_assert: assert property (p_bus_req_ack) else $error("bus ack timeout");
endinterface

module sv_assertions (
    input  logic       clk,
    input  logic       rst_n,
    input  logic       req,
    input  logic       ack,
    input  logic [7:0] din,
    output logic [7:0] dout
);
    sequence s_req_pulse(c, r);
        @(posedge c) r ##1 !r;
    endsequence : s_req_pulse

    property p_handshake(logic c, logic r, logic a);
        @(posedge c) disable iff (!rst_n)
        r |=> ##[0:2] a;
    endproperty : p_handshake

    // Concurrent assertions at module level (labeled and unlabeled)
    u_assert_hs: assert property (p_handshake(clk, req, ack))
        else $error("handshake failed");

    u_assume_req: assume property (@(posedge clk) disable iff (!rst_n) req |-> (din != 8'hFF));
    u_cover_pulse: cover sequence (s_req_pulse(clk, req));
    cover property (@(posedge clk) req && ack);
    restrict property (@(posedge clk) rst_n);

    // Immediate and deferred assertions in procedural blocks
    always_comb begin
        dout = din;
        chk_imm: assert (dout == din) else $error("mismatch");
        assert #0 (din !== 8'hxx);
        assert final (dout == din);
        assume (rst_n == 1'b0 || rst_n == 1'b1);
        cover (din == 8'h00);
    end

    final begin
        assert (ack == 1'b0 || ack == 1'b1);
    end
endmodule
