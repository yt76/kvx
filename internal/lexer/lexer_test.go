package lexer

import (
	"testing"
)

func TestLexerTokens(t *testing.T) {
	input := "`timescale 1ns/1ps\n" +
		"// Single line comment\n" +
		"/* Block comment */\n" +
		"module counter #(parameter WIDTH = 8) (\n" +
		"  input wire clk,\n" +
		"  output reg [WIDTH-1:0] count\n" +
		");\n" +
		"  always @(posedge clk) begin\n" +
		"    if (count == 8'hFF)\n" +
		"      count <= '0;\n" +
		"    else\n" +
		"      count <= count + 1'b1;\n" +
		"  end\n" +
		"endmodule\n"

	want := []struct {
		typ  TokenType
		lit  string
		line int
		col  int
	}{
		{TokenModule, "module", 3, 0},
		{TokenIdent, "counter", 3, 7},
		{TokenHash, "#", 3, 15},
		{TokenLParen, "(", 3, 16},
		{TokenParameter, "parameter", 3, 17},
		{TokenIdent, "WIDTH", 3, 27},
		{TokenAssignOp, "=", 3, 33},
		{TokenNumber, "8", 3, 35},
		{TokenRParen, ")", 3, 36},
		{TokenLParen, "(", 3, 38},
		{TokenInput, "input", 4, 2},
		{TokenWire, "wire", 4, 8},
		{TokenIdent, "clk", 4, 13},
		{TokenComma, ",", 4, 16},
		{TokenOutput, "output", 5, 2},
		{TokenReg, "reg", 5, 9},
		{TokenLBracket, "[", 5, 13},
		{TokenIdent, "WIDTH", 5, 14},
		{TokenMinus, "-", 5, 19},
		{TokenNumber, "1", 5, 20},
		{TokenColon, ":", 5, 21},
		{TokenNumber, "0", 5, 22},
		{TokenRBracket, "]", 5, 23},
		{TokenIdent, "count", 5, 25},
		{TokenRParen, ")", 6, 0},
		{TokenSemicolon, ";", 6, 1},
		{TokenAlways, "always", 7, 2},
		{TokenAt, "@", 7, 9},
		{TokenLParen, "(", 7, 10},
		{TokenPosedge, "posedge", 7, 11},
		{TokenIdent, "clk", 7, 19},
		{TokenRParen, ")", 7, 22},
		{TokenBegin, "begin", 7, 24},
		{TokenIf, "if", 8, 4},
		{TokenLParen, "(", 8, 7},
		{TokenIdent, "count", 8, 8},
		{TokenEq, "==", 8, 14},
		{TokenNumber, "8'hFF", 8, 17},
		{TokenRParen, ")", 8, 22},
		{TokenIdent, "count", 9, 6},
		{TokenLeq, "<=", 9, 12},
		{TokenNumber, "'0", 9, 15},
		{TokenSemicolon, ";", 9, 17},
		{TokenElse, "else", 10, 4},
		{TokenIdent, "count", 11, 6},
		{TokenLeq, "<=", 11, 12},
		{TokenIdent, "count", 11, 15},
		{TokenPlus, "+", 11, 21},
		{TokenNumber, "1'b1", 11, 23},
		{TokenSemicolon, ";", 11, 27},
		{TokenEnd, "end", 12, 2},
		{TokenEndmodule, "endmodule", 13, 0},
		{TokenEOF, "", 14, 0},
	}

	tokens := Tokenize(input)
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}

	for i, w := range want {
		got := tokens[i]
		if got.Type != w.typ || got.Literal != w.lit {
			t.Errorf("token[%d] = (%s, %q), want (%s, %q)", i, got.Type, got.Literal, w.typ, w.lit)
		}
		if got.Range.Start.Line != w.line || got.Range.Start.Column != w.col {
			t.Errorf("token[%d] (%q) start = %d:%d, want %d:%d",
				i, got.Literal, got.Range.Start.Line, got.Range.Start.Column, w.line, w.col)
		}
	}
}

func TestLexerEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType TokenType
		wantLit  string
	}{
		{
			name:     "signed hex literal",
			input:    "16'shA5_F0",
			wantType: TokenNumber,
			wantLit:  "16'shA5_F0",
		},
		{
			name:     "unsized hex literal",
			input:    "'hFF",
			wantType: TokenNumber,
			wantLit:  "'hFF",
		},
		{
			name:     "unsized binary literal",
			input:    "'b01_01",
			wantType: TokenNumber,
			wantLit:  "'b01_01",
		},
		{
			name:     "unsized signed decimal literal",
			input:    "'sd42",
			wantType: TokenNumber,
			wantLit:  "'sd42",
		},
		{
			name:     "unsized fill one literal",
			input:    "'1",
			wantType: TokenNumber,
			wantLit:  "'1",
		},
		{
			name:     "real literal with decimal point",
			input:    "1.5",
			wantType: TokenNumber,
			wantLit:  "1.5",
		},
		{
			name:     "real literal with scientific notation",
			input:    "1e-9",
			wantType: TokenNumber,
			wantLit:  "1e-9",
		},
		{
			name:     "real literal with decimal and positive exponent",
			input:    "2.5_0E+3",
			wantType: TokenNumber,
			wantLit:  "2.5_0E+3",
		},
		{
			name:     "system task identifier",
			input:    "$display",
			wantType: TokenSystemIdent,
			wantLit:  "$display",
		},
		{
			name:     "macro identifier usage",
			input:    "`DATA_W",
			wantType: TokenIdent,
			wantLit:  "`DATA_W",
		},
		{
			name:     "escaped identifier",
			input:    "\\bus+index ",
			wantType: TokenIdent,
			wantLit:  "\\bus+index",
		},
		{
			name:     "string literal",
			input:    `"hello \"verilog\""`,
			wantType: TokenString,
			wantLit:  `hello \"verilog\"`,
		},
		{
			name:     "unterminated block comment",
			input:    "/* unclosed comment",
			wantType: TokenIllegal,
			wantLit:  "unterminated block comment",
		},
		{
			name:     "unterminated string literal",
			input:    "\"unclosed string\n",
			wantType: TokenIllegal,
			wantLit:  "unterminated string literal",
		},
		{
			name:     "arithmetic shift right operator",
			input:    ">>>",
			wantType: TokenAshr,
			wantLit:  ">>>",
		},
		{
			name:     "case inequality operator",
			input:    "!==",
			wantType: TokenCaseNeq,
			wantLit:  "!==",
		},
		{
			name:     "increment operator",
			input:    "++",
			wantType: TokenPlusPlus,
			wantLit:  "++",
		},
		{
			name:     "compound add assign",
			input:    "+=",
			wantType: TokenPlusEq,
			wantLit:  "+=",
		},
		{
			name:     "compound shift left assign",
			input:    "<<=",
			wantType: TokenShlEq,
			wantLit:  "<<=",
		},
		{
			name:     "SystemVerilog logic keyword",
			input:    "logic",
			wantType: TokenLogic,
			wantLit:  "logic",
		},
		{
			name:     "SystemVerilog always_ff keyword",
			input:    "always_ff",
			wantType: TokenAlwaysFF,
			wantLit:  "always_ff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := New(tc.input).NextToken()
			if tok.Type != tc.wantType || tok.Literal != tc.wantLit {
				t.Errorf("got (%s, %q), want (%s, %q)", tok.Type, tok.Literal, tc.wantType, tc.wantLit)
			}
		})
	}
}

func TestTokenizeWithDirectives(t *testing.T) {
	input := "`include \"defs.vh\"\n" +
		"`define WIDTH 8\n" +
		"`ifdef SYNTH\n" +
		"`endif\n"

	// Standard Tokenize should skip all directives and return only EOF.
	plain := Tokenize(input)
	if len(plain) != 1 || plain[0].Type != TokenEOF {
		t.Fatalf("Tokenize() = %+v, want single TokenEOF", plain)
	}

	// TokenizeWithDirectives should emit directive tokens and their arguments.
	got := TokenizeWithDirectives(input)
	want := []struct {
		typ TokenType
		lit string
	}{
		{TokenDirective, "`include"},
		{TokenString, "defs.vh"},
		{TokenDirective, "`define"},
		{TokenIdent, "WIDTH"},
		{TokenNumber, "8"},
		{TokenDirective, "`ifdef"},
		{TokenIdent, "SYNTH"},
		{TokenDirective, "`endif"},
		{TokenEOF, ""},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Literal != w.lit {
			t.Errorf("token[%d] = (%s, %q), want (%s, %q)", i, got[i].Type, got[i].Literal, w.typ, w.lit)
		}
	}
}

func TestTokenizeWithTrivia(t *testing.T) {
	input := "// line comment\n" +
		"/* block\n" +
		"   comment */\n" +
		"wire a;\n"

	got := TokenizeWithTrivia(input)
	want := []struct {
		typ TokenType
		lit string
	}{
		{TokenComment, "// line comment"},
		{TokenComment, "/* block\n   comment */"},
		{TokenWire, "wire"},
		{TokenIdent, "a"},
		{TokenSemicolon, ";"},
		{TokenEOF, ""},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Literal != w.lit {
			t.Errorf("token[%d] = (%s, %q), want (%s, %q)", i, got[i].Type, got[i].Literal, w.typ, w.lit)
		}
	}
}
