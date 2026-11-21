// Regression test for SystemVerilog control constructs, operators, and modifiers.
module sv_control_ops (
    input  var logic        clk,
    input  var logic [7:0]  addr,
    input  var logic [31:0] din,
    output var logic [31:0] dout,
    output var logic        match_flag
);
    const int MAX_ITEMS = 4;
    static logic [7:0] mem [0:3];
    var logic [7:0] dyn_arr [];
    logic [7:0] fifo_q [$];
    logic [7:0] bounded_q [$:15];
    int lut [string];
    string tag = "kvx";

    function static logic [31:0] swap_bytes(input var logic [31:0] v);
        return {<<8{v}};
    endfunction

    // Wildcard comparison (==?, !=?) and `inside` operator
    always_comb begin
        if (addr inside {8'h00, [8'h10:8'h1F], 8'hFF}) begin
            match_flag = 1'b1;
        end else if (addr ==? 8'b1010_???? && addr !=? 8'b1010_0000) begin
            match_flag = 1'b1;
        end else begin
            match_flag = 1'b0;
        end
    end

    // `case (...) inside` and streaming operator
    always_comb begin
        case (addr) inside
            [8'h00:8'h0F]: dout = {>>{din}};
            8'b1???:       dout = swap_bytes(din);
            default:       dout = din;
        endcase
    end

    // `foreach` loop with implicit loop variable declaration
    always_ff @(posedge clk) begin
        foreach (mem[i]) begin
            if (i < MAX_ITEMS) begin
                mem[i] <= din[7:0];
            end
        end
    end

    // `fork ... join_any` and `fork ... join_none`
    initial begin
        fork
            $display("tag=%s", tag);
            begin
                lut["k"] = 1;
            end
        join_any

        fork
            $display("q_len=%0d", fifo_q[0]);
        join_none
    end
endmodule
