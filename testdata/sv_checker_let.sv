// SystemVerilog checker ... endchecker and let declarations regression test

let TOP_ONE = 1'b1;
let max_val(a, b) = (a > b) ? a : b;
let inc_val(int x, int step = 1) = x + step;

package util_pkg;
    let valid_sig(untyped s, logic en = 1'b1) = (s && en);
endpackage

checker req_ack_chk (
    input logic clk,
    untyped req,
    ack = 1'b0,
    output logic seen_ack = 1'b0
);
    rand bit free_var;
    logic pending;

    let req_active(r) = (r && TOP_ONE);

    default clocking @(posedge clk);
    endclocking

    property p_ack;
        @(posedge clk) req_active(req) |-> ##1 ack;
    endproperty

    assert property (p_ack);

    always_ff @(posedge clk) begin
        pending <= req;
        seen_ack <= ack | free_var;
    end
endchecker : req_ack_chk

module dut_with_checker (
    input  logic       clk,
    input  logic       req,
    input  logic       ack,
    input  logic [7:0] a,
    input  logic [7:0] b,
    output logic [7:0] y,
    output logic       seen
);
    import util_pkg::*;

    let sum_ab = a + b;
    let clamp(logic [7:0] v, logic [7:0] lim = 8'hFF) = (v > lim) ? lim : v;

    req_ack_chk u_chk (
        .clk(clk),
        .req(req),
        .ack(ack),
        .seen_ack(seen)
    );

    always_comb begin
        if (valid_sig(req)) begin
            y = clamp(max_val(a, b));
        end else begin
            y = inc_val(sum_ab, 0);
        end
    end
endmodule
