module procedural_control;
    reg clk;
    reg ready;
    reg done;

    initial begin
        clk = 1'b0;
        forever #5 clk = ~clk;
    end

    initial begin : main_seq
        ready = 1'b0;
        done  = 1'b0;
        repeat (10) @(posedge clk);
        ready = 1'b1;
        wait (ready);

        fork : par_blk
            begin : branch_a
                #20;
                disable branch_b;
            end
            begin : branch_b
                forever #10 done = ~done;
            end
        join
    end
endmodule
