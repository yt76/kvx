// SystemVerilog parameterized classes and type parameter resolution regression test

class packet;
    logic [31:0] addr;
    logic [31:0] data;

    function void print();
    endfunction
endclass

class base_box #(type T = packet, int DEPTH = 8);
    T item;

    function T get();
        return item;
    endfunction

    function void put(T val);
        item = val;
    endfunction
endclass

class ext_box #(type U = packet) extends base_box #(U, 16);
    U extra;

    function U peek();
        return item;
    endfunction
endclass

typedef base_box #(packet, 16) pkt_box_t;
typedef ext_box #(packet)      pkt_ext_t;

module param_class_tb;
    base_box #(packet)     b_pos;
    base_box #(.T(packet)) b_named;
    base_box               b_def;
    pkt_box_t              b_td;
    pkt_ext_t              e_td;
    ext_box #(packet)      e_pos;

    logic [31:0] a;
    logic [31:0] d;

    initial begin
        b_pos   = new();
        b_named = new();
        b_def   = new();
        b_td    = new();
        e_td    = new();
        e_pos   = new();

        a = b_pos.item.addr;
        d = b_named.item.data;
        a = b_def.item.addr;
        d = b_td.item.data;
        a = e_td.item.addr;
        d = e_pos.extra.data;
        b_pos.item.print();
    end
endmodule
