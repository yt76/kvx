package lexer

import (
	"kvx/internal/ast"
)

// Lexer tokenizes Verilog source text.
type Lexer struct {
	input  string
	offset int
	line   int
	col    int
}

// New creates a new Lexer for input.
func New(input string) *Lexer {
	return &Lexer{
		input: input,
	}
}

// Tokenize returns all tokens from input up to and including TokenEOF.
func Tokenize(input string) []Token {
	l := New(input)
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
			l.skipLineComment()
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && l.peekNext() == '*' {
			if tok, ok := l.skipBlockComment(); !ok {
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

	// Numeric literals (sized, unsized decimal, or unsized based like 'hFF, 'b0, '0)
	if isDigit(ch) || (ch == '\'' && isBaseOrFillStart(l.peekNext())) {
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
		return l.makeToken(TokenColon, ":", start)
	case ',':
		return l.makeToken(TokenComma, ",", start)
	case '.':
		return l.makeToken(TokenDot, ".", start)
	case '#':
		return l.makeToken(TokenHash, "#", start)
	case '@':
		return l.makeToken(TokenAt, "@", start)
	case '?':
		return l.makeToken(TokenQuestion, "?", start)
	case '+':
		return l.makeToken(TokenPlus, "+", start)
	case '-':
		return l.makeToken(TokenMinus, "-", start)
	case '*':
		return l.makeToken(TokenStar, "*", start)
	case '/':
		return l.makeToken(TokenSlash, "/", start)
	case '%':
		return l.makeToken(TokenPercent, "%", start)
	case '^':
		return l.makeToken(TokenCaret, "^", start)
	case '~':
		return l.makeToken(TokenTilde, "~", start)
	case '=':
		if l.peek() == '=' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				return l.makeToken(TokenCaseEq, "===", start)
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
			return l.makeToken(TokenNeq, "!=", start)
		}
		return l.makeToken(TokenBang, "!", start)
	case '<':
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
			return l.makeToken(TokenShr, ">>", start)
		}
		return l.makeToken(TokenGt, ">", start)
	case '&':
		if l.peek() == '&' {
			l.advance()
			return l.makeToken(TokenLand, "&&", start)
		}
		return l.makeToken(TokenAmp, "&", start)
	case '|':
		if l.peek() == '|' {
			l.advance()
			return l.makeToken(TokenLor, "||", start)
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
	if l.offset+1 >= len(l.input) {
		return 0
	}
	return l.input[l.offset+1]
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

func (l *Lexer) skipLineComment() {
	for l.offset < len(l.input) && l.peek() != '\n' {
		l.advance()
	}
}

func (l *Lexer) skipBlockComment() (Token, bool) {
	start := l.pos()
	l.advance() // '/'
	l.advance() // '*'
	for l.offset < len(l.input) {
		if l.peek() == '*' && l.peekNext() == '/' {
			l.advance()
			l.advance()
			return Token{}, true
		}
		l.advance()
	}
	return Token{
		Type:    TokenIllegal,
		Literal: "unterminated block comment",
		Range:   ast.Range{Start: start, End: l.pos()},
	}, false
}

func (l *Lexer) scanDirectiveOrMacro() (Token, bool) {
	start := l.pos()
	l.advance() // '`'
	name := l.readIdent()

	switch name {
	case "timescale", "define", "include", "ifdef", "ifndef", "elsif", "undef", "default_nettype", "pragma", "line":
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
	case "else", "endif", "resetall", "celldefine", "endcelldefine":
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

	// Read leading decimal digits (size or decimal number).
	for l.offset < len(l.input) && (isDigit(l.peek()) || l.peek() == '_') {
		l.advance()
	}

	// Skip optional spaces before base specifier if followed by ' (e.g. 8 'hFF is allowed, though 8'hFF is standard).
	if l.peek() == '\'' {
		l.advance() // '\''
		// Optional signed specifier 's' or 'S'
		if l.peek() == 's' || l.peek() == 'S' {
			l.advance()
		}
		// Base specifier: b/B, o/O, d/D, h/H
		if isBaseChar(l.peek()) {
			l.advance()
		}
		// Skip spaces between base and digits if any (Wait: only horizontal whitespace if followed by value digits).
		for l.offset < len(l.input) && isNumberValueChar(l.peek()) {
			l.advance()
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

func isBaseOrFillStart(ch byte) bool {
	if isBaseChar(ch) || ch == 's' || ch == 'S' {
		return true
	}
	switch ch {
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
