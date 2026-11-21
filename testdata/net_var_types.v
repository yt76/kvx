module net_var_types (
    input  wire       clk,
    inout  tri  [7:0] io_bus,
    output wor        irq_n
);
    supply0 gnd;
    supply1 vdd;
    tri     tri_sig;
    wand    and_bus;
    wor     or_bus;

    real     gain;
    time     ts;
    realtime rts;
    event    ev_done;

    assign and_bus = vdd;
    assign and_bus = ~gnd;
    assign or_bus  = gnd;
    assign irq_n   = or_bus;
    assign tri_sig = io_bus[0];

    function real scale_val(input real x);
        begin
            scale_val = x;
        end
    endfunction

    always @(posedge clk) begin : blk
        real local_r;
        time local_t;
        event local_ev;
        local_r = scale_val(gain);
        local_t = $time;
        ts <= local_t;
        rts <= local_r;
        -> ev_done;
        -> local_ev;
    end

    always @(ev_done) begin
        $display("event at %t", ts);
    end
endmodule
