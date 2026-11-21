package lexer

import (
	"kvx/internal/ast"
)

// Lexer tokenizes Verilog source text.
type Lexer struct {
	input          string
	offset         int
	line           int
	col            int
	emitDirectives bool
	emitComments   bool
}

// New creates a new Lexer for input.
func New(input string) *Lexer {
	return &Lexer{
		input: input,
	}
}

// Tokenize returns all tokens from input up to and including TokenEOF, skipping compiler directives and comments.
func Tokenize(input string) []Token {
	return tokenizeWith(New(input))
}

// TokenizeWithDirectives returns all tokens from input including compiler directives as TokenDirective.
func TokenizeWithDirectives(input string) []Token {
	l := New(input)
	l.emitDirectives = true
	return tokenizeWith(l)
}

// TokenizeWithTrivia returns all tokens from input including compiler directives and comments.
func TokenizeWithTrivia(input string) []Token {
	l := New(input)
	l.emitDirectives = true
	l.emitComments = true
	return tokenizeWith(l)
}

func tokenizeWith(l *Lexer) []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens
}

// NextToken scans and returns the next lexical token.
func (l *Lexer) NextToken() Token {
	for {
		l.skipWhitespace()

		if l.offset >= len(l.input) {
			pos := l.pos()
			return Token{Type: TokenEOF, Literal: "", Range: ast.Range{Start: pos, End: pos}}
		}

		ch := l.peek()

		// Single-line comment: //
		if ch == '/' && l.peekNext() == '/' {
			if tok, emitted := l.scanLineComment(); emitted {
				return tok
			}
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && l.peekNext() == '*' {
			if tok, emitted := l.scanBlockComment(); emitted {
				return tok
			}
			continue
		}

		// Compiler directive or macro usage: `...
		if ch == '`' {
			if tok, emitted := l.scanDirectiveOrMacro(); emitted {
				return tok
			}
			continue
		}

		break
	}

	start := l.pos()
	ch := l.peek()

	// Identifiers and keywords
	if isIdentStart(ch) {
		lit := l.readIdent()
		return Token{
			Type:    LookupIdent(lit),
			Literal: lit,
			Range:   ast.Range{Start: start, End: l.pos()},
		}
	}

	// Escaped identifier: \<non-whitespace><whitespace>
	if ch == '\\' {
		lit := l.readEscapedIdent()
		return Token{
			Type:    TokenIdent,
			Literal: lit,
			Range:   ast.Range{Start: start, End: l.pos()},
		}
	}

	// System task/function identifier: $display, $clog2, etc.
	if ch == '$' && isIdentPart(l.peekNext()) {
		l.advance()
		lit := "$" + l.readIdent()
		return Token{
			Type:    TokenSystemIdent,
			Literal: lit,
			Range:   ast.Range{Start: start, End: l.pos()},
		}
	}

	// Numeric literals (sized, unsized decimal, real/scientific, or unsized based like 'hFF, 'b0, '0)
	if isDigit(ch) || (ch == '\'' && l.isUnsizedLiteralStart()) {
		lit := l.readNumber()
		return Token{
			Type:    TokenNumber,
			Literal: lit,
			Range:   ast.Range{Start: start, End: l.pos()},
		}
	}

	// String literal
	if ch == '"' {
		return l.readString(start)
	}

	// Multi-character operators and single-character punctuation
	l.advance()
	switch ch {
	case '(':
		return l.makeToken(TokenLParen, "(", start)
	case ')':
		return l.makeToken(TokenRParen, ")", start)
	case '[':
		return l.makeToken(TokenLBracket, "[", start)
	case ']':
		return l.makeToken(TokenRBracket, "]", start)
	case '{':
		return l.makeToken(TokenLBrace, "{", start)
	case '}':
		return l.makeToken(TokenRBrace, "}", start)
	case ';':
		return l.makeToken(TokenSemicolon, ";", start)
	case ':':
		if l.peek() == ':' {
			l.advance()
			return l.makeToken(TokenColonColon, "::", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenColonEq, ":=", start)
		}
		if l.peek() == '/' {
			l.advance()
			return l.makeToken(TokenColonSlash, ":/", start)
		}
		return l.makeToken(TokenColon, ":", start)
	case ',':
		return l.makeToken(TokenComma, ",", start)
	case '.':
		return l.makeToken(TokenDot, ".", start)
	case '#':
		if l.peek() == '#' {
			l.advance()
			return l.makeToken(TokenHashHash, "##", start)
		}
		if (l.peek() == '-' || l.peek() == '=') && l.peekNext() == '#' {
			op := "#" + string(l.advance()) + string(l.advance())
			return l.makeToken(TokenHashHash, op, start)
		}
		return l.makeToken(TokenHash, "#", start)
	case '@':
		return l.makeToken(TokenAt, "@", start)
	case '?':
		return l.makeToken(TokenQuestion, "?", start)
	case '\'':
		return l.makeToken(TokenTick, "'", start)
	case '$':
		return l.makeToken(TokenDollar, "$", start)
	case '+':
		if l.peek() == '+' {
			l.advance()
			return l.makeToken(TokenPlusPlus, "++", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenPlusEq, "+=", start)
		}
		return l.makeToken(TokenPlus, "+", start)
	case '-':
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TokenArrow, "->", start)
		}
		if l.peek() == '-' {
			l.advance()
			return l.makeToken(TokenMinusMinus, "--", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenMinusEq, "-=", start)
		}
		return l.makeToken(TokenMinus, "-", start)
	case '*':
		if l.peek() == '*' {
			l.advance()
			return l.makeToken(TokenPower, "**", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenMulEq, "*=", start)
		}
		return l.makeToken(TokenStar, "*", start)
	case '/':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenDivEq, "/=", start)
		}
		return l.makeToken(TokenSlash, "/", start)
	case '%':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenModEq, "%=", start)
		}
		return l.makeToken(TokenPercent, "%", start)
	case '^':
		if l.peek() == '~' {
			l.advance()
			return l.makeToken(TokenXnor, "^~", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenXorEq, "^=", start)
		}
		return l.makeToken(TokenCaret, "^", start)
	case '~':
		if l.peek() == '^' {
			l.advance()
			return l.makeToken(TokenXnor, "~^", start)
		}
		return l.makeToken(TokenTilde, "~", start)
	case '=':
		if l.peek() == '=' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				return l.makeToken(TokenCaseEq, "===", start)
			}
			if l.peek() == '?' {
				l.advance()
				return l.makeToken(TokenWildEq, "==?", start)
			}
			return l.makeToken(TokenEq, "==", start)
		}
		return l.makeToken(TokenAssignOp, "=", start)
	case '!':
		if l.peek() == '=' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				return l.makeToken(TokenCaseNeq, "!==", start)
			}
			if l.peek() == '?' {
				l.advance()
				return l.makeToken(TokenWildNeq, "!=?", start)
			}
			return l.makeToken(TokenNeq, "!=", start)
		}
		return l.makeToken(TokenBang, "!", start)
	case '<':
		if l.peek() == '-' && l.peekNext() == '>' {
			l.advance()
			l.advance()
			return l.makeToken(TokenEquiv, "<->", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenLeq, "<=", start)
		}
		if l.peek() == '<' {
			l.advance()
			if l.peek() == '<' {
				l.advance()
				return l.makeToken(TokenAshl, "<<<", start)
			}
			if l.peek() == '=' {
				l.advance()
				return l.makeToken(TokenShlEq, "<<=", start)
			}
			return l.makeToken(TokenShl, "<<", start)
		}
		return l.makeToken(TokenLt, "<", start)
	case '>':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenGeq, ">=", start)
		}
		if l.peek() == '>' {
			l.advance()
			if l.peek() == '>' {
				l.advance()
				return l.makeToken(TokenAshr, ">>>", start)
			}
			if l.peek() == '=' {
				l.advance()
				return l.makeToken(TokenShrEq, ">>=", start)
			}
			return l.makeToken(TokenShr, ">>", start)
		}
		return l.makeToken(TokenGt, ">", start)
	case '&':
		if l.peek() == '&' {
			l.advance()
			return l.makeToken(TokenLand, "&&", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenAndEq, "&=", start)
		}
		return l.makeToken(TokenAmp, "&", start)
	case '|':
		if l.peek() == '-' && l.peekNext() == '>' {
			l.advance()
			l.advance()
			return l.makeToken(TokenImpliesOverlapping, "|->", start)
		}
		if l.peek() == '=' && l.peekNext() == '>' {
			l.advance()
			l.advance()
			return l.makeToken(TokenImpliesNonOverlapping, "|=>", start)
		}
		if l.peek() == '|' {
			l.advance()
			return l.makeToken(TokenLor, "||", start)
		}
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TokenOrEq, "|=", start)
		}
		return l.makeToken(TokenPipe, "|", start)
	default:
		return l.makeToken(TokenIllegal, string(ch), start)
	}
}

func (l *Lexer) makeToken(typ TokenType, lit string, start ast.Position) Token {
	return Token{
		Type:    typ,
		Literal: lit,
		Range:   ast.Range{Start: start, End: l.pos()},
	}
}

func (l *Lexer) pos() ast.Position {
	return ast.Position{
		Line:   l.line,
		Column: l.col,
		Offset: l.offset,
	}
}

func (l *Lexer) peek() byte {
	if l.offset >= len(l.input) {
		return 0
	}
	return l.input[l.offset]
}

func (l *Lexer) peekNext() byte {
	return l.peekAt(1)
}

func (l *Lexer) peekAt(n int) byte {
	idx := l.offset + n
	if idx < 0 || idx >= len(l.input) {
		return 0
	}
	return l.input[idx]
}

func (l *Lexer) advance() byte {
	if l.offset >= len(l.input) {
		return 0
	}
	ch := l.input[l.offset]
	l.offset++
	if ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespace() {
	for l.offset < len(l.input) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) scanLineComment() (Token, bool) {
	start := l.pos()
	startOffset := l.offset
	for l.offset < len(l.input) && l.peek() != '\n' && l.peek() != '\r' {
		l.advance()
	}
	if l.emitComments {
		return Token{
			Type:    TokenComment,
			Literal: l.input[startOffset:l.offset],
			Range:   ast.Range{Start: start, End: l.pos()},
		}, true
	}
	return Token{}, false
}

func (l *Lexer) scanBlockComment() (Token, bool) {
	start := l.pos()
	startOffset := l.offset
	l.advance() // '/'
	l.advance() // '*'
	for l.offset < len(l.input) {
		if l.peek() == '*' && l.peekNext() == '/' {
			l.advance()
			l.advance()
			if l.emitComments {
				return Token{
					Type:    TokenComment,
					Literal: l.input[startOffset:l.offset],
					Range:   ast.Range{Start: start, End: l.pos()},
				}, true
			}
			return Token{}, false
		}
		l.advance()
	}
	return Token{
		Type:    TokenIllegal,
		Literal: "unterminated block comment",
		Range:   ast.Range{Start: start, End: l.pos()},
	}, true
}

func (l *Lexer) scanDirectiveOrMacro() (Token, bool) {
	start := l.pos()
	l.advance() // '`'
	if l.peek() == '`' {
		l.advance() // second '`' for macro token pasting ``
		return Token{
			Type:    TokenPaste,
			Literal: "``",
			Range:   ast.Range{Start: start, End: l.pos()},
		}, true
	}
	if l.peek() == '"' {
		return l.readMacroString(start), true
	}
	name := l.readIdent()

	switch name {
	case "timescale", "define", "include", "ifdef", "ifndef", "elsif", "undef", "undefineall",
		"default_nettype", "pragma", "line", "begin_keywords", "end_keywords",
		"unconnected_drive", "nounconnected_drive",
		"else", "endif", "resetall", "celldefine", "endcelldefine":
		if l.emitDirectives {
			return Token{
				Type:    TokenDirective,
				Literal: "`" + name,
				Range:   ast.Range{Start: start, End: l.pos()},
			}, true
		}
		if name == "else" || name == "endif" || name == "resetall" || name == "celldefine" || name == "endcelldefine" ||
			name == "undefineall" || name == "end_keywords" || name == "nounconnected_drive" {
			return Token{}, false
		}
		// Skip remainder of the directive line (respecting backslash continuation).
		for l.offset < len(l.input) {
			if l.peek() == '\\' && (l.peekNext() == '\n' || l.peekNext() == '\r') {
				l.advance()
				if l.peek() == '\r' {
					l.advance()
				}
				if l.peek() == '\n' {
					l.advance()
				}
				continue
			}
			if l.peek() == '\n' {
				break
			}
			l.advance()
		}
		return Token{}, false
	default:
		// Treat macro constant/identifier usage (e.g. `WIDTH) as an identifier token.
		return Token{
			Type:    TokenIdent,
			Literal: "`" + name,
			Range:   ast.Range{Start: start, End: l.pos()},
		}, true
	}
}

func (l *Lexer) readIdent() string {
	startOffset := l.offset
	for l.offset < len(l.input) && isIdentPart(l.peek()) {
		l.advance()
	}
	return l.input[startOffset:l.offset]
}

func (l *Lexer) readEscapedIdent() string {
	startOffset := l.offset
	l.advance() // '\'
	for l.offset < len(l.input) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			break
		}
		l.advance()
	}
	return l.input[startOffset:l.offset]
}

func (l *Lexer) readNumber() string {
	startOffset := l.offset

	// Read leading decimal digits (size or integer part of decimal/real number).
	for l.offset < len(l.input) && (isDigit(l.peek()) || l.peek() == '_') {
		l.advance()
	}

	// Based literal: [size]'[s|S]<base>[spaces]<digits> or '[0|1|x|X|z|Z|?]
	if l.peek() == '\'' {
		l.advance() // '\''
		if (l.peek() == 's' || l.peek() == 'S') && isBaseChar(l.peekNext()) {
			l.advance()
		}
		if isBaseChar(l.peek()) {
			l.advance()
			// IEEE 1364 allows horizontal whitespace between base specifier and value digits.
			wsOffset := 0
			for {
				ch := l.peekAt(wsOffset)
				if ch == ' ' || ch == '\t' {
					wsOffset++
					continue
				}
				break
			}
			if wsOffset > 0 && isNumberValueChar(l.peekAt(wsOffset)) {
				for j := 0; j < wsOffset; j++ {
					l.advance()
				}
			}
		}
		for l.offset < len(l.input) && isNumberValueChar(l.peek()) {
			l.advance()
		}
		return l.input[startOffset:l.offset]
	}

	// Real literal fractional part: '.' followed by at least one digit (e.g. 1.5, 1_000.25)
	if l.peek() == '.' && isDigit(l.peekNext()) {
		l.advance() // '.'
		for l.offset < len(l.input) && (isDigit(l.peek()) || l.peek() == '_') {
			l.advance()
		}
	}

	// Real literal exponent part: [e|E][+|-]?<digits> (e.g. 1e-9, 2.5E+3, 1e6)
	if l.peek() == 'e' || l.peek() == 'E' {
		expOff := 1
		if l.peekAt(expOff) == '+' || l.peekAt(expOff) == '-' {
			expOff++
		}
		if isDigit(l.peekAt(expOff)) {
			l.advance() // 'e' or 'E'
			if l.peek() == '+' || l.peek() == '-' {
				l.advance()
			}
			for l.offset < len(l.input) && (isDigit(l.peek()) || l.peek() == '_') {
				l.advance()
			}
		}
	}

	return l.input[startOffset:l.offset]
}

func (l *Lexer) readString(start ast.Position) Token {
	l.advance() // opening '"'
	startOffset := l.offset
	for l.offset < len(l.input) {
		ch := l.peek()
		if ch == '\\' {
			l.advance()
			if l.offset < len(l.input) {
				l.advance()
			}
			continue
		}
		if ch == '"' {
			lit := l.input[startOffset:l.offset]
			l.advance() // closing '"'
			return Token{
				Type:    TokenString,
				Literal: lit,
				Range:   ast.Range{Start: start, End: l.pos()},
			}
		}
		if ch == '\n' {
			break
		}
		l.advance()
	}
	return Token{
		Type:    TokenIllegal,
		Literal: "unterminated string literal",
		Range:   ast.Range{Start: start, End: l.pos()},
	}
}

// readMacroString scans a macro stringification literal `"...`" (including escaped `\`" or \").
func (l *Lexer) readMacroString(start ast.Position) Token {
	l.advance() // opening '"' after '`'
	startOffset := l.offset
	for l.offset < len(l.input) {
		ch := l.peek()
		if ch == '`' && l.peekNext() == '\\' && l.peekAt(2) == '`' && l.peekAt(3) == '"' {
			l.advance()
			l.advance()
			l.advance()
			l.advance()
			continue
		}
		if ch == '\\' {
			l.advance()
			if l.peek() == '`' && l.peekNext() == '"' {
				l.advance()
				l.advance()
				continue
			}
			if l.offset < len(l.input) {
				l.advance()
			}
			continue
		}
		if ch == '`' && l.peekNext() == '"' {
			lit := l.input[startOffset:l.offset]
			l.advance() // '`'
			l.advance() // '"'
			return Token{
				Type:    TokenString,
				Literal: lit,
				Range:   ast.Range{Start: start, End: l.pos()},
			}
		}
		if ch == '\n' {
			break
		}
		l.advance()
	}
	return Token{
		Type:    TokenIllegal,
		Literal: "unterminated macro string literal",
		Range:   ast.Range{Start: start, End: l.pos()},
	}
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch) || ch == '$'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isBaseChar(ch byte) bool {
	switch ch {
	case 'b', 'B', 'o', 'O', 'd', 'D', 'h', 'H':
		return true
	default:
		return false
	}
}

func (l *Lexer) isUnsizedLiteralStart() bool {
	next := l.peekNext()
	if isBaseChar(next) {
		return true
	}
	if (next == 's' || next == 'S') && isBaseChar(l.peekAt(2)) {
		return true
	}
	switch next {
	case '0', '1', 'x', 'X', 'z', 'Z', '?':
		return true
	default:
		return false
	}
}

func isNumberValueChar(ch byte) bool {
	return isDigit(ch) ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F') ||
		ch == '_' || ch == 'x' || ch == 'X' || ch == 'z' || ch == 'Z' || ch == '?'
}
