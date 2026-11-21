(* keep_hierarchy = "yes" *)
module attributes_sample #(
    (* fsm_encoding = "one_hot" *) parameter WIDTH = 8
) (
    (* mark_debug = "true" *) input wire clk,
    (* mark_debug = "true", keep = "true" *) input wire [WIDTH-1:0] din,
    (* dont_touch = "yes" *) output reg [WIDTH-1:0] dout
);
    (* ram_style = "block" *) reg [WIDTH-1:0] mem [0:15];
    (* keep = "true" *) (* ASYNC_REG = "TRUE" *) wire [WIDTH-1:0] stage;
    reg [WIDTH-1:0] comb;

    (* keep = "true" *) assign stage = (* no_opt *) din + 1'b1;

    always @(*) begin
        (* full_case, parallel_case *)
        case (din[1:0])
            2'b00: comb = stage;
            default: comb = din;
        endcase
    end

    always @(posedge clk) begin
        mem[0] <= comb;
        dout <= mem[0];
    end
endmodule
