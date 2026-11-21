primitive udp_dff (q, clk, d);
    output q;
    input clk, d;
    reg q;
    initial q = 1'b0;
    table
        // clk  d  : q : q+
           (01) 0  : ? : 0;
           (01) 1  : ? : 1;
           (0?) ?  : ? : -;
           ?   (??): ? : -;
    endtable
endprimitive

module specify_and_udp (
    input  wire clk,
    input  wire d,
    output wire q
);
    reg notifier;
    udp_dff u_dff (q, clk, d);

    specify
        specparam tSU = 0.5, tHD = 0.2;
        (posedge clk => (q +: d)) = (1.0, 1.2);
        $setuphold(posedge clk, posedge d, tSU, tHD, notifier);
    endspecify
endmodule
