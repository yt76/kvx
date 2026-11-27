// SystemVerilog randcase and randsequence statements regression test

module sv_rand_ctrl_tb;
    int x;
    int y;
    int w1;
    int w2;
    int sel;
    bit flag;

    initial begin
        w1 = 3;
        w2 = 1;
        sel = 0;
        flag = 1'b1;

        randcase
            3 : x = 1;
            1 : begin
                x = 2;
                y = 3;
            end
            w1 + w2 : x = 4;
        endcase

        randsequence (main)
            main   : first second done ;
            first  : add := 3 | dec := 1 ;
            second : rand join (0.5) step1 step2 | repeat (2) step1 ;
            done   : { $display("x=%0d y=%0d", x, y); } ;
            add    : { x = x + 1; } ;
            dec    : { x = x - 1; } ;
            step1  : if (flag) { y = 1; } else { y = 2; } ;
            step2  : case (sel)
                         0 : { y = 3; }
                         default : { y = 4; }
                     endcase ;
        endsequence

        randsequence (top)
            top : work(x) ;
            int work(int v) : { return v + 1; } ;
        endsequence

        randsequence ()
            only_one : { x = 0; } ;
        endsequence
    end
endmodule
