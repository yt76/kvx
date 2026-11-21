`define EMPTY_MARKER
`define MULTI_COMMENT_ADD(a, b) /* line1
   line2 */ ((a) + \
   (b))
`define LOG_VAL(x) $display(`"value (`\`"x`\`") = %h`", x)

config demo_cfg;
    design rtlLib.corner_cases;
    default liblist rtlLib;
endconfig : demo_cfg

module corner_cases (
    input  wire [3:0] a,
    input  wire [3:0] b,
    output wire [3:0] y_gate,
    output reg  [3:0] y_comb
);
    `EMPTY_MARKER

    and [3:0] (y_gate, a, b);

    function [3:0] fn_id(input [3:0] v);
        begin : fn_blk
            fn_id = v;
        end : fn_blk
    endfunction : fn_id

    task tk_log(input [3:0] v);
        begin : tk_blk
            `LOG_VAL(v);
        end : tk_blk
    endtask : tk_log

    always @(* ) begin : comb_blk
        `EMPTY_MARKER
        y_comb = `MULTI_COMMENT_ADD(fn_id(a), b);
    end : comb_blk

    initial begin : init_blk
        fork : fk_blk
            tk_log(a);
        join : fk_blk
    end : init_blk
endmodule : corner_cases
