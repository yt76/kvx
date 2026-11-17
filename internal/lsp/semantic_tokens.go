package lsp

import (
	"strings"

	"kvx/internal/analyzer"
	"kvx/internal/lexer"
)

// Indices into defaultSemanticTokensLegend.TokenTypes.
const (
	semTokenKeyword uint32 = iota
	semTokenType
	semTokenVariable
	semTokenProperty
	semTokenParameter
	semTokenNumber
	semTokenString
	semTokenFunction
	semTokenMacro
	semTokenOperator
)

// Bitmask flags corresponding to defaultSemanticTokensLegend.TokenModifiers.
const (
	semModDeclaration uint32 = 1 << iota
	semModReadonly
)

var defaultSemanticTokensLegend = SemanticTokensLegend{
	TokenTypes: []string{
		"keyword",
		"type",
		"variable",
		"property",
		"parameter",
		"number",
		"string",
		"function",
		"macro",
		"operator",
	},
	TokenModifiers: []string{
		"declaration",
		"readonly",
	},
}

func computeSemanticTokens(doc *Document) SemanticTokens {
	if doc == nil {
		return SemanticTokens{Data: []uint32{}}
	}

	tokens := lexer.TokenizeWithDirectives(doc.Text)
	data := make([]uint32, 0, len(tokens)*5)

	prevLine := 0
	prevCol := 0
	var prevTok lexer.Token

	for _, tok := range tokens {
		if tok.Type == lexer.TokenEOF || tok.Type == lexer.TokenIllegal {
			continue
		}
		if tok.Range.Start.Line != tok.Range.End.Line {
			prevTok = tok
			continue
		}

		length := tok.Range.End.Column - tok.Range.Start.Column
		if length <= 0 {
			prevTok = tok
			continue
		}

		tokType, tokMods, ok := classifySemanticToken(tok, prevTok, doc.Analysis)
		prevTok = tok
		if !ok {
			continue
		}

		line := tok.Range.Start.Line
		col := tok.Range.Start.Column

		deltaLine := line - prevLine
		deltaStart := col
		if deltaLine == 0 {
			deltaStart = col - prevCol
		}

		data = append(data,
			uint32(deltaLine),
			uint32(deltaStart),
			uint32(length),
			tokType,
			tokMods,
		)

		prevLine = line
		prevCol = col
	}

	return SemanticTokens{Data: data}
}

func classifySemanticToken(tok, prevTok lexer.Token, an *analyzer.Analysis) (uint32, uint32, bool) {
	if isKeywordToken(tok.Type) {
		return semTokenKeyword, 0, true
	}

	switch tok.Type {
	case lexer.TokenDirective:
		return semTokenMacro, 0, true
	case lexer.TokenNumber:
		return semTokenNumber, 0, true
	case lexer.TokenString:
		return semTokenString, 0, true
	case lexer.TokenSystemIdent:
		return semTokenFunction, 0, true
	case lexer.TokenIdent:
		if strings.HasPrefix(tok.Literal, "`") {
			return semTokenMacro, 0, true
		}
		if prevTok.Type == lexer.TokenDirective {
			switch prevTok.Literal {
			case "`define":
				return semTokenMacro, semModDeclaration, true
			case "`ifdef", "`ifndef", "`elsif", "`undef":
				return semTokenMacro, 0, true
			}
		}
		if prevTok.Type == lexer.TokenDot {
			return semTokenProperty, 0, true
		}
		if an != nil {
			if sym := an.SymbolAt(tok.Range.Start); sym != nil {
				var mods uint32
				if sym.SelectionRange.Start.Line == tok.Range.Start.Line &&
					sym.SelectionRange.Start.Column == tok.Range.Start.Column {
					mods |= semModDeclaration
				}
				switch sym.Kind {
				case analyzer.KindModule:
					return semTokenType, mods, true
				case analyzer.KindParam:
					return semTokenParameter, mods | semModReadonly, true
				case analyzer.KindPort:
					if strings.HasPrefix(sym.TypeDetail, "input") {
						mods |= semModReadonly
					}
					return semTokenProperty, mods, true
				case analyzer.KindNet, analyzer.KindInstance:
					return semTokenVariable, mods, true
				}
			}
		}
		return semTokenVariable, 0, true
	default:
		if isOperatorToken(tok.Type) {
			return semTokenOperator, 0, true
		}
		return 0, 0, false
	}
}

func isKeywordToken(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenModule, lexer.TokenEndmodule,
		lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout,
		lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger,
		lexer.TokenParameter, lexer.TokenLocalparam, lexer.TokenSigned,
		lexer.TokenAssign, lexer.TokenAlways, lexer.TokenInitial,
		lexer.TokenPosedge, lexer.TokenNegedge, lexer.TokenOr,
		lexer.TokenBegin, lexer.TokenEnd,
		lexer.TokenIf, lexer.TokenElse,
		lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez, lexer.TokenEndcase, lexer.TokenDefault,
		lexer.TokenFor, lexer.TokenWhile:
		return true
	default:
		return false
	}
}

func isOperatorToken(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenAssignOp, lexer.TokenLeq, lexer.TokenGeq,
		lexer.TokenEq, lexer.TokenNeq, lexer.TokenCaseEq, lexer.TokenCaseNeq,
		lexer.TokenLt, lexer.TokenGt,
		lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenAmp, lexer.TokenPipe, lexer.TokenCaret, lexer.TokenTilde, lexer.TokenBang,
		lexer.TokenLand, lexer.TokenLor,
		lexer.TokenShl, lexer.TokenShr, lexer.TokenAshl, lexer.TokenAshr,
		lexer.TokenQuestion, lexer.TokenAt, lexer.TokenHash:
		return true
	default:
		return false
	}
}
