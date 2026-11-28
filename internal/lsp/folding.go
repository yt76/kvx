package lsp

import (
	"sort"
	"strings"

	"kvx/internal/lexer"
)

type foldEntry struct {
	closeType lexer.TokenType
	startLine int
	kind      string
}

// computeFoldingRanges calculates LSP FoldingRanges for a Verilog/SystemVerilog source text.
// It operates directly on trivia-preserving tokens so folding continues to work even while
// the user is editing incomplete code.
func computeFoldingRanges(text string) []FoldingRange {
	if text == "" {
		return []FoldingRange{}
	}

	tokens := lexer.TokenizeWithTrivia(text)

	var ranges []FoldingRange
	addRange := func(startLine, endLine int, kind string) {
		if endLine > startLine {
			ranges = append(ranges, FoldingRange{
				StartLine: startLine,
				EndLine:   endLine,
				Kind:      kind,
			})
		}
	}

	var blockStack []foldEntry
	var parenStack []int
	var braceStack []int
	var condStack []int // start lines of active `ifdef / `ifndef / `elsif / `else branches

	// Track consecutive full-line `//` comments and consecutive `include / import lines.
	commentStart, commentEnd := -1, -1
	flushComments := func() {
		if commentStart >= 0 && commentEnd > commentStart {
			addRange(commentStart, commentEnd, FoldingKindComment)
		}
		commentStart, commentEnd = -1, -1
	}

	importStart, importEnd := -1, -1
	flushImports := func() {
		if importStart >= 0 && importEnd > importStart {
			addRange(importStart, importEnd, FoldingKindImports)
		}
		importStart, importEnd = -1, -1
	}

	// Track non-trivia token context on the current statement so we don't push
	// bodyless prototypes (e.g. `extern function`, `import "DPI-C" function`,
	// `typedef class`, `virtual interface`, `assert property`, `disable fork`).
	stmtHasExternOrPure := false
	stmtHasDPI := false
	stmtHasTypedef := false
	stmtHasVirtual := false
	stmtHasDefault := false
	var prevCodeTok lexer.Token
	lastCodeLine := -1

	popBlock := func(closeType lexer.TokenType, closeLine int) {
		for idx := len(blockStack) - 1; idx >= 0; idx-- {
			if blockStack[idx].closeType == closeType {
				entry := blockStack[idx]
				addRange(entry.startLine, closeLine, entry.kind)
				blockStack = blockStack[:idx]
				return
			}
		}
	}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type == lexer.TokenEOF {
			break
		}

		// 1. Comments
		if tok.Type == lexer.TokenComment {
			if strings.HasPrefix(tok.Literal, "/*") {
				flushComments()
				if tok.Range.End.Line > tok.Range.Start.Line {
					addRange(tok.Range.Start.Line, tok.Range.End.Line, FoldingKindComment)
				}
			} else if strings.HasPrefix(tok.Literal, "//") {
				// Only group standalone full-line comments (not trailing comments on code lines).
				if tok.Range.Start.Line != lastCodeLine {
					if commentStart < 0 {
						commentStart = tok.Range.Start.Line
						commentEnd = tok.Range.Start.Line
					} else if tok.Range.Start.Line == commentEnd+1 {
						commentEnd = tok.Range.Start.Line
					} else {
						flushComments()
						commentStart = tok.Range.Start.Line
						commentEnd = tok.Range.Start.Line
					}
				}
			}
			continue
		}

		flushComments()

		// 2. Preprocessor directives
		if tok.Type == lexer.TokenDirective {
			line := tok.Range.Start.Line
			switch tok.Literal {
			case "`include":
				if importStart < 0 {
					importStart = line
					importEnd = line
				} else if line == importEnd+1 {
					importEnd = line
				} else {
					flushImports()
					importStart = line
					importEnd = line
				}
			case "`ifdef", "`ifndef":
				flushImports()
				condStack = append(condStack, line)
			case "`elsif", "`else":
				flushImports()
				if len(condStack) > 0 {
					startLine := condStack[len(condStack)-1]
					if line-1 > startLine {
						addRange(startLine, line-1, FoldingKindRegion)
					}
					condStack[len(condStack)-1] = line
				}
			case "`endif":
				flushImports()
				if len(condStack) > 0 {
					startLine := condStack[len(condStack)-1]
					condStack = condStack[:len(condStack)-1]
					addRange(startLine, line, FoldingKindRegion)
				}
			default:
				flushImports()
			}
			lastCodeLine = tok.Range.End.Line
			continue
		}

		// Track consecutive `import pkg::*;` statements (excluding `import "DPI-C"`).
		if tok.Type == lexer.TokenImport {
			if i+1 < len(tokens) && tokens[i+1].Type == lexer.TokenString {
				flushImports()
				stmtHasDPI = true
			} else {
				line := tok.Range.Start.Line
				if importStart < 0 {
					importStart = line
					importEnd = line
				} else if line == importEnd+1 {
					importEnd = line
				} else {
					flushImports()
					importStart = line
					importEnd = line
				}
			}
			prevCodeTok = tok
			lastCodeLine = tok.Range.End.Line
			continue
		}
		if tok.Range.Start.Line > importEnd {
			flushImports()
		}

		// 3. Delimiters: `(...)` and `{...}`
		switch tok.Type {
		case lexer.TokenLParen:
			parenStack = append(parenStack, tok.Range.Start.Line)
		case lexer.TokenRParen:
			if len(parenStack) > 0 {
				startLine := parenStack[len(parenStack)-1]
				parenStack = parenStack[:len(parenStack)-1]
				addRange(startLine, tok.Range.Start.Line, "")
			}
		case lexer.TokenLBrace:
			braceStack = append(braceStack, tok.Range.Start.Line)
		case lexer.TokenRBrace:
			if len(braceStack) > 0 {
				startLine := braceStack[len(braceStack)-1]
				braceStack = braceStack[:len(braceStack)-1]
				addRange(startLine, tok.Range.Start.Line, "")
			}
		}

		// 4. Keyword-delimited blocks
		inParens := len(parenStack) > 0
		switch tok.Type {
		case lexer.TokenExtern, lexer.TokenPure:
			stmtHasExternOrPure = true
		case lexer.TokenExport:
			if i+1 < len(tokens) && tokens[i+1].Type == lexer.TokenString {
				stmtHasDPI = true
			}
		case lexer.TokenTypedef:
			stmtHasTypedef = true
		case lexer.TokenVirtual:
			stmtHasVirtual = true
		case lexer.TokenDefault:
			stmtHasDefault = true

		case lexer.TokenModule, lexer.TokenMacromodule:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndmodule, startLine: tok.Range.Start.Line})
		case lexer.TokenEndmodule:
			popBlock(lexer.TokenEndmodule, tok.Range.Start.Line)

		case lexer.TokenInterface:
			// Exclude `virtual interface bus_if vif;` and generic interface port `(interface bus)`.
			if !stmtHasVirtual && !inParens {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndinterface, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndinterface:
			popBlock(lexer.TokenEndinterface, tok.Range.Start.Line)

		case lexer.TokenPackage:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndpackage, startLine: tok.Range.Start.Line})
		case lexer.TokenEndpackage:
			popBlock(lexer.TokenEndpackage, tok.Range.Start.Line)

		case lexer.TokenProgram:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndprogram, startLine: tok.Range.Start.Line})
		case lexer.TokenEndprogram:
			popBlock(lexer.TokenEndprogram, tok.Range.Start.Line)

		case lexer.TokenChecker:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndchecker, startLine: tok.Range.Start.Line})
		case lexer.TokenEndchecker:
			popBlock(lexer.TokenEndchecker, tok.Range.Start.Line)

		case lexer.TokenClass:
			// Exclude forward typedef `typedef class foo;`.
			if !stmtHasTypedef {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndclass, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndclass:
			popBlock(lexer.TokenEndclass, tok.Range.Start.Line)

		case lexer.TokenFunction:
			if !stmtHasExternOrPure && !stmtHasDPI && !inParens {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndfunction, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndfunction:
			popBlock(lexer.TokenEndfunction, tok.Range.Start.Line)

		case lexer.TokenTask:
			if !stmtHasExternOrPure && !stmtHasDPI && !inParens {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndtask, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndtask:
			popBlock(lexer.TokenEndtask, tok.Range.Start.Line)

		case lexer.TokenBegin:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEnd, startLine: tok.Range.Start.Line})
		case lexer.TokenEnd:
			popBlock(lexer.TokenEnd, tok.Range.Start.Line)

		case lexer.TokenFork:
			// Exclude `disable fork;` and `wait fork;`.
			if prevCodeTok.Type != lexer.TokenDisable && prevCodeTok.Type != lexer.TokenWait {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenJoin, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenJoin, lexer.TokenJoinAny, lexer.TokenJoinNone:
			popBlock(lexer.TokenJoin, tok.Range.Start.Line)

		case lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez, lexer.TokenRandcase:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndcase, startLine: tok.Range.Start.Line})
		case lexer.TokenEndcase:
			popBlock(lexer.TokenEndcase, tok.Range.Start.Line)

		case lexer.TokenGenerate:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndgenerate, startLine: tok.Range.Start.Line})
		case lexer.TokenEndgenerate:
			popBlock(lexer.TokenEndgenerate, tok.Range.Start.Line)

		case lexer.TokenClocking:
			// Exclude `default clocking cb;` (reference to an existing clocking block without body).
			if !inParens && !(stmtHasDefault && i+2 < len(tokens) && tokens[i+1].Type == lexer.TokenIdent && tokens[i+2].Type == lexer.TokenSemicolon) {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndclocking, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndclocking:
			popBlock(lexer.TokenEndclocking, tok.Range.Start.Line)

		case lexer.TokenCovergroup:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndgroup, startLine: tok.Range.Start.Line})
		case lexer.TokenEndgroup:
			popBlock(lexer.TokenEndgroup, tok.Range.Start.Line)

		case lexer.TokenProperty:
			// Exclude `assert/assume/cover/restrict property (...)` and checker port types.
			if !inParens && prevCodeTok.Type != lexer.TokenAssert && prevCodeTok.Type != lexer.TokenAssume &&
				prevCodeTok.Type != lexer.TokenCover && prevCodeTok.Type != lexer.TokenRestrict {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndproperty, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenEndproperty:
			popBlock(lexer.TokenEndproperty, tok.Range.Start.Line)

		case lexer.TokenSequence:
			if !inParens && prevCodeTok.Type != lexer.TokenCover && prevCodeTok.Type != lexer.TokenRestrict {
				blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndsequence, startLine: tok.Range.Start.Line})
			}
		case lexer.TokenRandsequence:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndsequence, startLine: tok.Range.Start.Line})
		case lexer.TokenEndsequence:
			popBlock(lexer.TokenEndsequence, tok.Range.Start.Line)

		case lexer.TokenSpecify:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndspecify, startLine: tok.Range.Start.Line})
		case lexer.TokenEndspecify:
			popBlock(lexer.TokenEndspecify, tok.Range.Start.Line)

		case lexer.TokenPrimitive:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndprimitive, startLine: tok.Range.Start.Line})
		case lexer.TokenEndprimitive:
			popBlock(lexer.TokenEndprimitive, tok.Range.Start.Line)

		case lexer.TokenTable:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndtable, startLine: tok.Range.Start.Line})
		case lexer.TokenEndtable:
			popBlock(lexer.TokenEndtable, tok.Range.Start.Line)

		case lexer.TokenConfig:
			blockStack = append(blockStack, foldEntry{closeType: lexer.TokenEndconfig, startLine: tok.Range.Start.Line})
		case lexer.TokenEndconfig:
			popBlock(lexer.TokenEndconfig, tok.Range.Start.Line)
		}

		if tok.Type == lexer.TokenSemicolon {
			stmtHasExternOrPure = false
			stmtHasDPI = false
			stmtHasTypedef = false
			stmtHasVirtual = false
			stmtHasDefault = false
		}
		prevCodeTok = tok
		lastCodeLine = tok.Range.End.Line
	}

	flushComments()
	flushImports()

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartLine != ranges[j].StartLine {
			return ranges[i].StartLine < ranges[j].StartLine
		}
		return ranges[i].EndLine < ranges[j].EndLine
	})

	// Deduplicate identical (StartLine, EndLine) pairs.
	dedup := ranges[:0]
	for _, r := range ranges {
		if len(dedup) > 0 && dedup[len(dedup)-1].StartLine == r.StartLine && dedup[len(dedup)-1].EndLine == r.EndLine {
			continue
		}
		dedup = append(dedup, r)
	}
	return dedup
}
