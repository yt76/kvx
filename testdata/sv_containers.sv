// SystemVerilog container types (dynamic array, queue, associative array, string) built-in methods regression test

typedef int int_q_t[$];
typedef logic [7:0] key_t;

class item_c;
    logic [31:0] addr;
    logic [31:0] data;
endclass

module sv_containers_tb;
    int          dyn_arr[];
    int          q[$];
    int          bounded_q[$:15];
    int          str_map[string];
    logic [7:0]  key_map[key_t];
    int          wild_map[*];
    string       msg;
    int_q_t      td_q;
    item_c       pkts[$];

    int          n;
    string       sub;
    string       k;
    logic [31:0] a;

    initial begin
        // Dynamic array methods
        dyn_arr = new[4];
        n = dyn_arr.size();
        dyn_arr.sort();
        dyn_arr.reverse();
        dyn_arr.delete();

        // Queue methods
        q.push_back(10);
        q.push_front(5);
        q.insert(1, 7);
        n = q.pop_front();
        n = q.pop_back();
        n = q.size();
        q.delete();

        bounded_q.push_back(1);
        td_q.push_back(2);
        n = td_q.size();

        // Associative array methods
        str_map["alpha"] = 1;
        n = str_map.num();
        if (str_map.exists("alpha")) begin
            if (str_map.first(k)) begin
                n = str_map.next(k);
            end
            str_map.delete("alpha");
        end
        n = key_map.size();
        n = wild_map.num();

        // String methods
        msg = "Hello_SV";
        n = msg.len();
        sub = msg.substr(0, 4);
        msg = sub.toupper();
        n = msg.compare("HELLO");

        // Queue of class objects: both container methods and element fields work
        pkts.push_back(null);
        n = pkts.size();
        a = pkts[0].addr;
    end
endmodule
