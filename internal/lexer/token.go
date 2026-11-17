package lexer

import "kvx/internal/ast"

// TokenType identifies the lexical category of a Token.
type TokenType string

const (
	TokenEOF     TokenType = "EOF"
	TokenIllegal TokenType = "ILLEGAL"

	// Identifiers and literals
	TokenIdent       TokenType = "IDENT"
	TokenSystemIdent TokenType = "SYSTEM_IDENT" // e.g. $display, $clog2
	TokenNumber      TokenType = "NUMBER"       // e.g. 42, 8'hFF, 1'b0
	TokenString      TokenType = "STRING"       // e.g. "hello"

	// Keywords
	TokenModule     TokenType = "module"
	TokenEndmodule  TokenType = "endmodule"
	TokenInput      TokenType = "input"
	TokenOutput     TokenType = "output"
	TokenInout      TokenType = "inout"
	TokenWire       TokenType = "wire"
	TokenReg        TokenType = "reg"
	TokenInteger    TokenType = "integer"
	TokenParameter  TokenType = "parameter"
	TokenLocalparam TokenType = "localparam"
	TokenSigned     TokenType = "signed"
	TokenAssign     TokenType = "assign"
	TokenAlways     TokenType = "always"
	TokenInitial    TokenType = "initial"
	TokenPosedge    TokenType = "posedge"
	TokenNegedge    TokenType = "negedge"
	TokenOr         TokenType = "or"
	TokenBegin      TokenType = "begin"
	TokenEnd        TokenType = "end"
	TokenIf         TokenType = "if"
	TokenElse       TokenType = "else"
	TokenCase       TokenType = "case"
	TokenCasex      TokenType = "casex"
	TokenCasez      TokenType = "casez"
	TokenEndcase    TokenType = "endcase"
	TokenDefault    TokenType = "default"
	TokenFor        TokenType = "for"
	TokenWhile      TokenType = "while"

	// Delimiters and punctuation
	TokenLParen    TokenType = "("
	TokenRParen    TokenType = ")"
	TokenLBracket  TokenType = "["
	TokenRBracket  TokenType = "]"
	TokenLBrace    TokenType = "{"
	TokenRBrace    TokenType = "}"
	TokenSemicolon TokenType = ";"
	TokenColon     TokenType = ":"
	TokenComma     TokenType = ","
	TokenDot       TokenType = "."
	TokenHash      TokenType = "#"
	TokenAt        TokenType = "@"
	TokenQuestion  TokenType = "?"

	// Operators
	TokenAssignOp TokenType = "="
	TokenLeq      TokenType = "<="
	TokenGeq      TokenType = ">="
	TokenEq       TokenType = "=="
	TokenNeq      TokenType = "!="
	TokenCaseEq   TokenType = "==="
	TokenCaseNeq  TokenType = "!=="
	TokenLt       TokenType = "<"
	TokenGt       TokenType = ">"
	TokenPlus     TokenType = "+"
	TokenMinus    TokenType = "-"
	TokenStar     TokenType = "*"
	TokenSlash    TokenType = "/"
	TokenPercent  TokenType = "%"
	TokenAmp      TokenType = "&"
	TokenPipe     TokenType = "|"
	TokenCaret    TokenType = "^"
	TokenTilde    TokenType = "~"
	TokenBang     TokenType = "!"
	TokenLand     TokenType = "&&"
	TokenLor      TokenType = "||"
	TokenShl      TokenType = "<<"
	TokenShr      TokenType = ">>"
	TokenAshl     TokenType = "<<<"
	TokenAshr     TokenType = ">>>"
)

var keywords = map[string]TokenType{
	"module":     TokenModule,
	"endmodule":  TokenEndmodule,
	"input":      TokenInput,
	"output":     TokenOutput,
	"inout":      TokenInout,
	"wire":       TokenWire,
	"reg":        TokenReg,
	"integer":    TokenInteger,
	"parameter":  TokenParameter,
	"localparam": TokenLocalparam,
	"signed":     TokenSigned,
	"assign":     TokenAssign,
	"always":     TokenAlways,
	"initial":    TokenInitial,
	"posedge":    TokenPosedge,
	"negedge":    TokenNegedge,
	"or":         TokenOr,
	"begin":      TokenBegin,
	"end":        TokenEnd,
	"if":         TokenIf,
	"else":       TokenElse,
	"case":       TokenCase,
	"casex":      TokenCasex,
	"casez":      TokenCasez,
	"endcase":    TokenEndcase,
	"default":    TokenDefault,
	"for":        TokenFor,
	"while":      TokenWhile,
}

// LookupIdent returns the keyword TokenType for ident, or TokenIdent if not a keyword.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TokenIdent
}

// Keywords returns the list of supported Verilog keywords (useful for completion).
func Keywords() []string {
	out := make([]string, 0, len(keywords))
	for k := range keywords {
		out = append(out, k)
	}
	return out
}

// Token represents a lexical token with its source range.
type Token struct {
	Type    TokenType
	Literal string
	Range   ast.Range
}
