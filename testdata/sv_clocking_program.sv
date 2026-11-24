// Regression test for SystemVerilog clocking blocks and program blocks.
interface tb_bus_if (input logic clk, input logic rst_n);
    logic       req;
    logic       ack;
    logic [7:0] wdata;
    logic [7:0] rdata;

    clocking cb @(posedge clk);
        default input #1step output #1ns;
        input  ack, rdata;
        output req, wdata;
    endclocking : cb

    modport tb (clocking cb, input rst_n);
endinterface : tb_bus_if

program automatic bus_test #(
    parameter int NUM_TX = 4
) (
    input  logic       clk,
    tb_bus_if.tb       bus,
    output logic [7:0] last_read
);
    default clocking prog_cb @(posedge clk);
        default input #1step output #0;
        input  ack = bus.cb.ack;
    endclocking : prog_cb

    initial begin
        bus.cb.req   <= 1'b0;
        bus.cb.wdata <= 8'h00;
        ##2;
        for (int i = 0; i < NUM_TX; i++) begin
            @(bus.cb);
            bus.cb.req   <= 1'b1;
            bus.cb.wdata <= 8'(i);
            ##1 bus.cb.req <= 1'b0;
            last_read = bus.cb.rdata;
        end
    end
endprogram : bus_test

module sv_clocking_program_top (
    input  logic       clk,
    input  logic       rst_n,
    output logic [7:0] last_read
);
    global clocking g_cb @(posedge clk);
    endclocking : g_cb

    tb_bus_if u_bus (.clk(clk), .rst_n(rst_n));

    bus_test #(.NUM_TX(4)) u_test (
        .clk(clk),
        .bus(u_bus.tb),
        .last_read(last_read)
    );
endmodule
