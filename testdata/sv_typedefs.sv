typedef logic [7:0] byte_t;

typedef enum logic [1:0] {
    ST_IDLE = 2'd0,
    ST_RUN  = 2'd1,
    ST_DONE = 2'd2
} state_t;

typedef struct packed {
    byte_t addr;
    logic  valid;
} pkt_t;

typedef union packed {
    logic [7:0] raw;
    struct packed {
        logic [3:0] hi;
        logic [3:0] lo;
    } nib;
} word_u;

module sv_typedefs #(
    parameter type T = byte_t,
    parameter state_t INIT_ST = ST_IDLE
) (
    input  logic   clk,
    input  logic   rst_n,
    input  state_t state_in,
    input  pkt_t   pkt_in,
    input  T       data_in,
    output state_t state_out,
    output pkt_t   pkt_out,
    output T       data_out
);
    enum logic { MODE_A, MODE_B } mode_q;
    state_t cur_state, next_state;
    pkt_t   pkt_q;
    word_u  u_val;
    T       buf_val;

    function automatic state_t advance_state(input state_t s);
        if (s == ST_IDLE)
            return ST_RUN;
        return ST_DONE;
    endfunction

    always_comb begin
        next_state = advance_state(cur_state);
        pkt_out    = '{addr: data_in, valid: pkt_in.valid};
        u_val      = word_u'(data_in);
        data_out   = T'(u_val.raw);
        state_out  = state_t'(next_state);
    end

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) begin
            cur_state <= INIT_ST;
            pkt_q     <= '{default: '0};
            mode_q    <= MODE_A;
            buf_val   <= '0;
        end else begin
            cur_state <= state_in;
            pkt_q     <= pkt_in;
            mode_q    <= MODE_B;
            buf_val   <= data_in;
        end
    end
endmodule
