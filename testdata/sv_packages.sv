package common_pkg;
    localparam int WIDTH = 16;

    typedef enum logic [1:0] {
        ST_IDLE = 2'd0,
        ST_BUSY = 2'd1
    } state_t;

    function automatic logic [WIDTH-1:0] inc_val(input logic [WIDTH-1:0] v);
        return v + 1;
    endfunction
endpackage : common_pkg

module sv_packages
    import common_pkg::WIDTH;
#(
    parameter int W = WIDTH
) (
    input  logic               clk,
    input  common_pkg::state_t st_in,
    input  logic [W-1:0]       din,
    output logic [W-1:0]       dout,
    output common_pkg::state_t st_out
);
    import common_pkg::*;
    state_t cur_st;

    always_ff @(posedge clk) begin
        if (st_in == common_pkg::ST_IDLE) begin
            cur_st <= ST_BUSY;
            dout   <= common_pkg::inc_val(din);
        end else begin
            cur_st <= st_in;
            dout   <= inc_val(din);
        end
    end

    assign st_out = cur_st;
endmodule
