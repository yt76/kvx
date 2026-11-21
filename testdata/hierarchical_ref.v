module leaf_cell (
    input  wire a,
    output wire y
);
    wire deep_sig = a;
    assign y = deep_sig;
endmodule

module sub_block (
    input  wire a,
    output wire y
);
    wire internal_sig;
    leaf_cell u_leaf (.a(a), .y(internal_sig));
    assign y = internal_sig;
endmodule

module hierarchical_ref (
    input  wire din,
    output wire y1,
    output wire y2,
    output wire y3
);
    sub_block u_sub (.a(din), .y(y1));

    genvar i;
    generate
        for (i = 0; i < 1; i = i + 1) begin : g_blk
            sub_block u_g (.a(din), .y());
        end
    endgenerate

    assign y2 = u_sub.internal_sig & u_sub.u_leaf.deep_sig;
    assign y3 = g_blk[0].u_g.internal_sig;

    initial begin
        u_sub.reset_task(1'b1);
        u_sub.internal_sig = 1'b0;
    end
endmodule
