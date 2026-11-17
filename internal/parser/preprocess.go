package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvx/internal/ast"
	"kvx/internal/lexer"
)

// Options configures file path and `include resolution during parsing.
type Options struct {
	// FilePath is the path of the source file being parsed (used to resolve relative `include paths).
	FilePath string
	// IncludeDirs is an optional list of directories searched for `include files.
	IncludeDirs []string
	// ReadFile reads a file by path. If nil, os.ReadFile is used.
	ReadFile func(path string) ([]byte, error)
}

type preprocessState struct {
	opts       Options
	macros     map[string][]lexer.Token
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
	return &preprocessState{
		opts:       opts,
		macros:     make(map[string][]lexer.Token),
		fileOrigin: make(map[string]ast.Range),
	}
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

			case "`ifdef", "`ifndef", "`elsif", "`undef":
				if i+1 < len(raw) && raw[i+1].Range.Start.Line == dirLine && raw[i+1].Type == lexer.TokenIdent {
					i++
					argTok := raw[i]
					argTok.Range.File = fileTag
					s.macroRefs = append(s.macroRefs, ast.IdentifierRef{
						Name:  "`" + argTok.Literal,
						Range: argTok.Range,
					})
				}

			case "`else", "`endif", "`resetall", "`celldefine", "`endcelldefine":
				// Standalone directive with no arguments on the line.

			default:
				// Directives such as `timescale, `default_nettype, `pragma, `line: skip the line.
				i = skipDirectiveLine(raw, i, dirLine)
			}
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
