// SystemVerilog package re-export (`export pkg::*;`, `export pkg::sym;`, `export *::*;`) regression test

package base_pkg;
    typedef logic [7:0] byte_t;
    localparam int BASE_W = 8;
    localparam int HIDDEN_W = 4;

    function automatic int add1(int x);
        return x + 1;
    endfunction
endpackage

package selective_pkg;
    import base_pkg::*;
    export base_pkg::byte_t, base_pkg::add1;
    localparam int SEL_W = BASE_W + 1;
endpackage

package reexport_pkg;
    import base_pkg::*;
    export base_pkg::*;
    localparam int RE_W = 16;
endpackage

package wildcard_all_pkg;
    import base_pkg::*;
    export *::*;
    localparam int ALL_W = 32;
endpackage

module pkg_export_tb;
    import selective_pkg::*;
    import reexport_pkg::BASE_W;
    import wildcard_all_pkg::HIDDEN_W;

    byte_t b;
    int    v;

    initial begin
        b = 8'hAA;
        v = add1(BASE_W + HIDDEN_W + SEL_W);
    end
endmodule
