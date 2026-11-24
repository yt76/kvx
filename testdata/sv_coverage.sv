// Regression test for SystemVerilog functional coverage: covergroup, coverpoint, cross, and bins.
class TxCoverage;
    logic [7:0] addr;
    logic [1:0] mode;

    covergroup cg_tx;
        option.per_instance = 1;
        cp_addr: coverpoint addr {
            bins low    = {[8'h00:8'h3F]};
            bins high[] = {[8'h40:8'h7F]};
        }
        cp_mode: coverpoint mode;
        cr_addr_mode: cross cp_addr, cp_mode;
    endgroup : cg_tx

    function new();
        cg_tx = new();
    endfunction

    function void sample_tx(input logic [7:0] a, input logic [1:0] m);
        this.addr = a;
        this.mode = m;
        cg_tx.sample();
    endfunction
endclass

module sv_coverage (
    input logic       clk,
    input logic       rst_n,
    input logic [7:0] addr,
    input logic [1:0] cmd,
    input logic       valid
);
    covergroup cg_bus @(posedge clk iff rst_n);
        option.per_instance = 1;
        type_option.weight  = 2;

        cp_addr: coverpoint addr iff (valid) {
            bins zero          = {8'h00};
            bins quartile[4]   = {[8'h01:8'h7F]};
            bins even          = {[8'h80:8'hDF]} with (item % 2 == 0);
            wildcard bins high = {8'b111?_????};
            ignore_bins  ign   = {8'hFE};
            illegal_bins bad   = {8'hFF};
            bins         rest  = default;
        }

        cp_cmd: coverpoint cmd {
            bins cmds[] = {[2'b00:2'b10]};
            bins trans  = (2'b00 => 2'b01 => 2'b10);
        }

        cr_addr_cmd: cross cp_addr, cp_cmd {
            ignore_bins ign_zero = binsof(cp_addr) intersect {8'h00};
        }
    endgroup : cg_bus

    cg_bus u_cg;
    TxCoverage u_tx_cov;

    initial begin
        u_cg = new();
        u_tx_cov = new();
    end

    always @(posedge clk) begin
        if (valid) begin
            u_cg.sample();
            u_tx_cov.sample_tx(addr, cmd);
        end
    end
endmodule
