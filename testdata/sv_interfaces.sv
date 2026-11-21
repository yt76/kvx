// Regression test for SystemVerilog interfaces, modports, and interface ports.
interface axi_stream_if #(
    parameter int DATA_W = 32
) (
    input logic clk,
    input logic rst_n
);
    logic              valid;
    logic              ready;
    logic [DATA_W-1:0] data;
    logic              last;

    function automatic logic is_handshake();
        return valid && ready;
    endfunction

    modport master (
        input  clk, rst_n, ready,
        output valid, data, last,
        import is_handshake
    );

    modport slave (
        input  clk, rst_n, valid, data, last,
        output ready,
        import is_handshake
    );
endinterface : axi_stream_if

module producer (
    axi_stream_if.master m_axis,
    input  logic         en,
    input  logic [31:0]  din
);
    always_ff @(posedge m_axis.clk or negedge m_axis.rst_n) begin
        if (!m_axis.rst_n) begin
            m_axis.valid <= 1'b0;
            m_axis.data  <= '0;
            m_axis.last  <= 1'b0;
        end else if (en) begin
            m_axis.valid <= 1'b1;
            m_axis.data  <= din;
            m_axis.last  <= 1'b1;
        end
    end
endmodule

module consumer (
    input logic         ready_in,
    axi_stream_if.slave s_axis,
    output logic [31:0] dout
);
    assign s_axis.ready = ready_in;
    assign dout = s_axis.valid ? s_axis.data : 32'd0;
endmodule

module monitor (
    axi_stream_if bus,
    interface     generic_bus,
    output logic  active
);
    assign active = bus.valid & bus.ready & generic_bus.valid;
endmodule

module top (
    input  logic        clk,
    input  logic        rst_n,
    input  logic        en,
    input  logic [31:0] din,
    output logic [31:0] dout,
    output logic        active
);
    axi_stream_if #(.DATA_W(32)) u_axis (.clk(clk), .rst_n(rst_n));

    producer u_prod (
        .m_axis(u_axis.master),
        .en(en),
        .din(din)
    );

    consumer u_cons (
        .ready_in(1'b1),
        .s_axis(u_axis.slave),
        .dout(dout)
    );

    monitor u_mon (
        .bus(u_axis),
        .generic_bus(u_axis),
        .active(active)
    );
endmodule
