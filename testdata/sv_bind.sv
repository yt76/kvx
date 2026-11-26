// Regression test for SystemVerilog `bind` statements (top-level, instance-list, and hierarchical).
module dut_mod #(parameter int W = 8) (
    input  logic         clk,
    input  logic         req,
    output logic [W-1:0] dout
);
    logic internal_busy;
    assign internal_busy = req;
    assign dout = {W{internal_busy}};
endmodule

module sva_dut_chk #(parameter int W = 8) (
    input logic clk,
    input logic req,
    input logic busy
);
    u_chk: assert property (@(posedge clk) req |-> busy);
endmodule

// Top-level bind to all instances of dut_mod
bind dut_mod sva_dut_chk #(.W(W)) u_chk_all (
    .clk(clk),
    .req(req),
    .busy(internal_busy)
);

module tb_bind_top (
    input  logic       clk,
    input  logic       req,
    output logic [7:0] dout0,
    output logic [7:0] dout1
);
    dut_mod u_dut0 (.clk(clk), .req(req), .dout(dout0));
    dut_mod u_dut1 (.clk(clk), .req(req), .dout(dout1));

    // Bind to specific instances of dut_mod with `.*`
    bind dut_mod : u_dut0, u_dut1 sva_dut_chk u_chk_sel (.*, .busy(internal_busy));

    // Bind using hierarchical target path
    bind tb_bind_top.u_dut0 sva_dut_chk u_chk_hier (.clk(clk), .req(req), .busy(internal_busy));
endmodule
