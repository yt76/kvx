package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"kvx/internal/ast"
	"kvx/internal/lexer"
)

// ParseError represents a syntax error discovered during parsing.
type ParseError struct {
	Range   ast.Range
	Message string
}

func (e ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Range.Start.Line+1, e.Range.Start.Column+1, e.Message)
}

// Parser is a hand-written recursive descent parser for core Verilog.
type Parser struct {
	tokens     []lexer.Token
	pos        int
	errors     []ParseError
	fileOrigin map[string]ast.Range
}

// Parse tokenizes and parses input into an AST SourceFile and a list of syntax errors.
func Parse(input string) (*ast.SourceFile, []ParseError) {
	return ParseWithOptions(input, Options{})
}

// ParseWithOptions preprocesses (`include and `define) and parses input into an AST SourceFile.
func ParseWithOptions(input string, opts Options) (*ast.SourceFile, []ParseError) {
	pp := newPreprocessState(opts)
	tokens := pp.preprocessRoot(input)

	p := &Parser{
		tokens:     tokens,
		errors:     append([]ParseError(nil), pp.errors...),
		fileOrigin: pp.fileOrigin,
	}
	file := p.parseSourceFile()
	file.Includes = pp.includes
	file.Macros = pp.macroDecls
	file.Refs = append(file.Refs, pp.macroRefs...)
	return file, p.errors
}

func (p *Parser) parseSourceFile() *ast.SourceFile {
	file := &ast.SourceFile{}

	for !p.isEOF() {
		tok := p.cur()
		if tok.Type == lexer.TokenIllegal {
			p.addError(tok.Range, tok.Literal)
			p.advance()
			continue
		}
		if tok.Type == lexer.TokenSemicolon {
			p.advance()
			continue
		}
		if p.isAttrStart() {
			p.skipAttributes()
			continue
		}
		if tok.Type == lexer.TokenModule {
			if mod := p.parseModuleDecl(); mod != nil {
				file.Modules = append(file.Modules, mod)
			}
			continue
		}
		if tok.Type == lexer.TokenParameter || tok.Type == lexer.TokenLocalparam {
			p.parseTopLevelParamDecl(file)
			continue
		}

		p.addError(tok.Range, fmt.Sprintf("unexpected token %q outside module declaration", tok.Literal))
		p.advance()
		for !p.isEOF() &&
			p.cur().Type != lexer.TokenModule &&
			p.cur().Type != lexer.TokenParameter &&
			p.cur().Type != lexer.TokenLocalparam {
			if p.cur().Type == lexer.TokenIllegal {
				p.addError(p.cur().Range, p.cur().Literal)
			}
			p.advance()
		}
	}

	return file
}

func (p *Parser) parseTopLevelParamDecl(file *ast.SourceFile) {
	dummyMod := &ast.ModuleDecl{}
	p.parseParamDeclItem(dummyMod)
	file.Params = append(file.Params, dummyMod.Params...)
	file.Refs = append(file.Refs, dummyMod.Refs...)
}

func (p *Parser) parseModuleDecl() *ast.ModuleDecl {
	startTok := p.advance() // consume 'module'
	mod := &ast.ModuleDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		mod.Name = nameTok.Literal
		mod.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected module name after 'module'")
	}

	// Optional parameter port list: #(...)
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseModuleParamPortList(mod)
		} else {
			p.addError(p.cur().Range, "expected '(' after '#' in module header")
		}
	}

	// Optional port list: (...)
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(mod)
	}

	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
	} else {
		p.addError(p.prev().Range, "expected ';' after module header")
	}

	// Parse module body items until 'endmodule', 'module', or EOF.
	for !p.isEOF() && p.cur().Type != lexer.TokenEndmodule && p.cur().Type != lexer.TokenModule {
		p.parseModuleItem(mod)
	}

	if p.cur().Type == lexer.TokenEndmodule {
		endTok := p.advance()
		mod.Range.End = endTok.Range.End
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected 'endmodule' to close module %q", mod.Name))
		mod.Range.End = p.prev().Range.End
	}

	return mod
}

func (p *Parser) parseModuleParamPortList(mod *ast.ModuleDecl) {
	p.advance() // consume '('

	for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
		p.skipAttributes()
		if p.cur().Type == lexer.TokenRParen {
			break
		}
		startPos := p.cur().Range.Start
		kind := "parameter"
		if p.cur().Type == lexer.TokenParameter || p.cur().Type == lexer.TokenLocalparam {
			kind = p.cur().Literal
			p.advance()
		}

		signed := false
		if p.cur().Type == lexer.TokenSigned {
			signed = true
			p.advance()
		}

		var width *ast.BitRange
		if p.cur().Type == lexer.TokenLBracket {
			width = p.parseBitRange(mod)
		}

		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected parameter name")
			p.syncToParamOrPortDelimiter()
			if p.cur().Type == lexer.TokenComma {
				p.advance()
			}
			continue
		}

		nameTok := p.advance()
		valText := ""
		endPos := nameTok.Range.End

		if p.cur().Type == lexer.TokenAssignOp {
			p.advance() // '='
			valText, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenRParen)
		}

		param := &ast.ParamDecl{
			Kind:      kind,
			Signed:    signed,
			Width:     width,
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			ValueText: valText,
			Range:     ast.Range{File: nameTok.Range.File, Start: startPos, End: endPos},
		}
		mod.Params = append(mod.Params, param)
		mod.Items = append(mod.Items, param)

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	if p.cur().Type == lexer.TokenRParen {
		p.advance()
	} else {
		p.addError(p.prev().Range, "expected ')' to close parameter list")
	}
}

func (p *Parser) parseModulePortList(mod *ast.ModuleDecl) {
	p.advance() // consume '('

	var curDir string
	var curNetType string
	var curSigned bool
	var curWidth *ast.BitRange

	for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
		p.skipAttributes()
		if p.cur().Type == lexer.TokenRParen {
			break
		}
		startPos := p.cur().Range.Start

		if isPortDirection(p.cur().Type) {
			curDir = p.cur().Literal
			curNetType = ""
			curSigned = false
			curWidth = nil
			p.advance()

			if p.cur().Type == lexer.TokenWire || p.cur().Type == lexer.TokenReg {
				curNetType = p.cur().Literal
				p.advance()
			}
			if p.cur().Type == lexer.TokenSigned {
				curSigned = true
				p.advance()
			}
			if p.cur().Type == lexer.TokenLBracket {
				curWidth = p.parseBitRange(mod)
			}
		} else if curDir != "" {
			// Subsequent port in ANSI port list might override netType/signed/width or inherit curDir.
			if p.cur().Type == lexer.TokenWire || p.cur().Type == lexer.TokenReg {
				curNetType = p.cur().Literal
				curSigned = false
				curWidth = nil
				p.advance()
			}
			if p.cur().Type == lexer.TokenSigned {
				curSigned = true
				p.advance()
			}
			if p.cur().Type == lexer.TokenLBracket {
				curWidth = p.parseBitRange(mod)
			}
		}

		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected port identifier")
			p.syncToParamOrPortDelimiter()
			if p.cur().Type == lexer.TokenComma {
				p.advance()
			}
			continue
		}

		nameTok := p.advance()
		endPos := nameTok.Range.End

		// Optional default/initial value or unpacked array dimension in port list
		if p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}

		if curDir != "" {
			port := &ast.PortDecl{
				Direction: curDir,
				NetType:   curNetType,
				Signed:    curSigned,
				Width:     curWidth,
				Name:      nameTok.Literal,
				NameRange: nameTok.Range,
				Range:     ast.Range{File: nameTok.Range.File, Start: startPos, End: endPos},
			}
			mod.Ports = append(mod.Ports, port)
			mod.Items = append(mod.Items, port)
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		if p.cur().Type != lexer.TokenRParen {
			p.addError(p.cur().Range, fmt.Sprintf("expected ',' or ')' in port list, got %q", p.cur().Literal))
			p.syncToParamOrPortDelimiter()
			if p.cur().Type == lexer.TokenComma {
				p.advance()
			}
		}
	}

	if p.cur().Type == lexer.TokenRParen {
		p.advance()
	} else {
		p.addError(p.prev().Range, "expected ')' to close port list")
	}
}

func (p *Parser) parseModuleItem(mod *ast.ModuleDecl) {
	if p.isAttrStart() {
		p.skipAttributes()
		if p.isEOF() || p.cur().Type == lexer.TokenEndmodule || p.cur().Type == lexer.TokenModule || p.cur().Type == lexer.TokenEndgenerate || p.cur().Type == lexer.TokenEnd {
			return
		}
	}
	tok := p.cur()
	switch tok.Type {
	case lexer.TokenIllegal:
		p.addError(tok.Range, tok.Literal)
		p.advance()
	case lexer.TokenSemicolon:
		p.advance()
	case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout:
		p.parsePortDeclItem(mod)
	case lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger, lexer.TokenGenvar:
		p.parseNetDeclItem(mod)
	case lexer.TokenParameter, lexer.TokenLocalparam:
		p.parseParamDeclItem(mod)
	case lexer.TokenAssign:
		p.parseAssignStmt(mod)
	case lexer.TokenAlways, lexer.TokenInitial:
		p.parseProceduralBlock(mod)
	case lexer.TokenFunction:
		p.parseFunctionDecl(mod)
	case lexer.TokenTask:
		p.parseTaskDecl(mod)
	case lexer.TokenGenerate:
		p.parseGenerateRegion(mod)
	case lexer.TokenFor, lexer.TokenIf, lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez, lexer.TokenBegin:
		p.parseGenerateConstruct(mod)
	case lexer.TokenIdent:
		p.parseModuleInstOrUnknown(mod)
	default:
		p.addError(tok.Range, fmt.Sprintf("unexpected token %q in module body", tok.Literal))
		p.advance()
		p.syncToStatementEnd()
	}
}

func (p *Parser) parsePortDeclItem(mod *ast.ModuleDecl) {
	startTok := p.advance() // input / output / inout
	dir := startTok.Literal

	netType := ""
	if p.cur().Type == lexer.TokenWire || p.cur().Type == lexer.TokenReg {
		netType = p.cur().Literal
		p.advance()
	}

	signed := false
	if p.cur().Type == lexer.TokenSigned {
		signed = true
		p.advance()
	}

	var width *ast.BitRange
	if p.cur().Type == lexer.TokenLBracket {
		width = p.parseBitRange(mod)
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected port identifier after %q", dir))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		port := &ast.PortDecl{
			Direction: dir,
			NetType:   netType,
			Signed:    signed,
			Width:     width,
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
		}
		mod.Ports = append(mod.Ports, port)
		mod.Items = append(mod.Items, port)

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon("port declaration")
}

func (p *Parser) parseNetDeclItem(mod *ast.ModuleDecl) {
	startTok := p.advance() // wire / reg / integer
	kind := startTok.Literal

	signed := false
	if p.cur().Type == lexer.TokenSigned {
		signed = true
		p.advance()
	}

	var width *ast.BitRange
	if p.cur().Type == lexer.TokenLBracket {
		width = p.parseBitRange(mod)
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected signal identifier after %q", kind))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		// Optional unpacked array dimensions: e.g. reg [7:0] mem [0:255];
		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}

		// Optional inline initialization: e.g. wire a = b;
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}

		decl := &ast.NetDecl{
			Kind:      kind,
			Signed:    signed,
			Width:     width,
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
		}
		mod.Items = append(mod.Items, decl)

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon(kind + " declaration")
}

func (p *Parser) parseParamDeclItem(mod *ast.ModuleDecl) {
	startTok := p.advance() // parameter / localparam
	kind := startTok.Literal

	signed := false
	if p.cur().Type == lexer.TokenSigned {
		signed = true
		p.advance()
	}

	var width *ast.BitRange
	if p.cur().Type == lexer.TokenLBracket {
		width = p.parseBitRange(mod)
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected identifier after %q", kind))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		valText := ""
		endPos := nameTok.Range.End

		if p.cur().Type == lexer.TokenAssignOp {
			p.advance() // '='
			valText, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
			if valText == "" {
				p.addError(p.cur().Range, fmt.Sprintf("expected value expression for %s %q", kind, nameTok.Literal))
			}
		} else {
			p.addError(nameTok.Range, fmt.Sprintf("expected '=' and value in %s declaration %q", kind, nameTok.Literal))
		}

		param := &ast.ParamDecl{
			Kind:      kind,
			Signed:    signed,
			Width:     width,
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			ValueText: valText,
			Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
		}
		mod.Params = append(mod.Params, param)
		mod.Items = append(mod.Items, param)

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon(kind + " declaration")
}

func (p *Parser) parseAssignStmt(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'assign'
	stmt := &ast.AssignStmt{
		Range: ast.Range{Start: startTok.Range.Start, End: startTok.Range.End},
	}

	// Collect LHS up to '='
	lhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenAssignOp, lexer.TokenSemicolon)
	if len(lhsTokens) == 0 {
		p.addError(startTok.Range, "expected target signal after 'assign'")
	}

	if p.cur().Type == lexer.TokenAssignOp {
		eqTok := p.advance() // '='
		rhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenSemicolon)
		if len(rhsTokens) == 0 {
			p.addError(eqTok.Range, "expected expression after '=' in assign statement")
		}
	} else {
		p.addError(p.cur().Range, "expected '=' in assign statement")
	}

	if p.cur().Type == lexer.TokenSemicolon {
		semi := p.advance()
		stmt.Range.End = semi.Range.End
	} else {
		p.addError(p.prev().Range, "expected ';' after assign statement")
		stmt.Range.End = p.prev().Range.End
	}

	mod.Items = append(mod.Items, stmt)
}

func (p *Parser) parseProceduralBlock(mod *ast.ModuleDecl) {
	startTok := p.advance() // 'always' or 'initial'
	block := &ast.ProceduralBlock{
		Kind:  startTok.Literal,
		Range: ast.Range{Start: startTok.Range.Start, End: startTok.Range.End},
	}

	// Optional sensitivity list: @(...) or @*
	if p.cur().Type == lexer.TokenAt {
		p.advance() // '@'
		if p.cur().Type == lexer.TokenStar {
			p.advance()
		} else if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
		} else {
			p.addError(p.cur().Range, "expected sensitivity list after '@'")
		}
	}

	p.parseStatement(block, mod)
	block.Range.End = p.prev().Range.End
	mod.Items = append(mod.Items, block)
}

func (p *Parser) parseFunctionDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'function'
	fn := &ast.FunctionDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenAutomatic {
		fn.Automatic = true
		p.advance()
	}

	if p.cur().Type == lexer.TokenSigned {
		fn.Signed = true
		p.advance()
	}

	if p.cur().Type == lexer.TokenInteger || p.cur().Type == lexer.TokenReg {
		if p.cur().Type == lexer.TokenInteger {
			fn.ReturnType = p.cur().Literal
		}
		p.advance()
		if p.cur().Type == lexer.TokenSigned {
			fn.Signed = true
			p.advance()
		}
	}

	if p.cur().Type == lexer.TokenLBracket {
		fn.Width = p.parseBitRange(mod)
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		fn.Name = nameTok.Literal
		fn.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected function name")
	}

	tempMod := &ast.ModuleDecl{}

	// Optional ANSI formal argument list: (input ...)
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(tempMod)
	}

	p.expectSemicolon("function header")

	// Parse function item declarations (input, reg, integer, parameter, localparam) and statements.
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndfunction &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenModule {
		if p.isAttrStart() {
			p.skipAttributes()
			continue
		}
		switch p.cur().Type {
		case lexer.TokenIllegal:
			p.addError(p.cur().Range, p.cur().Literal)
			p.advance()
		case lexer.TokenSemicolon:
			p.advance()
		case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout:
			p.parsePortDeclItem(tempMod)
		case lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger:
			p.parseNetDeclItem(tempMod)
		case lexer.TokenParameter, lexer.TokenLocalparam:
			p.parseParamDeclItem(tempMod)
		default:
			block := &ast.ProceduralBlock{
				Kind:  "function",
				Range: ast.Range{File: p.cur().Range.File, Start: p.cur().Range.Start, End: p.cur().Range.End},
			}
			p.parseStatement(block, tempMod)
		}
	}

	if p.cur().Type == lexer.TokenEndfunction {
		endTok := p.advance()
		fn.Range.End = endTok.Range.End
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected 'endfunction' to close function %q", fn.Name))
		fn.Range.End = p.prev().Range.End
	}

	fn.Params = tempMod.Ports
	for _, item := range tempMod.Items {
		if _, isPort := item.(*ast.PortDecl); !isPort {
			fn.Locals = append(fn.Locals, item)
		}
	}
	fn.Refs = tempMod.Refs
	mod.Items = append(mod.Items, fn)
}

func (p *Parser) parseTaskDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'task'
	task := &ast.TaskDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenAutomatic {
		task.Automatic = true
		p.advance()
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		task.Name = nameTok.Literal
		task.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected task name")
	}

	tempMod := &ast.ModuleDecl{}

	// Optional ANSI formal argument list: (input ..., output ...)
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(tempMod)
	}

	p.expectSemicolon("task header")

	// Parse task item declarations (input, output, inout, reg, integer, parameter, localparam) and statements.
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndtask &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenModule {
		if p.isAttrStart() {
			p.skipAttributes()
			continue
		}
		switch p.cur().Type {
		case lexer.TokenIllegal:
			p.addError(p.cur().Range, p.cur().Literal)
			p.advance()
		case lexer.TokenSemicolon:
			p.advance()
		case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout:
			p.parsePortDeclItem(tempMod)
		case lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger:
			p.parseNetDeclItem(tempMod)
		case lexer.TokenParameter, lexer.TokenLocalparam:
			p.parseParamDeclItem(tempMod)
		default:
			block := &ast.ProceduralBlock{
				Kind:  "task",
				Range: ast.Range{File: p.cur().Range.File, Start: p.cur().Range.Start, End: p.cur().Range.End},
			}
			p.parseStatement(block, tempMod)
		}
	}

	if p.cur().Type == lexer.TokenEndtask {
		endTok := p.advance()
		task.Range.End = endTok.Range.End
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected 'endtask' to close task %q", task.Name))
		task.Range.End = p.prev().Range.End
	}

	task.Params = tempMod.Ports
	for _, item := range tempMod.Items {
		if _, isPort := item.(*ast.PortDecl); !isPort {
			task.Locals = append(task.Locals, item)
		}
	}
	task.Refs = tempMod.Refs
	mod.Items = append(mod.Items, task)
}

func (p *Parser) parseGenerateRegion(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'generate'
	gen := &ast.GenerateBlock{
		Kind:  "generate",
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	subMod := &ast.ModuleDecl{}
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndgenerate &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenModule {
		p.parseModuleItem(subMod)
	}

	if p.cur().Type == lexer.TokenEndgenerate {
		endTok := p.advance()
		gen.Range.End = endTok.Range.End
	} else {
		p.addError(startTok.Range, "expected 'endgenerate' to close 'generate' block")
		gen.Range.End = p.prev().Range.End
	}

	gen.Items = subMod.Items
	gen.Refs = subMod.Refs
	mod.Refs = append(mod.Refs, subMod.Refs...)
	mod.Items = append(mod.Items, gen)
}

func (p *Parser) parseGenerateConstruct(mod *ast.ModuleDecl) {
	if p.isEOF() {
		return
	}

	startTok := p.cur()
	gen := &ast.GenerateBlock{
		Kind:  startTok.Literal,
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}
	subMod := &ast.ModuleDecl{}

	switch startTok.Type {
	case lexer.TokenBegin:
		beginTok := p.advance() // 'begin'
		if p.cur().Type == lexer.TokenColon {
			p.advance() // ':'
			if p.cur().Type == lexer.TokenIdent {
				lbl := p.advance()
				gen.Label = lbl.Literal
			} else {
				p.addError(p.cur().Range, "expected generate block label identifier after ':'")
			}
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndgenerate &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			p.parseModuleItem(subMod)
		}

		if p.cur().Type == lexer.TokenEnd {
			p.advance()
		} else {
			p.addError(beginTok.Range, "expected 'end' to close generate 'begin' block")
		}

	case lexer.TokenFor:
		forTok := p.advance() // 'for'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParensOpts(&gen.Refs, subMod, true)
		} else {
			p.addError(forTok.Range, "expected '(' after 'for' in generate block")
		}
		if !p.isEOF() &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndgenerate &&
			p.cur().Type != lexer.TokenEndmodule {
			p.parseModuleItem(subMod)
		}

	case lexer.TokenIf:
		ifTok := p.advance() // 'if'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&gen.Refs, subMod)
		} else {
			p.addError(ifTok.Range, "expected '(' after 'if' in generate block")
		}
		if !p.isEOF() &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndgenerate &&
			p.cur().Type != lexer.TokenEndmodule {
			p.parseModuleItem(subMod)
		}
		if p.cur().Type == lexer.TokenElse {
			p.advance() // 'else'
			if !p.isEOF() &&
				p.cur().Type != lexer.TokenEnd &&
				p.cur().Type != lexer.TokenEndgenerate &&
				p.cur().Type != lexer.TokenEndmodule {
				p.parseModuleItem(subMod)
			}
		}

	case lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez:
		caseTok := p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&gen.Refs, subMod)
		} else {
			p.addError(caseTok.Range, fmt.Sprintf("expected '(' after %q in generate block", caseTok.Literal))
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEndcase &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndgenerate &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			if p.cur().Type == lexer.TokenDefault {
				p.advance()
				if p.cur().Type == lexer.TokenColon {
					p.advance()
				}
				p.parseModuleItem(subMod)
				continue
			}
			tokensBeforeColon := p.collectTokensUntil(&gen.Refs, subMod, lexer.TokenColon, lexer.TokenEndcase)
			if p.cur().Type == lexer.TokenColon {
				p.advance() // ':'
				p.parseModuleItem(subMod)
			} else if len(tokensBeforeColon) == 0 {
				break
			} else {
				p.addError(p.cur().Range, "expected ':' after generate case item expression")
				p.syncToStatementEnd()
			}
		}

		if p.cur().Type == lexer.TokenEndcase {
			p.advance()
		} else {
			p.addError(caseTok.Range, fmt.Sprintf("expected 'endcase' to close generate %q", caseTok.Literal))
		}
	}

	gen.Range.End = p.prev().Range.End
	gen.Items = subMod.Items
	gen.Refs = append(gen.Refs, subMod.Refs...)
	mod.Refs = append(mod.Refs, gen.Refs...)
	mod.Items = append(mod.Items, gen)
}

func (p *Parser) parseStatement(block *ast.ProceduralBlock, mod *ast.ModuleDecl) {
	if p.isEOF() {
		return
	}
	if p.isAttrStart() {
		p.skipAttributes()
		if p.isEOF() || p.cur().Type == lexer.TokenEnd || p.cur().Type == lexer.TokenEndcase || p.cur().Type == lexer.TokenEndfunction || p.cur().Type == lexer.TokenEndmodule {
			return
		}
	}

	switch p.cur().Type {
	case lexer.TokenSemicolon:
		p.advance()

	case lexer.TokenBegin:
		beginTok := p.advance() // 'begin'
		if p.cur().Type == lexer.TokenColon {
			p.advance() // ':'
			if p.cur().Type == lexer.TokenIdent {
				lbl := p.advance()
				if block.Label == "" {
					block.Label = lbl.Literal
				}
			} else {
				p.addError(p.cur().Range, "expected block label identifier after ':'")
			}
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndfunction &&
			p.cur().Type != lexer.TokenEndtask &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			p.parseStatement(block, mod)
		}

		if p.cur().Type == lexer.TokenEnd {
			p.advance()
		} else {
			p.addError(beginTok.Range, "expected 'end' to close 'begin' block")
		}

	case lexer.TokenIf:
		ifTok := p.advance() // 'if'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else {
			p.addError(ifTok.Range, "expected '(' after 'if'")
		}
		p.parseStatement(block, mod)
		if p.cur().Type == lexer.TokenElse {
			p.advance() // 'else'
			p.parseStatement(block, mod)
		}

	case lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez:
		caseTok := p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else {
			p.addError(caseTok.Range, fmt.Sprintf("expected '(' after %q", caseTok.Literal))
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEndcase &&
			p.cur().Type != lexer.TokenEnd &&
			p.cur().Type != lexer.TokenEndfunction &&
			p.cur().Type != lexer.TokenEndtask &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			if p.cur().Type == lexer.TokenDefault {
				p.advance()
				if p.cur().Type == lexer.TokenColon {
					p.advance()
				}
				p.parseStatement(block, mod)
				continue
			}
			// Parse case item expressions up to ':'
			tokensBeforeColon := p.collectTokensUntil(&block.Refs, mod, lexer.TokenColon, lexer.TokenEndcase)
			if p.cur().Type == lexer.TokenColon {
				p.advance() // ':'
				p.parseStatement(block, mod)
			} else if len(tokensBeforeColon) == 0 {
				break
			} else {
				p.addError(p.cur().Range, "expected ':' after case item expression")
				p.syncToStatementEnd()
			}
		}

		if p.cur().Type == lexer.TokenEndcase {
			p.advance()
		} else {
			p.addError(caseTok.Range, fmt.Sprintf("expected 'endcase' to close %q", caseTok.Literal))
		}

	case lexer.TokenFor, lexer.TokenWhile:
		loopTok := p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParensOpts(&block.Refs, mod, loopTok.Type == lexer.TokenFor)
		} else {
			p.addError(loopTok.Range, fmt.Sprintf("expected '(' after %q", loopTok.Literal))
		}
		p.parseStatement(block, mod)

	case lexer.TokenHash:
		// Delay control: #10 stmt; or #(10) stmt;
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
		}
		p.parseStatement(block, mod)

	case lexer.TokenAt:
		// Event control inside procedural block
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else if p.cur().Type == lexer.TokenIdent || p.cur().Type == lexer.TokenStar {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
		}
		p.parseStatement(block, mod)

	case lexer.TokenSystemIdent:
		// System task call: e.g. $display("...", a);
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		}
		p.expectSemicolon("system task call")

	case lexer.TokenIdent:
		// Task call statement: my_task(a, b); or my_task;
		if p.peekN(1).Type == lexer.TokenLParen {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
			p.parseBalancedParens(&block.Refs, mod)
			p.expectSemicolon("task call")
			return
		}
		if p.peekN(1).Type == lexer.TokenSemicolon {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
			p.advance() // ';'
			return
		}
		fallthrough

	case lexer.TokenLBrace:
		// Procedural assignment: a = b; or a <= b; or {a, b} <= c;
		p.collectTokensUntil(&block.Refs, mod, lexer.TokenAssignOp, lexer.TokenLeq, lexer.TokenSemicolon)
		if p.cur().Type == lexer.TokenAssignOp || p.cur().Type == lexer.TokenLeq {
			opTok := p.advance()
			rhs := p.collectTokensUntil(&block.Refs, mod, lexer.TokenSemicolon)
			if len(rhs) == 0 {
				p.addError(opTok.Range, fmt.Sprintf("expected expression after %q", opTok.Literal))
			}
			p.expectSemicolon("procedural assignment")
		} else {
			p.addError(p.cur().Range, "expected '=' or '<=' in procedural assignment")
			p.syncToStatementEnd()
		}

	default:
		p.addError(p.cur().Range, fmt.Sprintf("unexpected token %q in statement", p.cur().Literal))
		p.advance()
		p.syncToStatementEnd()
	}
}

func (p *Parser) parseModuleInstOrUnknown(mod *ast.ModuleDecl) {
	modNameTok := p.advance() // module type name

	var paramRefs []ast.IdentifierRef
	// Optional parameter value assignment: #(...) or #VALUE
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseInstConnections(&paramRefs, mod)
		} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			if tok.Type == lexer.TokenIdent {
				p.recordRef(&paramRefs, mod, tok)
			}
		} else {
			p.addError(p.cur().Range, "expected '(' after '#' in module instantiation")
		}
	}

	if p.cur().Type != lexer.TokenIdent {
		p.addError(modNameTok.Range, fmt.Sprintf("unexpected identifier %q (expected module instantiation or valid keyword)", modNameTok.Literal))
		p.syncToStatementEnd()
		return
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected instance name in module instantiation")
			p.syncToStatementEnd()
			return
		}
		instNameTok := p.advance()
		inst := &ast.ModuleInst{
			ModuleName:        modNameTok.Literal,
			ModuleNameRange:   modNameTok.Range,
			InstanceName:      instNameTok.Literal,
			InstanceNameRange: instNameTok.Range,
			Range:             ast.Range{File: modNameTok.Range.File, Start: modNameTok.Range.Start, End: instNameTok.Range.End},
			Refs:              append([]ast.IdentifierRef(nil), paramRefs...),
		}

		// Optional instance array range: e.g. sub_mod u_sub [3:0] (...);
		if p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
		}

		if p.cur().Type == lexer.TokenLParen {
			p.parseInstConnections(&inst.Refs, mod)
			inst.Range.End = p.prev().Range.End
		} else {
			p.addError(instNameTok.Range, fmt.Sprintf("expected '(' for port connections of instance %q", inst.InstanceName))
		}

		mod.Items = append(mod.Items, inst)

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	if p.cur().Type == lexer.TokenSemicolon {
		semi := p.advance()
		if len(mod.Items) > 0 {
			if lastInst, ok := mod.Items[len(mod.Items)-1].(*ast.ModuleInst); ok {
				lastInst.Range.End = semi.Range.End
			}
		}
	} else {
		p.addError(p.prev().Range, "expected ';' after module instantiation")
	}
}

// parseInstConnections parses (.port(expr), ...) or positional (expr, ...) while only recording
// local signal references (skipping the `.portName` identifier itself since that belongs to the instantiated module).
func (p *Parser) parseInstConnections(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl) {
	openTok := p.advance() // '('
	depth := 1
	for !p.isEOF() && depth > 0 {
		if p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		if p.isAttrStart() {
			p.parseAttribute()
			continue
		}
		if p.cur().Type == lexer.TokenDot && depth == 1 {
			p.advance() // '.'
			if p.cur().Type == lexer.TokenIdent {
				p.advance() // skip target module's port name
			}
			continue
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLParen:
			depth++
		case lexer.TokenRParen:
			depth--
		case lexer.TokenIdent:
			p.recordRef(refs, mod, tok)
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
		}
	}
	if depth != 0 {
		p.addError(openTok.Range, "unclosed '(' in module instantiation")
	}
}

func (p *Parser) parseBitRange(mod *ast.ModuleDecl) *ast.BitRange {
	startTok := p.advance() // '['
	var msbParts []string
	var lsbParts []string
	inLSB := false

	for !p.isEOF() && p.cur().Type != lexer.TokenRBracket {
		if p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		tok := p.advance()
		if tok.Type == lexer.TokenColon {
			inLSB = true
			continue
		}
		if tok.Type == lexer.TokenIdent {
			p.recordRef(nil, mod, tok)
		}
		if inLSB {
			lsbParts = append(lsbParts, tok.Literal)
		} else {
			msbParts = append(msbParts, tok.Literal)
		}
	}

	endPos := p.prev().Range.End
	if p.cur().Type == lexer.TokenRBracket {
		rbr := p.advance()
		endPos = rbr.Range.End
	} else {
		p.addError(startTok.Range, "expected ']' to close bit range")
	}

	msb := strings.Join(msbParts, "")
	lsb := strings.Join(lsbParts, "")
	raw := "[" + msb + "]"
	if inLSB {
		raw = "[" + msb + ":" + lsb + "]"
	}

	return &ast.BitRange{
		MSB:   msb,
		LSB:   lsb,
		Raw:   raw,
		Range: ast.Range{Start: startTok.Range.Start, End: endPos},
	}
}

func (p *Parser) parseExprUntil(mod *ast.ModuleDecl, stopTokens ...lexer.TokenType) (string, ast.Position) {
	var parts []string
	endPos := p.prev().Range.End
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0

	for !p.isEOF() {
		tok := p.cur()
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			for _, stop := range stopTokens {
				if tok.Type == stop {
					return strings.Join(parts, " "), endPos
				}
			}
			if tok.Type == lexer.TokenSemicolon || tok.Type == lexer.TokenEndmodule {
				return strings.Join(parts, " "), endPos
			}
		}
		if p.isAttrStart() {
			p.parseAttribute()
			endPos = p.prev().Range.End
			continue
		}

		p.advance()
		endPos = tok.Range.End
		parts = append(parts, tok.Literal)

		switch tok.Type {
		case lexer.TokenLParen:
			parenDepth++
		case lexer.TokenRParen:
			if parenDepth > 0 {
				parenDepth--
			}
		case lexer.TokenLBracket:
			bracketDepth++
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case lexer.TokenLBrace:
			braceDepth++
		case lexer.TokenRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.TokenIdent:
			p.recordRef(nil, mod, tok)
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
		}
	}

	return strings.Join(parts, " "), endPos
}

func (p *Parser) collectTokensUntil(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl, stopTokens ...lexer.TokenType) []lexer.Token {
	var collected []lexer.Token
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0

	for !p.isEOF() {
		tok := p.cur()
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			for _, stop := range stopTokens {
				if tok.Type == stop {
					return collected
				}
			}
			if isStatementBoundary(tok.Type) {
				return collected
			}
		}
		if p.isAttrStart() {
			p.parseAttribute()
			continue
		}

		p.advance()
		collected = append(collected, tok)

		switch tok.Type {
		case lexer.TokenLParen:
			parenDepth++
		case lexer.TokenRParen:
			if parenDepth > 0 {
				parenDepth--
			}
		case lexer.TokenLBracket:
			bracketDepth++
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case lexer.TokenLBrace:
			braceDepth++
		case lexer.TokenRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.TokenIdent:
			p.recordRef(refs, mod, tok)
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
		}
	}

	return collected
}

func (p *Parser) parseBalancedParens(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl) {
	p.parseBalancedParensOpts(refs, mod, false)
}

func (p *Parser) parseBalancedParensOpts(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl, allowSemicolon bool) {
	openTok := p.advance() // '('
	depth := 1
	for !p.isEOF() && depth > 0 {
		if (!allowSemicolon && p.cur().Type == lexer.TokenSemicolon) ||
			p.cur().Type == lexer.TokenEndmodule ||
			p.cur().Type == lexer.TokenEndfunction ||
			p.cur().Type == lexer.TokenEndtask ||
			p.cur().Type == lexer.TokenEndgenerate {
			break
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLParen:
			depth++
		case lexer.TokenRParen:
			depth--
		case lexer.TokenGenvar, lexer.TokenInteger:
			if allowSemicolon && mod != nil && p.cur().Type == lexer.TokenIdent {
				nameTok := p.advance()
				mod.Items = append(mod.Items, &ast.NetDecl{
					Kind:      tok.Literal,
					Name:      nameTok.Literal,
					NameRange: nameTok.Range,
					Range:     ast.Range{File: tok.Range.File, Start: tok.Range.Start, End: nameTok.Range.End},
				})
			}
		case lexer.TokenIdent:
			p.recordRef(refs, mod, tok)
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
		}
	}
	if depth != 0 {
		p.addError(openTok.Range, "unclosed '('")
	}
}

func (p *Parser) recordRef(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl, tok lexer.Token) {
	ref := ast.IdentifierRef{
		Name:  tok.Literal,
		Range: tok.Range,
	}
	if itemRefs != nil {
		*itemRefs = append(*itemRefs, ref)
	}
	if mod != nil {
		mod.Refs = append(mod.Refs, ref)
	}
}

func (p *Parser) expectSemicolon(context string) {
	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
		return
	}
	p.addError(p.prev().Range, fmt.Sprintf("expected ';' after %s", context))
}

func (p *Parser) syncToParamOrPortDelimiter() {
	for !p.isEOF() {
		switch p.cur().Type {
		case lexer.TokenComma, lexer.TokenRParen, lexer.TokenSemicolon, lexer.TokenEndmodule:
			return
		default:
			p.advance()
		}
	}
}

func (p *Parser) syncToStatementEnd() {
	for !p.isEOF() {
		switch p.cur().Type {
		case lexer.TokenSemicolon:
			p.advance()
			return
		case lexer.TokenEnd, lexer.TokenEndcase, lexer.TokenEndfunction, lexer.TokenEndtask, lexer.TokenEndgenerate, lexer.TokenEndmodule, lexer.TokenModule,
			lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout,
			lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger, lexer.TokenGenvar,
			lexer.TokenParameter, lexer.TokenLocalparam,
			lexer.TokenAssign, lexer.TokenAlways, lexer.TokenInitial, lexer.TokenFunction, lexer.TokenTask, lexer.TokenGenerate:
			return
		default:
			p.advance()
		}
	}
}

func isPortDirection(t lexer.TokenType) bool {
	return t == lexer.TokenInput || t == lexer.TokenOutput || t == lexer.TokenInout
}

func isStatementBoundary(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenSemicolon, lexer.TokenEnd, lexer.TokenEndcase, lexer.TokenEndfunction, lexer.TokenEndtask, lexer.TokenEndgenerate, lexer.TokenEndmodule, lexer.TokenModule,
		lexer.TokenAlways, lexer.TokenInitial, lexer.TokenAssign, lexer.TokenFunction, lexer.TokenTask, lexer.TokenGenerate:
		return true
	default:
		return false
	}
}

func (p *Parser) cur() lexer.Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekN(offset int) lexer.Token {
	idx := p.pos + offset
	if idx < 0 {
		return p.tokens[0]
	}
	if idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[idx]
}

// isAttrStart reports whether the current tokens begin a Verilog-2001 attribute instance `(* ... *)`.
// It explicitly excludes `(*)` (wildcard sensitivity list in `always @(*)`).
func (p *Parser) isAttrStart() bool {
	t0 := p.cur()
	t1 := p.peekN(1)
	if t0.Type != lexer.TokenLParen || t1.Type != lexer.TokenStar {
		return false
	}
	if t0.Range.End.Line != t1.Range.Start.Line || t0.Range.End.Column != t1.Range.Start.Column {
		return false
	}
	// `(*)` is a wildcard sensitivity list, not an attribute instance.
	t2 := p.peekN(2)
	if t2.Type == lexer.TokenRParen &&
		t1.Range.End.Line == t2.Range.Start.Line &&
		t1.Range.End.Column == t2.Range.Start.Column {
		return false
	}
	return true
}

func (p *Parser) isAttrEnd() bool {
	t0 := p.cur()
	t1 := p.peekN(1)
	return t0.Type == lexer.TokenStar &&
		t1.Type == lexer.TokenRParen &&
		t0.Range.End.Line == t1.Range.Start.Line &&
		t0.Range.End.Column == t1.Range.Start.Column
}

func (p *Parser) parseAttribute() {
	openTok := p.advance() // '('
	p.advance()            // '*'

	for !p.isEOF() {
		if p.isAttrEnd() {
			p.advance() // '*'
			p.advance() // ')'
			return
		}
		if p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		if p.cur().Type == lexer.TokenIllegal {
			p.addError(p.cur().Range, p.cur().Literal)
		}
		p.advance()
	}

	p.addError(openTok.Range, "unclosed attribute '(*'")
}

func (p *Parser) skipAttributes() {
	for p.isAttrStart() {
		p.parseAttribute()
	}
}

func (p *Parser) prev() lexer.Token {
	if p.pos == 0 {
		return p.tokens[0]
	}
	return p.tokens[p.pos-1]
}

func (p *Parser) advance() lexer.Token {
	tok := p.cur()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

func (p *Parser) isEOF() bool {
	return p.cur().Type == lexer.TokenEOF
}

func (p *Parser) addError(rng ast.Range, msg string) {
	if rng.File != "" {
		msg = fmt.Sprintf("in included file %q (%d:%d): %s",
			filepath.Base(rng.File), rng.Start.Line+1, rng.Start.Column+1, msg)
		if origin, ok := p.fileOrigin[rng.File]; ok {
			rng = origin
		}
	}
	p.errors = append(p.errors, ParseError{
		Range:   rng,
		Message: msg,
	})
}
