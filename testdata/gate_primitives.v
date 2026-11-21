module gate_primitives (
    input  wire a,
    input  wire b,
    input  wire oe,
    output wire y_and,
    output wire y_or,
    output wire y_not,
    output wire y_nand,
    output wire y_xor,
    inout  wire io,
    output wire p_up
);
    and (y_and, a, b);
    or (y_or, a, b);
    not (y_not, a);
    bufif1 (io, a, oe);
    nand #1 (y_nand, a, b);
    xor u_xor (y_xor, a, b);
    pullup (p_up);
endmodule
