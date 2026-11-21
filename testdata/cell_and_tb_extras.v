`begin_keywords "1364-2005"
`unconnected_drive pull0
`define MAKE_NET(prefix, idx) prefix``_net_``idx

primitive udp_buf (y, a);
    output y;
    input a;
    table
        0 : 0;
        1 : 1;
    endtable
endprimitive

macromodule cell_inv (y, a, );
    input a;
    reg y;
    output y;
    always @(*) y = ~a;
endmodule

module cell_and_tb_extras #(
    parameter W = 4
) (
    input  wire          clk,
    input  wire          rst_n,
    input  wire          oe,
    input  uwire [W-1:0] a,
    input  uwire [W-1:0] b,
    inout  tri   [W-1:0] io_pin,
    output reg   [W-1:0] q,
    output wire  [W-1:0] y_xnor,
    output wire          y_udp
);
    localparam DEPTH = 2 ** W;
    tri0   t0_sig;
    tri1   t1_sig;
    triand tand_sig;
    trior  tor_sig;
    trireg (large) cap_node;
    wire vectored [W-1:0] `MAKE_NET(my, 0);
    wire (weak1, weak0) [W-1:0] pull_bus = a;

    assign (strong1, pull0) #1 `MAKE_NET(my, 0) = (a ~^ b) & (a ^~ b);
    assign y_xnor = `MAKE_NET(my, 0) ^ pull_bus;

    bufif1 (strong1, strong0) #1 (io_pin[0], a[0], oe);
    udp_buf (y_udp, a[0]);

    always @(rst_n) begin
        if (!rst_n)
            assign q = {W{1'b0}};
        else
            deassign q;
    end

    always @(posedge clk) begin
        q <= y_xnor;
    end

    initial begin
        force y_xnor = {W{1'b0}};
        #10 release y_xnor;
    end
endmodule

`nounconnected_drive
`end_keywords
