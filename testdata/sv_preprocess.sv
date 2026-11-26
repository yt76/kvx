// SystemVerilog preprocessor full compliance regression test

`delay_mode_zero
`protect
`endprotect

`define MK_SIG(prefix, name, suffix) logic prefix``name``suffix;
`define MK_HEX(w, val) w``'h``val
`define LOG_VAL(tag, expr) $display(`"tag=expr (`\`"ok`\`")`", `__FILE__, `__LINE__)
`define PREFIX_STR(p, s) `"p``_``s`"

`define FEATURE_A
`ifdef FEATURE_A
    `define MODE_VAL 8'd1
`elsif FEATURE_B
    `define MODE_VAL 8'd2
`else
    `define MODE_VAL 8'd0
`endif

module pp_full_test (
    input  logic       clk,
    input  logic [7:0] din,
    output logic [7:0] dout
);
    // Token pasting with empty prefix / empty suffix
    `MK_SIG(, data_mid, )
    `MK_SIG(r_, pipe, _q)

    // Numeric token pasting
    localparam logic [7:0] MAGIC = `MK_HEX(8, A5);
    localparam logic [7:0] MODE  = `MODE_VAL;

    string msg;
    int    line_no;

    initial begin
        `LOG_VAL(check, din);
        msg = `PREFIX_STR(sys, init);
        line_no = `__LINE__;
    end

    always_ff @(posedge clk) begin
        data_mid <= din[0];
        r_pipe_q <= data_mid;
        dout     <= (din ^ MAGIC) + MODE + {7'd0, r_pipe_q};
    end
endmodule

`undef FEATURE_A
`ifndef FEATURE_A
module pp_after_undef (input logic a, output logic y);
    assign y = a;
endmodule
`endif
