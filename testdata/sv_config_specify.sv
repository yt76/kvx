// Verilog/SystemVerilog config ... endconfig and specify ... endspecify detailed regression test

module leaf_cell (
    input  wire clk,
    input  wire en,
    input  wire d,
    output wire q
);
    reg notifier;
    assign q = en ? d : 1'b0;

    specify
        specparam tSU = 0.5, tHD = 0.2;
        specparam tRISE = 1.0, tFALL = 1.2;
        specparam PATHPULSE$clk$q = (0.1, 0.2);

        pulsestyle_onevent q;
        noshowcancelled q;

        if (en) (posedge clk => (q +: d)) = (tRISE, tFALL);
        ifnone (posedge clk => (q +: d)) = (1.5, 1.5);
        $setuphold(posedge clk, posedge d, tSU, tHD, notifier);
        $width(posedge clk, tSU);
    endspecify
endmodule

module top_design (
    input  wire clk,
    input  wire en,
    input  wire d,
    output wire q
);
    leaf_cell u_leaf (
        .clk(clk),
        .en(en),
        .d(d),
        .q(q)
    );
endmodule

config top_cfg;
    design work.top_design;
    default liblist rtl_lib gate_lib;
    instance top_design.u_leaf use gate_lib.leaf_cell;
    cell leaf_cell liblist gate_lib;
endconfig : top_cfg
