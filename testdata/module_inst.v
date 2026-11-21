module sub_cell (
    input wire a,
    output wire y
);
    assign y = a;
endmodule

module top_inst (
    input wire [1:0] din,
    output wire [1:0] dout
);
    sub_cell u0 (
        .a(din[0]),
        .y(dout[0])
    );

    generate
        if (1) begin : g_cell
            sub_cell u1 (
                .a(din[1]),
                .y(dout[1])
            );
        end
    endgenerate
endmodule
