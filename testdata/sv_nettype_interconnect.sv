// SystemVerilog user-defined nettype and interconnect regression test

package net_pkg;
    typedef struct {
        real voltage;
        real current;
    } analog_s;

    function automatic real sum_res(input real drivers[]);
        return 0.0;
    endfunction

    nettype real wreal_t;
    nettype real wreal_sum_t with sum_res;
    nettype analog_s analog_net_t;
endpackage

nettype net_pkg::wreal_t top_wreal_t;

module analog_driver (
    output net_pkg::wreal_t out_v,
    inout  interconnect     bus_pin
);
    assign out_v = 1.8;
endmodule

module interconnect_top (
    interconnect       a,
    interconnect [3:0] bus
);
    import net_pkg::*;

    nettype struct {
        real v;
        real i;
    } local_vi_t;

    wreal_t      vdd;
    wreal_sum_t  vsum;
    top_wreal_t  vtop;
    analog_net_t node_a;
    local_vi_t   node_b;
    interconnect ic_wire;
    interconnect [3:0] ic_bus;

    analog_driver u_drv (
        .out_v(vdd),
        .bus_pin(ic_wire)
    );

    assign vsum = vdd;
    assign vtop = vsum;
    assign node_a.voltage = 1.2;
    assign node_b.v = node_a.voltage;
endmodule
