package lexer

import "kvx/internal/ast"

// TokenType identifies the lexical category of a Token.
type TokenType string

const (
	TokenEOF     TokenType = "EOF"
	TokenIllegal TokenType = "ILLEGAL"

	// Identifiers, literals, and trivia
	TokenIdent       TokenType = "IDENT"
	TokenSystemIdent TokenType = "SYSTEM_IDENT" // e.g. $display, $clog2
	TokenDirective   TokenType = "DIRECTIVE"    // e.g. `include, `define, `ifdef
	TokenComment     TokenType = "COMMENT"      // e.g. // ... or /* ... */
	TokenNumber      TokenType = "NUMBER"       // e.g. 42, 8'hFF, 1'b0
	TokenString      TokenType = "STRING"       // e.g. "hello"

	// Keywords
	TokenModule       TokenType = "module"
	TokenMacromodule  TokenType = "macromodule"
	TokenEndmodule    TokenType = "endmodule"
	TokenPrimitive    TokenType = "primitive"
	TokenEndprimitive TokenType = "endprimitive"
	TokenTable        TokenType = "table"
	TokenEndtable     TokenType = "endtable"
	TokenSpecify      TokenType = "specify"
	TokenEndspecify   TokenType = "endspecify"
	TokenConfig       TokenType = "config"
	TokenEndconfig    TokenType = "endconfig"
	TokenInput        TokenType = "input"
	TokenOutput       TokenType = "output"
	TokenInout        TokenType = "inout"
	TokenWire         TokenType = "wire"
	TokenUwire        TokenType = "uwire"
	TokenReg          TokenType = "reg"
	TokenInteger      TokenType = "integer"
	TokenTri          TokenType = "tri"
	TokenTri0         TokenType = "tri0"
	TokenTri1         TokenType = "tri1"
	TokenTriand       TokenType = "triand"
	TokenTrior        TokenType = "trior"
	TokenTrireg       TokenType = "trireg"
	TokenSupply0      TokenType = "supply0"
	TokenSupply1      TokenType = "supply1"
	TokenWand         TokenType = "wand"
	TokenWor          TokenType = "wor"
	TokenScalared     TokenType = "scalared"
	TokenVectored     TokenType = "vectored"
	TokenReal         TokenType = "real"
	TokenTime         TokenType = "time"
	TokenRealtime     TokenType = "realtime"
	TokenEvent        TokenType = "event"
	TokenParameter    TokenType = "parameter"
	TokenLocalparam   TokenType = "localparam"
	TokenDefparam     TokenType = "defparam"
	TokenSigned       TokenType = "signed"
	TokenAssign       TokenType = "assign"
	TokenDeassign     TokenType = "deassign"
	TokenForce        TokenType = "force"
	TokenRelease      TokenType = "release"
	TokenAlways       TokenType = "always"
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
	TokenFor         TokenType = "for"
	TokenWhile       TokenType = "while"
	TokenRepeat      TokenType = "repeat"
	TokenForever     TokenType = "forever"
	TokenWait        TokenType = "wait"
	TokenFork        TokenType = "fork"
	TokenJoin        TokenType = "join"
	TokenDisable     TokenType = "disable"
	TokenFunction    TokenType = "function"
	TokenEndfunction TokenType = "endfunction"
	TokenTask        TokenType = "task"
	TokenEndtask     TokenType = "endtask"
	TokenAutomatic   TokenType = "automatic"
	TokenGenerate    TokenType = "generate"
	TokenEndgenerate TokenType = "endgenerate"
	TokenGenvar      TokenType = "genvar"

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
	TokenArrow    TokenType = "->"
	TokenPower    TokenType = "**"
	TokenXnor     TokenType = "~^"
	TokenPaste    TokenType = "``"
)

var keywords = map[string]TokenType{
	"module":       TokenModule,
	"macromodule":  TokenMacromodule,
	"endmodule":    TokenEndmodule,
	"primitive":    TokenPrimitive,
	"endprimitive": TokenEndprimitive,
	"table":        TokenTable,
	"endtable":     TokenEndtable,
	"specify":      TokenSpecify,
	"endspecify":   TokenEndspecify,
	"config":       TokenConfig,
	"endconfig":    TokenEndconfig,
	"input":        TokenInput,
	"output":       TokenOutput,
	"inout":        TokenInout,
	"wire":         TokenWire,
	"uwire":        TokenUwire,
	"reg":          TokenReg,
	"integer":      TokenInteger,
	"tri":          TokenTri,
	"tri0":         TokenTri0,
	"tri1":         TokenTri1,
	"triand":       TokenTriand,
	"trior":        TokenTrior,
	"trireg":       TokenTrireg,
	"supply0":      TokenSupply0,
	"supply1":      TokenSupply1,
	"wand":         TokenWand,
	"wor":          TokenWor,
	"scalared":     TokenScalared,
	"vectored":     TokenVectored,
	"real":         TokenReal,
	"time":         TokenTime,
	"realtime":     TokenRealtime,
	"event":        TokenEvent,
	"parameter":    TokenParameter,
	"localparam":   TokenLocalparam,
	"defparam":     TokenDefparam,
	"signed":       TokenSigned,
	"assign":       TokenAssign,
	"deassign":     TokenDeassign,
	"force":        TokenForce,
	"release":      TokenRelease,
	"always":       TokenAlways,
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
	"for":         TokenFor,
	"while":       TokenWhile,
	"repeat":      TokenRepeat,
	"forever":     TokenForever,
	"wait":        TokenWait,
	"fork":        TokenFork,
	"join":        TokenJoin,
	"disable":     TokenDisable,
	"function":    TokenFunction,
	"endfunction": TokenEndfunction,
	"task":        TokenTask,
	"endtask":     TokenEndtask,
	"automatic":   TokenAutomatic,
	"generate":    TokenGenerate,
	"endgenerate": TokenEndgenerate,
	"genvar":      TokenGenvar,
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
