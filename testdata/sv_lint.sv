// Regression test for semantic linter diagnostics:
// 1. Assignment to input port (`in_a`)
// 2. Procedural assignment to wire (`net_w`)
// 3. Non-blocking assignment (`<=`) in always_comb
// 4. Blocking assignment (`=`) to module-level signal in always_ff
// 5. Multiple drivers to the same signal across always_comb and assign
module sv_lint (
    input  logic clk,
    input  logic in_a,
    output logic out_comb,
    output logic out_ff,
    output logic multi_sig
);
    wire net_w;

    assign in_a = 1'b0;      // 1. cannot assign to input port "in_a"
    assign multi_sig = in_a;

    always_comb begin
        net_w = in_a;        // 2. cannot procedurally assign to net "net_w"
        out_comb <= in_a;    // 3. non-blocking assignment '<=' in always_comb block
        multi_sig = 1'b1;    // 5. multiple drivers to "multi_sig"
    end

    always_ff @(posedge clk) begin
        out_ff = in_a;       // 4. blocking assignment "=" to "out_ff" in always_ff block
    end
endmodule
