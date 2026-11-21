// Regression test for SystemVerilog OOP: classes, inheritance, virtual methods, constraints, and virtual interfaces.
interface pkt_bus_if (input logic clk);
    logic [7:0] addr;
    logic [7:0] data;
    logic       valid;
endinterface

typedef class ExtPacket;

virtual class BasePacket #(parameter int W = 8);
    rand logic [W-1:0] addr;
    rand logic [W-1:0] data;
    protected int      id;
    static int         pkt_count = 0;

    constraint c_addr_range {
        addr inside {[8'h00:8'h7F]};
        data dist { 8'h00 := 10, [8'h01:8'hFF] :/ 90 };
    }

    function new(input int init_id = 0);
        this.id   = init_id;
        this.addr = '0;
        this.data = '0;
        pkt_count++;
    endfunction

    pure virtual function logic [W-1:0] calc_crc();
endclass : BasePacket

class ExtPacket extends BasePacket #(8);
    rand bit       has_crc;
    randc bit [1:0] tag;
    local logic [7:0] cached_crc;
    virtual pkt_bus_if vif;

    constraint c_order {
        solve has_crc before addr;
        soft tag == 2'b01;
        (has_crc == 1'b1) -> (addr != 8'h00);
    }

    function new(input int init_id = 1);
        super.new(init_id);
        this.has_crc = 1'b1;
        this.cached_crc = 8'h00;
    endfunction

    virtual function logic [7:0] calc_crc();
        this.cached_crc = this.addr ^ this.data;
        return this.cached_crc;
    endfunction

    extern virtual task drive_bus();
endclass : ExtPacket

task ExtPacket::drive_bus();
    if (vif != null) begin
        vif.addr  = addr;
        vif.data  = data;
        vif.valid = 1'b1;
    end
endtask

module sv_classes_tb (
    input  logic       clk,
    output logic [7:0] crc_out
);
    pkt_bus_if u_bus (.clk(clk));
    ExtPacket pkt;

    initial begin
        pkt = new(42);
        pkt.vif = u_bus;
        pkt.addr = 8'h12;
        pkt.data = 8'h34;
        crc_out = pkt.calc_crc();
        pkt.drive_bus();
    end
endmodule
