module sv_sub (
    input  logic       clk,
    input  logic [7:0] din,
    output logic [7:0] dout
);
    assign dout = din;
endmodule

module sv_core #(
    parameter int WIDTH = 8
) (
    input  logic             clk,
    input  logic             rst_n,
    input  logic [1:0]       sel,
    input  logic [WIDTH-1:0] din,
    output logic [WIDTH-1:0] dout,
    output logic [WIDTH-1:0] comb_out,
    output logic [WIDTH-1:0] latch_out
);
    localparam logic [WIDTH-1:0] ZERO = '0;
    logic [3:0][WIDTH-1:0] pipe;
    logic [WIDTH-1:0]      sub_out;
    bit                    flag;

    sv_sub u_sub0 (.clk, .din(din), .dout(sub_out));
    sv_sub u_sub1 (.*, .dout());

    function automatic logic [WIDTH-1:0] sat_add(input logic [WIDTH-1:0] a, input int unsigned b);
        logic [WIDTH-1:0] res;
        res = a;
        res += b;
        return res;
    endfunction

    function void noop_fn(input bit en);
        if (!en) return;
    endfunction

    always_comb begin
        comb_out = ZERO;
        unique case (sel)
            2'b00: comb_out = din;
            2'b01: comb_out = sub_out;
            default: comb_out = sat_add(din, 1);
        endcase
        priority if (flag) begin
            for (int i = 0; i < 4; i++) begin
                if (i == 2) continue;
                comb_out ^= pipe[i];
                if (i == 3) break;
            end
        end
    end

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) begin
            dout <= ZERO;
            flag <= 1'b0;
        end else begin
            int cnt;
            cnt = 0;
            do begin
                cnt++;
            end while (cnt < 2);
            pipe[0] <= comb_out;
            dout    <= pipe[0];
            flag    <= 1'b1;
        end
    end

    always_latch begin
        if (flag)
            latch_out = comb_out;
    end
endmodule
