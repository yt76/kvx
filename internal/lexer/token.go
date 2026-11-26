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
	TokenPackage      TokenType = "package"
	TokenEndpackage   TokenType = "endpackage"
	TokenImport       TokenType = "import"
	TokenExport       TokenType = "export"
	TokenInterface    TokenType = "interface"
	TokenEndinterface TokenType = "endinterface"
	TokenModport      TokenType = "modport"
	TokenProgram      TokenType = "program"
	TokenEndprogram   TokenType = "endprogram"
	TokenChecker      TokenType = "checker"
	TokenEndchecker   TokenType = "endchecker"
	TokenLet          TokenType = "let"
	TokenUntyped      TokenType = "untyped"
	TokenClocking     TokenType = "clocking"
	TokenEndclocking  TokenType = "endclocking"
	TokenGlobal       TokenType = "global"
	TokenBind         TokenType = "bind"
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
	TokenRef          TokenType = "ref"
	TokenWire         TokenType = "wire"
	TokenUwire        TokenType = "uwire"
	TokenReg          TokenType = "reg"
	TokenLogic        TokenType = "logic"
	TokenBit          TokenType = "bit"
	TokenByte         TokenType = "byte"
	TokenShortint     TokenType = "shortint"
	TokenInt          TokenType = "int"
	TokenLongint      TokenType = "longint"
	TokenStringKw     TokenType = "string"
	TokenChandle      TokenType = "chandle"
	TokenShortreal    TokenType = "shortreal"
	TokenContext      TokenType = "context"
	TokenVoid         TokenType = "void"
	TokenConst        TokenType = "const"
	TokenVar          TokenType = "var"
	TokenStatic       TokenType = "static"
	TokenTypedef      TokenType = "typedef"
	TokenEnum         TokenType = "enum"
	TokenStruct       TokenType = "struct"
	TokenUnion        TokenType = "union"
	TokenPacked       TokenType = "packed"
	TokenTypeKw       TokenType = "type"
	TokenClass        TokenType = "class"
	TokenEndclass     TokenType = "endclass"
	TokenExtends      TokenType = "extends"
	TokenVirtual      TokenType = "virtual"
	TokenPure         TokenType = "pure"
	TokenExtern       TokenType = "extern"
	TokenLocal        TokenType = "local"
	TokenProtected    TokenType = "protected"
	TokenSuper        TokenType = "super"
	TokenThis         TokenType = "this"
	TokenNew          TokenType = "new"
	TokenNull         TokenType = "null"
	TokenRand         TokenType = "rand"
	TokenRandc        TokenType = "randc"
	TokenConstraint   TokenType = "constraint"
	TokenSolve        TokenType = "solve"
	TokenBefore       TokenType = "before"
	TokenDist         TokenType = "dist"
	TokenSoft         TokenType = "soft"
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
	TokenUnsigned     TokenType = "unsigned"
	TokenAssign       TokenType = "assign"
	TokenDeassign     TokenType = "deassign"
	TokenForce        TokenType = "force"
	TokenRelease      TokenType = "release"
	TokenAlways       TokenType = "always"
	TokenAlwaysComb   TokenType = "always_comb"
	TokenAlwaysFF     TokenType = "always_ff"
	TokenAlwaysLatch  TokenType = "always_latch"
	TokenInitial      TokenType = "initial"
	TokenFinal        TokenType = "final"
	TokenAssert       TokenType = "assert"
	TokenAssume       TokenType = "assume"
	TokenCover        TokenType = "cover"
	TokenRestrict     TokenType = "restrict"
	TokenProperty     TokenType = "property"
	TokenEndproperty  TokenType = "endproperty"
	TokenSequence     TokenType = "sequence"
	TokenEndsequence  TokenType = "endsequence"
	TokenIff          TokenType = "iff"
	TokenCovergroup   TokenType = "covergroup"
	TokenEndgroup     TokenType = "endgroup"
	TokenCoverpoint   TokenType = "coverpoint"
	TokenCross        TokenType = "cross"
	TokenBins         TokenType = "bins"
	TokenIgnoreBins   TokenType = "ignore_bins"
	TokenIllegalBins  TokenType = "illegal_bins"
	TokenWildcard     TokenType = "wildcard"
	TokenIntersect    TokenType = "intersect"
	TokenBinsof       TokenType = "binsof"
	TokenWith         TokenType = "with"
	TokenPosedge    TokenType = "posedge"
	TokenNegedge    TokenType = "negedge"
	TokenOr         TokenType = "or"
	TokenBegin      TokenType = "begin"
	TokenEnd        TokenType = "end"
	TokenIf           TokenType = "if"
	TokenElse         TokenType = "else"
	TokenUnique       TokenType = "unique"
	TokenUnique0      TokenType = "unique0"
	TokenPriority     TokenType = "priority"
	TokenCase         TokenType = "case"
	TokenCasex        TokenType = "casex"
	TokenCasez        TokenType = "casez"
	TokenEndcase      TokenType = "endcase"
	TokenDefault      TokenType = "default"
	TokenFor          TokenType = "for"
	TokenForeach      TokenType = "foreach"
	TokenInside       TokenType = "inside"
	TokenWhile        TokenType = "while"
	TokenDo           TokenType = "do"
	TokenRepeat       TokenType = "repeat"
	TokenForever      TokenType = "forever"
	TokenWait         TokenType = "wait"
	TokenBreak        TokenType = "break"
	TokenContinue     TokenType = "continue"
	TokenReturn       TokenType = "return"
	TokenFork        TokenType = "fork"
	TokenJoin        TokenType = "join"
	TokenJoinAny     TokenType = "join_any"
	TokenJoinNone    TokenType = "join_none"
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
	TokenSemicolon  TokenType = ";"
	TokenColon      TokenType = ":"
	TokenColonColon TokenType = "::"
	TokenColonEq    TokenType = ":="
	TokenColonSlash TokenType = ":/"
	TokenComma      TokenType = ","
	TokenDot       TokenType = "."
	TokenHash      TokenType = "#"
	TokenAt        TokenType = "@"
	TokenQuestion  TokenType = "?"
	TokenTick      TokenType = "'"
	TokenDollar    TokenType = "$"

	// Operators
	TokenAssignOp TokenType = "="
	TokenLeq      TokenType = "<="
	TokenGeq      TokenType = ">="
	TokenEq       TokenType = "=="
	TokenNeq      TokenType = "!="
	TokenCaseEq   TokenType = "==="
	TokenCaseNeq  TokenType = "!=="
	TokenWildEq   TokenType = "==?"
	TokenWildNeq  TokenType = "!=?"
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
	TokenAshl       TokenType = "<<<"
	TokenAshr       TokenType = ">>>"
	TokenArrow      TokenType = "->"
	TokenFatArrow   TokenType = "=>"
	TokenEquiv      TokenType = "<->"
	TokenImpliesOverlapping    TokenType = "|->"
	TokenImpliesNonOverlapping TokenType = "|=>"
	TokenHashHash   TokenType = "##"
	TokenPower      TokenType = "**"
	TokenXnor       TokenType = "~^"
	TokenPaste      TokenType = "``"
	TokenPlusPlus   TokenType = "++"
	TokenMinusMinus TokenType = "--"
	TokenPlusEq     TokenType = "+="
	TokenMinusEq    TokenType = "-="
	TokenMulEq      TokenType = "*="
	TokenDivEq      TokenType = "/="
	TokenModEq      TokenType = "%="
	TokenAndEq      TokenType = "&="
	TokenOrEq       TokenType = "|="
	TokenXorEq      TokenType = "^="
	TokenShlEq      TokenType = "<<="
	TokenShrEq      TokenType = ">>="
)

var keywords = map[string]TokenType{
	"module":       TokenModule,
	"macromodule":  TokenMacromodule,
	"endmodule":    TokenEndmodule,
	"package":      TokenPackage,
	"endpackage":   TokenEndpackage,
	"import":       TokenImport,
	"export":       TokenExport,
	"interface":    TokenInterface,
	"endinterface": TokenEndinterface,
	"modport":      TokenModport,
	"program":      TokenProgram,
	"endprogram":   TokenEndprogram,
	"checker":      TokenChecker,
	"endchecker":   TokenEndchecker,
	"let":          TokenLet,
	"untyped":      TokenUntyped,
	"clocking":     TokenClocking,
	"endclocking":  TokenEndclocking,
	"global":       TokenGlobal,
	"bind":         TokenBind,
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
	"ref":          TokenRef,
	"wire":         TokenWire,
	"uwire":        TokenUwire,
	"reg":          TokenReg,
	"logic":        TokenLogic,
	"bit":          TokenBit,
	"byte":         TokenByte,
	"shortint":     TokenShortint,
	"int":          TokenInt,
	"longint":      TokenLongint,
	"string":       TokenStringKw,
	"chandle":      TokenChandle,
	"shortreal":    TokenShortreal,
	"context":      TokenContext,
	"void":         TokenVoid,
	"const":        TokenConst,
	"var":          TokenVar,
	"static":       TokenStatic,
	"typedef":      TokenTypedef,
	"enum":         TokenEnum,
	"struct":       TokenStruct,
	"union":        TokenUnion,
	"packed":       TokenPacked,
	"type":         TokenTypeKw,
	"class":        TokenClass,
	"endclass":     TokenEndclass,
	"extends":      TokenExtends,
	"virtual":      TokenVirtual,
	"pure":         TokenPure,
	"extern":       TokenExtern,
	"local":        TokenLocal,
	"protected":    TokenProtected,
	"super":        TokenSuper,
	"this":         TokenThis,
	"new":          TokenNew,
	"null":         TokenNull,
	"rand":         TokenRand,
	"randc":        TokenRandc,
	"constraint":   TokenConstraint,
	"solve":        TokenSolve,
	"before":       TokenBefore,
	"dist":         TokenDist,
	"soft":         TokenSoft,
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
	"unsigned":     TokenUnsigned,
	"assign":       TokenAssign,
	"deassign":     TokenDeassign,
	"force":        TokenForce,
	"release":      TokenRelease,
	"always":       TokenAlways,
	"always_comb":  TokenAlwaysComb,
	"always_ff":    TokenAlwaysFF,
	"always_latch": TokenAlwaysLatch,
	"initial":      TokenInitial,
	"final":        TokenFinal,
	"assert":       TokenAssert,
	"assume":       TokenAssume,
	"cover":        TokenCover,
	"restrict":     TokenRestrict,
	"property":     TokenProperty,
	"endproperty":  TokenEndproperty,
	"sequence":     TokenSequence,
	"endsequence":  TokenEndsequence,
	"iff":          TokenIff,
	"covergroup":   TokenCovergroup,
	"endgroup":     TokenEndgroup,
	"coverpoint":   TokenCoverpoint,
	"cross":        TokenCross,
	"bins":         TokenBins,
	"ignore_bins":  TokenIgnoreBins,
	"illegal_bins": TokenIllegalBins,
	"wildcard":     TokenWildcard,
	"intersect":    TokenIntersect,
	"binsof":       TokenBinsof,
	"with":         TokenWith,
	"posedge":    TokenPosedge,
	"negedge":    TokenNegedge,
	"or":         TokenOr,
	"begin":      TokenBegin,
	"end":        TokenEnd,
	"if":           TokenIf,
	"else":         TokenElse,
	"unique":       TokenUnique,
	"unique0":      TokenUnique0,
	"priority":     TokenPriority,
	"case":         TokenCase,
	"casex":        TokenCasex,
	"casez":        TokenCasez,
	"endcase":      TokenEndcase,
	"default":      TokenDefault,
	"for":          TokenFor,
	"foreach":      TokenForeach,
	"inside":       TokenInside,
	"while":        TokenWhile,
	"do":           TokenDo,
	"repeat":       TokenRepeat,
	"forever":      TokenForever,
	"wait":         TokenWait,
	"break":        TokenBreak,
	"continue":     TokenContinue,
	"return":       TokenReturn,
	"fork":        TokenFork,
	"join":        TokenJoin,
	"join_any":    TokenJoinAny,
	"join_none":   TokenJoinNone,
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
