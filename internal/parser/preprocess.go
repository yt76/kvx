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

type preprocessState struct {
	opts       Options
	macros     map[string][]lexer.Token
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
	macros := make(map[string][]lexer.Token, len(opts.Defines))
	for name, val := range opts.Defines {
		if name == "" {
			continue
		}
		if val == "" {
			macros[name] = nil
			continue
		}
		toks := lexer.Tokenize(val)
		if len(toks) > 0 && toks[len(toks)-1].Type == lexer.TokenEOF {
			toks = toks[:len(toks)-1]
		}
		macros[name] = toks
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

				// If this is a function-like macro `define FOO(a, b), skip the parameter list (...)
				// immediately following the macro name.
				if i+1 < len(raw) && raw[i+1].Range.Start.Line == dirLine &&
					raw[i+1].Type == lexer.TokenLParen &&
					raw[i+1].Range.Start.Column == nameTok.Range.End.Column {
					i++ // consume '('
					depth := 1
					for i+1 < len(raw) && depth > 0 {
						if raw[i+1].Range.Start.Line != dirLine {
							break
						}
						i++
						if raw[i].Type == lexer.TokenLParen {
							depth++
						} else if raw[i].Type == lexer.TokenRParen {
							depth--
						}
					}
				}

				var bodyTokens []lexer.Token
				var bodyParts []string
				endPos := nameTok.Range.End
				curLine := dirLine

				for i+1 < len(raw) && raw[i+1].Type != lexer.TokenEOF && raw[i+1].Range.Start.Line == curLine {
					i++
					bt := raw[i]
					if bt.Literal == "\\" {
						// Line continuation to next line.
						curLine++
						continue
					}
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
				s.macros[nameTok.Literal] = bodyTokens

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

			case "`resetall", "`celldefine", "`endcelldefine":
				// Standalone directive with no arguments on the line.

			default:
				// Directives such as `timescale, `default_nettype, `pragma, `line: skip the line.
				i = skipDirectiveLine(raw, i, dirLine)
			}
			continue
		}

		if !s.isActive() {
			continue
		}

		// Handle macro usage: `MACRO_NAME
		if tok.Type == lexer.TokenIdent && strings.HasPrefix(tok.Literal, "`") {
			macroName := strings.TrimPrefix(tok.Literal, "`")
			s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
				Name:  tok.Literal,
				Range: tok.Range,
			})
			if body, ok := s.macros[macroName]; ok && len(body) > 0 {
				for _, bt := range body {
					expanded := bt
					expanded.Range = tok.Range
					out = append(out, expanded)
				}
				continue
			}
		}

		out = append(out, tok)
	}

	return out
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
