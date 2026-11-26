// SystemVerilog DPI-C import / export regression test

import "DPI-C" pure function real c_sin(real x);
import "DPI-C" context c_init_ctx = function chandle sv_init(input string name);
import "DPI-C" context task c_wait_cycles(input int cycles, output shortreal val);

package dpi_pkg;
    import "DPI-C" pure function int c_add(int a, int b);
    import "DPI-C" context function void c_log(input string msg, input chandle ctx);
endpackage

module dpi_tb;
    import dpi_pkg::*;

    import "DPI-C" function void c_notify(input int id);
    export "DPI-C" function sv_callback;
    export "DPI-C" c_task_cb = task sv_task_cb;

    function automatic int sv_callback(int x);
        return c_add(x, 1);
    endfunction

    task automatic sv_task_cb(input int n);
        c_notify(n);
    endtask

    chandle h;
    real r;
    shortreal sr;
    int sum;

    initial begin
        chandle local_h;
        shortreal local_sr;
        h = sv_init("sim");
        local_h = h;
        r = c_sin(1.0);
        sum = sv_callback(41);
        c_log("done", local_h);
        c_wait_cycles(10, sr);
        local_sr = sr;
        sv_task_cb(sum);
    end
endmodule
