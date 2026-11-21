package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvx/internal/ast"
	"kvx/internal/lexer"
)

// Options configures file path, `include resolution, and predefined macros during parsing.
type Options struct {
	// FilePath is the path of the source file being parsed (used to resolve relative `include paths).
	FilePath string
	// IncludeDirs is an optional list of directories searched for `include files.
	IncludeDirs []string
	// Defines holds predefined macro names and their optional replacement text (e.g. from kvx.json).
	Defines map[string]string
	// ReadFile reads a file by path. If nil, os.ReadFile is used.
	ReadFile func(path string) ([]byte, error)
}

type condFrame struct {
	parentActive bool
	branchActive bool
	matched      bool
	seenElse     bool
	startRange   ast.Range
	rootRange    ast.Range
}

const maxMacroExpandDepth = 32

type macroParam struct {
	name       string
	defaultVal []lexer.Token
	hasDefault bool
}

type macroDef struct {
	params  []macroParam
	hasArgs bool
	body    []lexer.Token
}

type preprocessState struct {
	opts       Options
	macros     map[string]macroDef
	condStack  []condFrame
	includes   []*ast.IncludeDecl
	macroDecls []*ast.MacroDecl
	macroRefs  []ast.IdentifierRef
	errors     []ParseError
	// fileOrigin maps an included file path to the root-file `include Range that included it.
	fileOrigin map[string]ast.Range
}

func newPreprocessState(opts Options) *preprocessState {
	if opts.ReadFile == nil {
		opts.ReadFile = os.ReadFile
	}
	macros := make(map[string]macroDef, len(opts.Defines))
	for name, val := range opts.Defines {
		if name == "" {
			continue
		}
		if val == "" {
			macros[name] = macroDef{}
			continue
		}
		toks := lexer.Tokenize(val)
		if len(toks) > 0 && toks[len(toks)-1].Type == lexer.TokenEOF {
			toks = toks[:len(toks)-1]
		}
		macros[name] = macroDef{body: toks}
	}
	return &preprocessState{
		opts:       opts,
		macros:     macros,
		fileOrigin: make(map[string]ast.Range),
	}
}

func (s *preprocessState) isActive() bool {
	if len(s.condStack) == 0 {
		return true
	}
	top := s.condStack[len(s.condStack)-1]
	return top.parentActive && top.branchActive
}

func (s *preprocessState) preprocessRoot(input string) []lexer.Token {
	var stack []string
	if s.opts.FilePath != "" {
		if abs, err := filepath.Abs(s.opts.FilePath); err == nil {
			stack = append(stack, abs)
		} else {
			stack = append(stack, filepath.Clean(s.opts.FilePath))
		}
	}
	tokens := s.preprocessText(input, s.opts.FilePath, "", ast.Range{}, stack)
	for _, frame := range s.condStack {
		s.addPreprocessError(frame.startRange, frame.rootRange, "unterminated `ifdef or `ifndef")
	}
	pos := ast.Position{}
	if len(tokens) > 0 {
		pos = tokens[len(tokens)-1].Range.End
	}
	tokens = append(tokens, lexer.Token{
		Type:    lexer.TokenEOF,
		Literal: "",
		Range:   ast.Range{Start: pos, End: pos},
	})
	return tokens
}

func (s *preprocessState) preprocessText(
	input string,
	currentFilePath string,
	fileTag string,
	rootIncRange ast.Range,
	includeStack []string,
) []lexer.Token {
	raw := lexer.TokenizeWithDirectives(input)
	var out []lexer.Token

	for i := 0; i < len(raw); i++ {
		tok := raw[i]
		if tok.Type == lexer.TokenEOF {
			break
		}
		tok.Range.File = fileTag

		if tok.Type == lexer.TokenDirective {
			dirLine := tok.Range.Start.Line
			switch tok.Literal {
			case "`ifdef", "`ifndef":
				parentActive := s.isActive()
				if i+1 >= len(raw) || raw[i+1].Range.Start.Line != dirLine || raw[i+1].Type == lexer.TokenEOF {
					if parentActive {
						s.addPreprocessError(tok.Range, rootIncRange, fmt.Sprintf("expected macro name after %s", tok.Literal))
					}
					s.condStack = append(s.condStack, condFrame{
						parentActive: parentActive,
						branchActive: false,
						matched:      true,
						startRange:   tok.Range,
						rootRange:    rootIncRange,
					})
					continue
				}
				i++
				argTok := raw[i]
				argTok.Range.File = fileTag
				if parentActive {
					s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
						Name:  "`" + argTok.Literal,
						Range: argTok.Range,
					})
				}
				_, isDefined := s.macros[argTok.Literal]
				cond := isDefined
				if tok.Literal == "`ifndef" {
					cond = !isDefined
				}
				branchActive := parentActive && cond
				s.condStack = append(s.condStack, condFrame{
					parentActive: parentActive,
					branchActive: branchActive,
					matched:      !parentActive || branchActive,
					startRange:   tok.Range,
					rootRange:    rootIncRange,
				})
				continue

			case "`elsif":
				if len(s.condStack) == 0 {
					s.addPreprocessError(tok.Range, rootIncRange, "unexpected `elsif without matching `ifdef or `ifndef")
					i = skipDirectiveLine(raw, i, dirLine)
					continue
				}
				top := &s.condStack[len(s.condStack)-1]
				if top.seenElse {
					s.addPreprocessError(tok.Range, rootIncRange, "`elsif after `else is not allowed")
				}
				if i+1 >= len(raw) || raw[i+1].Range.Start.Line != dirLine || raw[i+1].Type == lexer.TokenEOF {
					if top.parentActive {
						s.addPreprocessError(tok.Range, rootIncRange, "expected macro name after `elsif")
					}
					top.branchActive = false
					continue
				}
				i++
				argTok := raw[i]
				argTok.Range.File = fileTag
				if top.parentActive {
					s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
						Name:  "`" + argTok.Literal,
						Range: argTok.Range,
					})
				}
				if !top.parentActive || top.matched || top.seenElse {
					top.branchActive = false
				} else {
					_, isDefined := s.macros[argTok.Literal]
					top.branchActive = isDefined
					if isDefined {
						top.matched = true
					}
				}
				continue

			case "`else":
				if len(s.condStack) == 0 {
					s.addPreprocessError(tok.Range, rootIncRange, "unexpected `else without matching `ifdef or `ifndef")
					continue
				}
				top := &s.condStack[len(s.condStack)-1]
				if top.seenElse {
					s.addPreprocessError(tok.Range, rootIncRange, "duplicate `else in conditional block")
					top.branchActive = false
					continue
				}
				top.seenElse = true
				if top.parentActive && !top.matched {
					top.branchActive = true
					top.matched = true
				} else {
					top.branchActive = false
				}
				continue

			case "`endif":
				if len(s.condStack) == 0 {
					s.addPreprocessError(tok.Range, rootIncRange, "unexpected `endif without matching `ifdef or `ifndef")
					continue
				}
				s.condStack = s.condStack[:len(s.condStack)-1]
				continue
			}

			// Skip all other directives when inside an inactive conditional branch.
			if !s.isActive() {
				i = skipDirectiveLine(raw, i, dirLine)
				continue
			}

			switch tok.Literal {
			case "`include":
				if i+1 >= len(raw) || raw[i+1].Range.Start.Line != dirLine || raw[i+1].Type != lexer.TokenString {
					s.addPreprocessError(tok.Range, rootIncRange, "expected filename string after `include")
					i = skipDirectiveLine(raw, i, dirLine)
					continue
				}
				strTok := raw[i+1]
				i++
				incRange := ast.Range{
					File:  fileTag,
					Start: tok.Range.Start,
					End:   strTok.Range.End,
				}
				effRootRange := incRange
				if fileTag != "" {
					effRootRange = rootIncRange
				}

				resolvedPath, data, err := s.resolveInclude(strTok.Literal, currentFilePath)
				if fileTag == "" {
					s.includes = append(s.includes, &ast.IncludeDecl{
						Path:         strTok.Literal,
						ResolvedPath: resolvedPath,
						Range:        incRange,
					})
				}
				if err != nil {
					s.addPreprocessError(incRange, effRootRange, fmt.Sprintf("cannot open include file %q", strTok.Literal))
					i = skipDirectiveLine(raw, i, dirLine)
					continue
				}

				if containsPath(includeStack, resolvedPath) {
					s.addPreprocessError(incRange, effRootRange, fmt.Sprintf("circular include detected for %q", strTok.Literal))
					i = skipDirectiveLine(raw, i, dirLine)
					continue
				}

				if _, exists := s.fileOrigin[resolvedPath]; !exists {
					s.fileOrigin[resolvedPath] = effRootRange
				}

				nextStack := append(includeStack, resolvedPath)
				incTokens := s.preprocessText(string(data), resolvedPath, resolvedPath, effRootRange, nextStack)
				out = append(out, incTokens...)
				i = skipDirectiveLine(raw, i, dirLine)

			case "`define":
				if i+1 >= len(raw) || raw[i+1].Range.Start.Line != dirLine || raw[i+1].Type == lexer.TokenEOF {
					s.addPreprocessError(tok.Range, rootIncRange, "expected macro name after `define")
					continue
				}
				nameTok := raw[i+1]
				nameTok.Range.File = fileTag
				i++

				// If this is a function-like macro `define FOO(a, b), parse the parameter list (...)
				// immediately following the macro name (no whitespace before '(').
				var params []macroParam
				hasArgs := false
				curLine := dirLine
				prevOff := nameTok.Range.End.Offset
				if i+1 < len(raw) && raw[i+1].Range.Start.Line == curLine &&
					raw[i+1].Type == lexer.TokenLParen &&
					raw[i+1].Range.Start.Column == nameTok.Range.End.Column {
					hasArgs = true
					i++ // consume '('
					params, i, curLine = parseMacroParams(input, raw, i, curLine, fileTag)
					prevOff = raw[i].Range.End.Offset
				}

				var bodyTokens []lexer.Token
				var bodyParts []string
				endPos := nameTok.Range.End

				for i+1 < len(raw) && raw[i+1].Type != lexer.TokenEOF {
					okLine, nextLine := sameDirectiveLine(input, curLine, prevOff, raw[i+1])
					if !okLine {
						break
					}
					curLine = nextLine
					i++
					bt := raw[i]
					prevOff = bt.Range.End.Offset
					if bt.Literal == "\\" {
						// Line continuation to next line.
						curLine = bt.Range.End.Line + 1
						continue
					}
					curLine = bt.Range.End.Line
					bt.Range.File = fileTag
					bodyTokens = append(bodyTokens, bt)
					bodyParts = append(bodyParts, bt.Literal)
					endPos = bt.Range.End
				}

				macroDecl := &ast.MacroDecl{
					Name:      nameTok.Literal,
					NameRange: nameTok.Range,
					ValueText: strings.Join(bodyParts, " "),
					Range: ast.Range{
						File:  fileTag,
						Start: tok.Range.Start,
						End:   endPos,
					},
				}
				s.macroDecls = append(s.macroDecls, macroDecl)
				s.macros[nameTok.Literal] = macroDef{
					params:  params,
					hasArgs: hasArgs,
					body:    bodyTokens,
				}

			case "`undef":
				if i+1 >= len(raw) || raw[i+1].Range.Start.Line != dirLine || raw[i+1].Type == lexer.TokenEOF {
					s.addPreprocessError(tok.Range, rootIncRange, "expected macro name after `undef")
					continue
				}
				i++
				argTok := raw[i]
				argTok.Range.File = fileTag
				s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
					Name:  "`" + argTok.Literal,
					Range: argTok.Range,
				})
				delete(s.macros, argTok.Literal)

			case "`resetall", "`celldefine", "`endcelldefine", "`end_keywords", "`nounconnected_drive":
				// Standalone directive with no arguments on the line.

			case "`undefineall":
				s.macros = make(map[string]macroDef)

			default:
				// Directives such as `timescale, `default_nettype, `pragma, `line, `begin_keywords, `unconnected_drive: skip the line.
				i = skipDirectiveLine(raw, i, dirLine)
			}
			continue
		}

		if !s.isActive() {
			continue
		}

		// Handle macro usage: `MACRO_NAME or `MACRO_NAME(arg1, arg2)
		if tok.Type == lexer.TokenIdent && strings.HasPrefix(tok.Literal, "`") {
			expanded, nextIdx, ok := s.tryExpandMacro(raw, i, fileTag, rootIncRange, map[string]bool{}, 0, true)
			if ok {
				out = append(out, expanded...)
				i = nextIdx
				continue
			}
		}

		out = append(out, tok)
	}

	return out
}

func parseMacroParams(input string, raw []lexer.Token, lparenIdx int, curLine int, fileTag string) ([]macroParam, int, int) {
	var params []macroParam
	i := lparenIdx
	depth := 1
	var currentName string
	var defaultToks []lexer.Token
	inDefault := false
	prevOff := raw[lparenIdx].Range.End.Offset

	flushParam := func() {
		if currentName != "" {
			params = append(params, macroParam{
				name:       currentName,
				defaultVal: defaultToks,
				hasDefault: inDefault,
			})
		}
		currentName = ""
		defaultToks = nil
		inDefault = false
	}

	for i+1 < len(raw) && depth > 0 {
		next := raw[i+1]
		if next.Type == lexer.TokenEOF {
			break
		}
		okLine, nextLine := sameDirectiveLine(input, curLine, prevOff, next)
		if !okLine {
			break
		}
		curLine = nextLine
		i++
		tok := raw[i]
		prevOff = tok.Range.End.Offset
		if tok.Literal == "\\" {
			curLine = tok.Range.End.Line + 1
			continue
		}
		curLine = tok.Range.End.Line
		tok.Range.File = fileTag

		switch tok.Type {
		case lexer.TokenLParen:
			depth++
			if inDefault {
				defaultToks = append(defaultToks, tok)
			}
		case lexer.TokenRParen:
			depth--
			if depth == 0 {
				flushParam()
			} else if inDefault {
				defaultToks = append(defaultToks, tok)
			}
		case lexer.TokenComma:
			if depth == 1 {
				flushParam()
			} else if inDefault {
				defaultToks = append(defaultToks, tok)
			}
		case lexer.TokenAssignOp:
			if depth == 1 && currentName != "" && !inDefault {
				inDefault = true
			} else if inDefault {
				defaultToks = append(defaultToks, tok)
			}
		default:
			if inDefault {
				defaultToks = append(defaultToks, tok)
			} else if depth == 1 && currentName == "" && tok.Type == lexer.TokenIdent {
				currentName = tok.Literal
			}
		}
	}

	return params, i, curLine
}

// parseMacroCallArgs parses the actual arguments in `FOO(arg1, arg2, ...)` starting at lparenIdx.
// It returns the slice of argument token lists, the index of the closing ')', and whether ')' was found.
func parseMacroCallArgs(tokens []lexer.Token, lparenIdx int, fileTag string) ([][]lexer.Token, int, bool) {
	var args [][]lexer.Token
	var curArg []lexer.Token
	parenDepth := 1
	bracketDepth := 0
	braceDepth := 0
	sawAnyToken := false
	i := lparenIdx

	for i+1 < len(tokens) {
		i++
		tok := tokens[i]
		if tok.Type == lexer.TokenEOF {
			return args, i - 1, false
		}
		if tok.Range.File == "" {
			tok.Range.File = fileTag
		}

		switch tok.Type {
		case lexer.TokenLParen:
			parenDepth++
			sawAnyToken = true
			curArg = append(curArg, tok)
		case lexer.TokenRParen:
			parenDepth--
			if parenDepth == 0 {
				if sawAnyToken || len(args) > 0 {
					args = append(args, curArg)
				}
				return args, i, true
			}
			curArg = append(curArg, tok)
		case lexer.TokenLBracket:
			bracketDepth++
			sawAnyToken = true
			curArg = append(curArg, tok)
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			sawAnyToken = true
			curArg = append(curArg, tok)
		case lexer.TokenLBrace:
			braceDepth++
			sawAnyToken = true
			curArg = append(curArg, tok)
		case lexer.TokenRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
			sawAnyToken = true
			curArg = append(curArg, tok)
		case lexer.TokenComma:
			if parenDepth == 1 && bracketDepth == 0 && braceDepth == 0 {
				args = append(args, curArg)
				curArg = nil
				sawAnyToken = true
			} else {
				sawAnyToken = true
				curArg = append(curArg, tok)
			}
		default:
			sawAnyToken = true
			curArg = append(curArg, tok)
		}
	}

	return args, i, false
}

func (s *preprocessState) tryExpandMacro(
	tokens []lexer.Token,
	idx int,
	fileTag string,
	rootIncRange ast.Range,
	expanding map[string]bool,
	depth int,
	recordRefs bool,
) ([]lexer.Token, int, bool) {
	tok := tokens[idx]
	if tok.Range.File == "" {
		tok.Range.File = fileTag
	}
	macroName := strings.TrimPrefix(tok.Literal, "`")
	if recordRefs {
		s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
			Name:  tok.Literal,
			Range: tok.Range,
		})
	}

	def, ok := s.macros[macroName]
	if !ok || expanding[macroName] || depth >= maxMacroExpandDepth {
		return nil, idx, false
	}

	if def.hasArgs {
		// Function-like macro invocation requires '(' following the macro name.
		if idx+1 >= len(tokens) || tokens[idx+1].Type != lexer.TokenLParen {
			return nil, idx, false
		}
		rawArgs, endIdx, closed := parseMacroCallArgs(tokens, idx+1, fileTag)
		if !closed {
			s.addPreprocessError(tok.Range, rootIncRange, fmt.Sprintf("unterminated argument list for macro `%s", macroName))
		}
		// Handle single empty argument: `FOO()` for a 1-parameter macro passes 1 empty arg.
		if len(rawArgs) == 0 && len(def.params) == 1 && !def.params[0].hasDefault {
			rawArgs = [][]lexer.Token{nil}
		}

		if len(rawArgs) > len(def.params) {
			s.addPreprocessError(tok.Range, rootIncRange, fmt.Sprintf("too many arguments for macro `%s: got %d, want %d", macroName, len(rawArgs), len(def.params)))
		}
		minRequired := 0
		for idxP, p := range def.params {
			if !p.hasDefault {
				minRequired = idxP + 1
			}
		}
		if len(rawArgs) < minRequired {
			s.addPreprocessError(tok.Range, rootIncRange, fmt.Sprintf("too few arguments for macro `%s: got %d, want at least %d", macroName, len(rawArgs), minRequired))
		}

		// Pre-expand macros within actual arguments so nested calls like `ADD(`A, `B) work cleanly.
		paramMap := make(map[string][]lexer.Token, len(def.params))
		for pIdx, p := range def.params {
			var argToks []lexer.Token
			if pIdx < len(rawArgs) && len(rawArgs[pIdx]) > 0 {
				argToks = s.expandTokenSlice(rawArgs[pIdx], fileTag, rootIncRange, expanding, depth+1, recordRefs)
			} else if p.hasDefault {
				argToks = cloneTokensWithRange(p.defaultVal, tok.Range)
			}
			paramMap[p.name] = argToks
		}

		substituted := make([]lexer.Token, 0, len(def.body))
		for _, bt := range def.body {
			if bt.Type == lexer.TokenIdent {
				if argToks, isParam := paramMap[bt.Literal]; isParam {
					substituted = append(substituted, argToks...)
					continue
				}
			}
			expanded := bt
			expanded.Range = tok.Range
			substituted = append(substituted, expanded)
		}

		substituted = pasteMacroTokens(substituted)
		nextExpanding := copyExpandingSet(expanding, macroName)
		res := s.expandTokenSlice(substituted, fileTag, rootIncRange, nextExpanding, depth+1, false)
		return res, endIdx, true
	}

	// Object-like macro
	if len(def.body) == 0 {
		return nil, idx, true
	}

	substituted := pasteMacroTokens(cloneTokensWithRange(def.body, tok.Range))
	nextExpanding := copyExpandingSet(expanding, macroName)
	res := s.expandTokenSlice(substituted, fileTag, rootIncRange, nextExpanding, depth+1, false)
	return res, idx, true
}

// pasteMacroTokens resolves the `` token pasting operator inside an expanded macro body.
func pasteMacroTokens(tokens []lexer.Token) []lexer.Token {
	hasPaste := false
	for _, t := range tokens {
		if t.Type == lexer.TokenPaste {
			hasPaste = true
			break
		}
	}
	if !hasPaste {
		return tokens
	}

	out := make([]lexer.Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Type == lexer.TokenPaste {
			if len(out) > 0 && i+1 < len(tokens) {
				prev := out[len(out)-1]
				next := tokens[i+1]
				combinedLit := prev.Literal + next.Literal
				out[len(out)-1] = lexer.Token{
					Type:    lexer.LookupIdent(combinedLit),
					Literal: combinedLit,
					Range:   prev.Range,
				}
				i++ // consume next
			}
			continue
		}
		out = append(out, tokens[i])
	}
	return out
}

func (s *preprocessState) expandTokenSlice(
	tokens []lexer.Token,
	fileTag string,
	rootIncRange ast.Range,
	expanding map[string]bool,
	depth int,
	recordRefs bool,
) []lexer.Token {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]lexer.Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type == lexer.TokenIdent && strings.HasPrefix(tok.Literal, "`") {
			if expanded, nextIdx, ok := s.tryExpandMacro(tokens, i, fileTag, rootIncRange, expanding, depth, recordRefs); ok {
				out = append(out, expanded...)
				i = nextIdx
				continue
			}
		}
		out = append(out, tok)
	}
	return out
}

func cloneTokensWithRange(tokens []lexer.Token, rng ast.Range) []lexer.Token {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]lexer.Token, len(tokens))
	for i, t := range tokens {
		t.Range = rng
		out[i] = t
	}
	return out
}

func copyExpandingSet(src map[string]bool, add string) map[string]bool {
	dst := make(map[string]bool, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	dst[add] = true
	return dst
}

func (s *preprocessState) resolveInclude(incPath string, currentFilePath string) (string, []byte, error) {
	var candidates []string

	if filepath.IsAbs(incPath) {
		candidates = append(candidates, incPath)
	} else {
		if currentFilePath != "" {
			candidates = append(candidates, filepath.Join(filepath.Dir(currentFilePath), incPath))
		}
		for _, dir := range s.opts.IncludeDirs {
			if dir != "" {
				candidates = append(candidates, filepath.Join(dir, incPath))
			}
		}
		if currentFilePath == "" {
			candidates = append(candidates, incPath)
		}
	}

	for _, cand := range candidates {
		data, err := s.opts.ReadFile(cand)
		if err == nil {
			if abs, absErr := filepath.Abs(cand); absErr == nil {
				return abs, data, nil
			}
			return filepath.Clean(cand), data, nil
		}
	}

	return "", nil, os.ErrNotExist
}

func (s *preprocessState) addPreprocessError(rng ast.Range, rootIncRange ast.Range, msg string) {
	if rng.File != "" {
		msg = fmt.Sprintf("in included file %q (%d:%d): %s",
			filepath.Base(rng.File), rng.Start.Line+1, rng.Start.Column+1, msg)
		rng = rootIncRange
	}
	s.errors = append(s.errors, ParseError{
		Range:   rng,
		Message: msg,
	})
}

// sameDirectiveLine reports whether nextTok is on the same logical directive line
// (either on curLine, or separated only by a multiline /* ... */ comment without any top-level newline).
func sameDirectiveLine(input string, curLine int, prevOffset int, nextTok lexer.Token) (bool, int) {
	if nextTok.Range.Start.Line == curLine {
		return true, curLine
	}
	nextOff := nextTok.Range.Start.Offset
	if prevOffset < 0 || nextOff > len(input) || prevOffset >= nextOff {
		return false, curLine
	}
	between := input[prevOffset:nextOff]
	inBlockComment := false
	for j := 0; j < len(between); j++ {
		if !inBlockComment && j+1 < len(between) && between[j] == '/' && between[j+1] == '*' {
			inBlockComment = true
			j++
			continue
		}
		if inBlockComment && j+1 < len(between) && between[j] == '*' && between[j+1] == '/' {
			inBlockComment = false
			j++
			continue
		}
		if !inBlockComment && between[j] == '\n' {
			return false, curLine
		}
	}
	return true, nextTok.Range.Start.Line
}

func skipDirectiveLine(tokens []lexer.Token, idx int, line int) int {
	curLine := line
	for idx+1 < len(tokens) && tokens[idx+1].Type != lexer.TokenEOF && tokens[idx+1].Range.Start.Line == curLine {
		idx++
		if tokens[idx].Literal == "\\" {
			curLine++
		}
	}
	return idx
}

func containsPath(stack []string, target string) bool {
	for _, p := range stack {
		if p == target {
			return true
		}
	}
	return false
}
