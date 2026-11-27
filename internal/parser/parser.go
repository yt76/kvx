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
	primitives map[string]bool
	typeNames  map[string]bool
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
		primitives: make(map[string]bool),
		typeNames:  make(map[string]bool),
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
		if tok.Type == lexer.TokenModule || tok.Type == lexer.TokenMacromodule || tok.Type == lexer.TokenProgram || tok.Type == lexer.TokenChecker {
			if mod := p.parseModuleDecl(); mod != nil {
				file.Modules = append(file.Modules, mod)
			}
			continue
		}
		if tok.Type == lexer.TokenInterface {
			if iface := p.parseInterfaceDecl(); iface != nil {
				file.Interfaces = append(file.Interfaces, iface)
			}
			continue
		}
		if tok.Type == lexer.TokenPackage {
			if pkg := p.parsePackageDecl(); pkg != nil {
				file.Packages = append(file.Packages, pkg)
			}
			continue
		}
		if tok.Type == lexer.TokenClass || (tok.Type == lexer.TokenVirtual && p.peekN(1).Type == lexer.TokenClass) {
			if cls := p.parseClassDecl(); cls != nil {
				file.Classes = append(file.Classes, cls)
			}
			continue
		}
		if tok.Type == lexer.TokenFunction {
			dummyMod := &ast.ModuleDecl{}
			p.parseFunctionDecl(dummyMod)
			for _, item := range dummyMod.Items {
				if fn, ok := item.(*ast.FunctionDecl); ok {
					file.Functions = append(file.Functions, fn)
				}
			}
			continue
		}
		if tok.Type == lexer.TokenTask {
			dummyMod := &ast.ModuleDecl{}
			p.parseTaskDecl(dummyMod)
			for _, item := range dummyMod.Items {
				if tk, ok := item.(*ast.TaskDecl); ok {
					file.Tasks = append(file.Tasks, tk)
				}
			}
			continue
		}
		if tok.Type == lexer.TokenExport {
			p.parseDPIExportDecl(nil, file)
			continue
		}
		if tok.Type == lexer.TokenLet {
			p.parseLetDecl(nil, file)
			continue
		}
		if tok.Type == lexer.TokenConstraint {
			dummyMod := &ast.ModuleDecl{}
			p.parseConstraintDecl(dummyMod)
			continue
		}
		if tok.Type == lexer.TokenCovergroup {
			dummyMod := &ast.ModuleDecl{}
			p.parseCovergroupDecl(dummyMod)
			continue
		}
		if tok.Type == lexer.TokenBind {
			p.parseBindStmt(nil, file)
			continue
		}
		if tok.Type == lexer.TokenImport {
			p.parseImportDecl(nil, file)
			continue
		}
		if tok.Type == lexer.TokenPrimitive {
			p.parsePrimitiveDecl()
			continue
		}
		if tok.Type == lexer.TokenConfig {
			if cfg := p.parseConfigDecl(); cfg != nil {
				file.Configs = append(file.Configs, cfg)
			}
			continue
		}
		if tok.Type == lexer.TokenTypedef || tok.Type == lexer.TokenNettype {
			p.parseTypedefDecl(nil, file)
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
			p.cur().Type != lexer.TokenMacromodule &&
			p.cur().Type != lexer.TokenProgram &&
			p.cur().Type != lexer.TokenChecker &&
			p.cur().Type != lexer.TokenLet &&
			p.cur().Type != lexer.TokenInterface &&
			p.cur().Type != lexer.TokenPackage &&
			p.cur().Type != lexer.TokenClass &&
			p.cur().Type != lexer.TokenVirtual &&
			p.cur().Type != lexer.TokenBind &&
			p.cur().Type != lexer.TokenImport &&
			p.cur().Type != lexer.TokenExport &&
			p.cur().Type != lexer.TokenPrimitive &&
			p.cur().Type != lexer.TokenConfig &&
			p.cur().Type != lexer.TokenTypedef &&
			p.cur().Type != lexer.TokenNettype &&
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
	startTok := p.advance() // consume 'module', 'macromodule', or 'program'
	isProg := startTok.Type == lexer.TokenProgram
	isChecker := startTok.Type == lexer.TokenChecker
	endKw := lexer.TokenEndmodule
	kwName := "module"
	endKwName := "endmodule"
	if isProg {
		endKw = lexer.TokenEndprogram
		kwName = "program"
		endKwName = "endprogram"
	} else if isChecker {
		endKw = lexer.TokenEndchecker
		kwName = "checker"
		endKwName = "endchecker"
	}
	if p.cur().Type == lexer.TokenAutomatic || p.cur().Type == lexer.TokenStatic {
		p.advance()
	}
	mod := &ast.ModuleDecl{
		IsProgram: isProg,
		IsChecker: isChecker,
		Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		mod.Name = nameTok.Literal
		mod.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, fmt.Sprintf("expected %s name after '%s'", kwName, kwName))
	}

	// Optional SystemVerilog header package imports: `module top import pkg::*; #(...) (...);`
	for p.cur().Type == lexer.TokenImport {
		p.parseImportDecl(mod, nil)
	}

	// Optional parameter port list: #(...)
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseModuleParamPortList(mod)
		} else {
			p.addError(p.cur().Range, fmt.Sprintf("expected '(' after '#' in %s header", kwName))
		}
	}

	// Optional port list: (...)
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(mod)
	}

	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected ';' after %s header", kwName))
	}

	// Parse body items until closing keyword, 'module', 'interface', 'package', 'primitive', or EOF.
	for !p.isEOF() &&
		p.cur().Type != endKw &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenEndprogram &&
		p.cur().Type != lexer.TokenEndchecker &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage &&
		p.cur().Type != lexer.TokenPrimitive {
		p.parseModuleItem(mod)
	}

	if p.cur().Type == endKw {
		p.advance()
		p.consumeOptionalEndLabel()
		mod.Range.End = p.prev().Range.End
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected '%s' to close %s %q", endKwName, kwName, mod.Name))
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

		isTypeParam := false
		if p.cur().Type == lexer.TokenTypeKw {
			isTypeParam = true
			p.advance()
		} else if isParamTypeKeyword(p.cur().Type) {
			p.advance()
		} else if p.isUserTypeBeforeIdent() {
			_, _ = p.parseScopedTypeName(nil, mod)
		}

		signed := false
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			signed = p.cur().Type == lexer.TokenSigned
			p.advance()
		}

		var width *ast.BitRange
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if width == nil {
				width = r
			}
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
		if isTypeParam && p.typeNames != nil {
			p.typeNames[nameTok.Literal] = true
		}
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
	var curTypeArgs []ast.TypeParamArg
	var curIfName string
	var curIfRange ast.Range
	var curModportName string
	var curModportRange ast.Range
	var curSigned bool
	var curWidth *ast.BitRange

	isPortType := func(t lexer.TokenType) bool {
		if isPortNetType(t) || t == lexer.TokenUntyped {
			return true
		}
		if mod != nil && mod.IsChecker {
			return t == lexer.TokenEvent || t == lexer.TokenProperty || t == lexer.TokenSequence
		}
		return false
	}

	for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
		p.skipAttributes()
		if p.cur().Type == lexer.TokenRParen {
			break
		}
		// Allow empty port slots in non-ANSI port lists: e.g. module m(a, , b);
		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		// Allow explicit port expressions in non-ANSI port lists: e.g. module m(.a(x), .b({y, z}));
		if p.cur().Type == lexer.TokenDot {
			p.advance() // '.'
			if p.cur().Type == lexer.TokenIdent {
				p.advance()
			}
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(nil, mod)
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
			}
			continue
		}
		startPos := p.cur().Range.Start

		if p.cur().Type == lexer.TokenConst && isPortDirection(p.peekN(1).Type) {
			p.advance() // 'const' before 'ref' / direction
		}
		if isPortDirection(p.cur().Type) {
			curDir = p.cur().Literal
			curNetType = ""
			curIfName = ""
			curIfRange = ast.Range{}
			curModportName = ""
			curModportRange = ast.Range{}
			curSigned = false
			curWidth = nil
			p.advance()

			for p.cur().Type == lexer.TokenVar || p.cur().Type == lexer.TokenConst {
				p.advance()
			}

			if isPortType(p.cur().Type) {
				curNetType = p.cur().Literal
				p.advance()
				// Allow `input wire logic [7:0] a`
				if isSVDataType(p.cur().Type) {
					p.advance()
				}
			} else if p.isUserTypeBeforeIdent() {
				curNetType, _ = p.parseScopedTypeName(nil, mod)
				if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
					curTypeArgs = p.parseTypeParamArgs(nil, mod)
				} else {
					curTypeArgs = nil
				}
			}
			if p.cur().Type == lexer.TokenScalared || p.cur().Type == lexer.TokenVectored {
				p.advance()
			}
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				curSigned = p.cur().Type == lexer.TokenSigned
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				r := p.parseBitRange(mod)
				if curWidth == nil {
					curWidth = r
				}
			}
		} else if p.cur().Type == lexer.TokenInterface {
			// Generic interface port: `interface bus` or `interface.master bus`
			p.advance()
			curDir = "interface"
			curNetType = ""
			curIfName = ""
			curIfRange = ast.Range{}
			curModportName = ""
			curModportRange = ast.Range{}
			curSigned = false
			curWidth = nil
			if p.cur().Type == lexer.TokenDot && p.peekN(1).Type == lexer.TokenIdent {
				p.advance() // '.'
				mpTok := p.advance()
				curModportName = mpTok.Literal
				curModportRange = mpTok.Range
				curNetType = "." + mpTok.Literal
			}
		} else if p.cur().Type == lexer.TokenIdent &&
			p.peekN(1).Type == lexer.TokenDot &&
			p.peekN(2).Type == lexer.TokenIdent &&
			p.peekN(3).Type == lexer.TokenIdent {
			// Typed interface port with modport: `axi_if.master bus`
			ifTok := p.advance()
			p.advance() // '.'
			mpTok := p.advance()
			curDir = "interface"
			curIfName = ifTok.Literal
			curIfRange = ifTok.Range
			curModportName = mpTok.Literal
			curModportRange = mpTok.Range
			curNetType = ifTok.Literal + "." + mpTok.Literal
			curSigned = false
			curWidth = nil
		} else if (curDir == "" || curDir == "interface") &&
			p.cur().Type == lexer.TokenIdent &&
			p.peekN(1).Type == lexer.TokenIdent {
			// Typed interface port without modport: `axi_if bus`
			ifTok := p.advance()
			curDir = "interface"
			curIfName = ifTok.Literal
			curIfRange = ifTok.Range
			curModportName = ""
			curModportRange = ast.Range{}
			curNetType = ifTok.Literal
			curSigned = false
			curWidth = nil
		} else if curDir == "" && (isPortType(p.cur().Type) || p.cur().Type == lexer.TokenConst || p.cur().Type == lexer.TokenVar) {
			// Directionless typed argument (common in SV functions/tasks, checkers, and DPI-C, e.g. `(int a, chandle p)`): defaults to `input`.
			curDir = "input"
			curNetType = ""
			curIfName = ""
			curIfRange = ast.Range{}
			curModportName = ""
			curModportRange = ast.Range{}
			curSigned = false
			curWidth = nil
			for p.cur().Type == lexer.TokenConst || p.cur().Type == lexer.TokenVar {
				p.advance()
			}
			if isPortDirection(p.cur().Type) {
				curDir = p.advance().Literal
			}
			if isPortType(p.cur().Type) {
				curNetType = p.advance().Literal
				if isSVDataType(p.cur().Type) {
					p.advance()
				}
			} else if p.isUserTypeBeforeIdent() {
				curNetType, _ = p.parseScopedTypeName(nil, mod)
				if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
					curTypeArgs = p.parseTypeParamArgs(nil, mod)
				} else {
					curTypeArgs = nil
				}
			}
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				curSigned = p.cur().Type == lexer.TokenSigned
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				r := p.parseBitRange(mod)
				if curWidth == nil {
					curWidth = r
				}
			}
		} else if curDir != "" {
			// Subsequent port in ANSI port list might override netType/signed/width or inherit curDir.
			if isPortType(p.cur().Type) {
				curNetType = p.cur().Literal
				curIfName = ""
				curIfRange = ast.Range{}
				curModportName = ""
				curModportRange = ast.Range{}
				curSigned = false
				curWidth = nil
				p.advance()
				if isSVDataType(p.cur().Type) {
					p.advance()
				}
			} else if p.isUserTypeBeforeIdent() {
				curNetType, _ = p.parseScopedTypeName(nil, mod)
				curIfName = ""
				curIfRange = ast.Range{}
				curModportName = ""
				curModportRange = ast.Range{}
				curSigned = false
				curWidth = nil
			}
			if p.cur().Type == lexer.TokenScalared || p.cur().Type == lexer.TokenVectored {
				p.advance()
			}
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				curSigned = p.cur().Type == lexer.TokenSigned
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				r := p.parseBitRange(mod)
				if curWidth == nil {
					curWidth = r
				}
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

		// Optional unpacked array dimensions in port list
		var unpackedDims []*ast.BitRange
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if r != nil {
				unpackedDims = append(unpackedDims, r)
			}
			endPos = p.prev().Range.End
		}

		// Optional inline initialization: e.g. output reg [7:0] q = 8'h00
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance() // '='
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenRParen)
		}

		portDir := curDir
		portNetType := curNetType
		if portDir == "" && mod != nil && mod.IsChecker {
			portDir = "input"
			if portNetType == "" {
				portNetType = "untyped"
			}
		}
		if portDir != "" {
			port := &ast.PortDecl{
				Direction:          portDir,
				NetType:            portNetType,
				TypeArgs:           curTypeArgs,
				InterfaceName:      curIfName,
				InterfaceNameRange: curIfRange,
				ModportName:        curModportName,
				ModportNameRange:   curModportRange,
				Signed:             curSigned,
				Width:              curWidth,
				UnpackedDims:       unpackedDims,
				Name:               nameTok.Literal,
				NameRange:          nameTok.Range,
				Range:              ast.Range{File: nameTok.Range.File, Start: startPos, End: endPos},
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
		if p.isEOF() || p.cur().Type == lexer.TokenEndmodule || p.cur().Type == lexer.TokenEndprogram || p.cur().Type == lexer.TokenEndchecker || p.cur().Type == lexer.TokenEndinterface || p.cur().Type == lexer.TokenModule || p.cur().Type == lexer.TokenEndgenerate || p.cur().Type == lexer.TokenEnd {
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
	case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout, lexer.TokenRef:
		p.parsePortDeclItem(mod)
	case lexer.TokenModport:
		p.parseModportDecl(mod)
	case lexer.TokenClocking:
		p.parseClockingDecl(mod, false, false, p.cur().Range.Start)
	case lexer.TokenGlobal:
		startPos := p.advance().Range.Start
		if p.cur().Type == lexer.TokenClocking {
			p.parseClockingDecl(mod, false, true, startPos)
		} else {
			p.addError(p.cur().Range, "expected 'clocking' after 'global'")
			p.syncToStatementEnd()
		}
	case lexer.TokenDefault:
		startPos := p.advance().Range.Start
		if p.cur().Type == lexer.TokenClocking {
			p.parseClockingDecl(mod, true, false, startPos)
		} else {
			p.addError(p.cur().Range, "expected 'clocking' after 'default' in module body")
			p.syncToStatementEnd()
		}
	case lexer.TokenProgram:
		if prog := p.parseModuleDecl(); prog != nil {
			mod.Items = append(mod.Items, prog)
		}
	case lexer.TokenWire, lexer.TokenUwire, lexer.TokenInterconnect, lexer.TokenReg,
		lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
		lexer.TokenInteger, lexer.TokenGenvar,
		lexer.TokenTri, lexer.TokenTri0, lexer.TokenTri1, lexer.TokenTriand, lexer.TokenTrior, lexer.TokenTrireg,
		lexer.TokenSupply0, lexer.TokenSupply1, lexer.TokenWand, lexer.TokenWor,
		lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent:
		p.parseNetDeclItem(mod)
	case lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenAutomatic,
		lexer.TokenLocal, lexer.TokenProtected, lexer.TokenRand, lexer.TokenRandc,
		lexer.TokenVirtual, lexer.TokenPure, lexer.TokenExtern:
		p.parseVarWithModifiers(mod)
	case lexer.TokenClass:
		if cls := p.parseClassDecl(); cls != nil {
			mod.Items = append(mod.Items, cls)
		}
	case lexer.TokenConstraint:
		p.parseConstraintDecl(mod)
	case lexer.TokenCovergroup:
		p.parseCovergroupDecl(mod)
	case lexer.TokenBind:
		p.parseBindStmt(mod, nil)
	case lexer.TokenChecker:
		if chk := p.parseModuleDecl(); chk != nil {
			mod.Items = append(mod.Items, chk)
		}
	case lexer.TokenLet:
		p.parseLetDecl(mod, nil)
	case lexer.TokenImport:
		p.parseImportDecl(mod, nil)
	case lexer.TokenExport:
		p.parseDPIExportDecl(mod, nil)
	case lexer.TokenTypedef, lexer.TokenNettype:
		p.parseTypedefDecl(mod, nil)
	case lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion:
		p.parseEnumOrStructVarDecl(mod)
	case lexer.TokenParameter, lexer.TokenLocalparam, lexer.TokenSpecparam:
		p.parseParamDeclItem(mod)
	case lexer.TokenDefparam:
		p.parseDefparamStmt(mod)
	case lexer.TokenAssign:
		p.parseAssignStmt(mod)
	case lexer.TokenAlways, lexer.TokenAlwaysComb, lexer.TokenAlwaysFF, lexer.TokenAlwaysLatch, lexer.TokenInitial, lexer.TokenFinal:
		p.parseProceduralBlock(mod)
	case lexer.TokenProperty, lexer.TokenSequence:
		p.parsePropertyOrSequenceDecl(mod)
	case lexer.TokenAssert, lexer.TokenAssume, lexer.TokenCover, lexer.TokenRestrict:
		p.parseAssertItem(mod, nil)
	case lexer.TokenFunction:
		p.parseFunctionDecl(mod)
	case lexer.TokenTask:
		p.parseTaskDecl(mod)
	case lexer.TokenGenerate:
		p.parseGenerateRegion(mod)
	case lexer.TokenSpecify:
		p.parseSpecifyBlock(mod)
	case lexer.TokenFor, lexer.TokenIf, lexer.TokenCase, lexer.TokenCasex, lexer.TokenCasez, lexer.TokenBegin:
		p.parseGenerateConstruct(mod)
	case lexer.TokenIdent, lexer.TokenOr:
		if tok.Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenColon && isAssertionKeyword(p.peekN(2).Type) {
			lblTok := p.advance()
			p.advance() // ':'
			p.parseAssertItem(mod, &lblTok)
		} else if tok.Type == lexer.TokenIdent && p.isUserTypeVarDecl() {
			p.parseUserTypeVarDecl(mod)
		} else {
			p.parseModuleInstOrUnknown(mod)
		}
	default:
		p.addError(tok.Range, fmt.Sprintf("unexpected token %q in module body", tok.Literal))
		p.advance()
		p.syncToStatementEnd()
	}
}

func (p *Parser) parsePortDeclItem(mod *ast.ModuleDecl) {
	startTok := p.advance() // input / output / inout / ref
	dir := startTok.Literal

	for p.cur().Type == lexer.TokenVar || p.cur().Type == lexer.TokenConst {
		p.advance()
	}

	netType := ""
	if isPortNetType(p.cur().Type) {
		netType = p.cur().Literal
		p.advance()
		if isSVDataType(p.cur().Type) {
			p.advance()
		}
	} else if p.isUserTypeBeforeIdent() {
		netType, _ = p.parseScopedTypeName(nil, mod)
	}

	if isParamTypeKeyword(p.cur().Type) {
		p.advance()
	}

	if p.cur().Type == lexer.TokenScalared || p.cur().Type == lexer.TokenVectored {
		p.advance()
	}

	signed := false
	if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
		signed = p.cur().Type == lexer.TokenSigned
		p.advance()
	}

	var width *ast.BitRange
	for p.cur().Type == lexer.TokenLBracket {
		r := p.parseBitRange(mod)
		if width == nil {
			width = r
		}
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected port identifier after %q", dir))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		// Optional unpacked array dimensions: e.g. input [7:0] arr [0:3];
		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}

		// Optional inline initialization: e.g. output reg [7:0] q = 8'h00;
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance() // '='
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}

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

	if isParamTypeKeyword(p.cur().Type) {
		p.advance()
	}

	signed := false
	var width *ast.BitRange
	for {
		if p.isDriveOrChargeStrength() {
			p.parseBalancedParens(nil, nil)
			continue
		}
		if p.cur().Type == lexer.TokenScalared || p.cur().Type == lexer.TokenVectored {
			p.advance()
			continue
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			signed = p.cur().Type == lexer.TokenSigned
			p.advance()
			continue
		}
		if p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if width == nil {
				width = r
			}
			continue
		}
		if p.cur().Type == lexer.TokenHash {
			p.advance()
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(nil, mod)
			} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
				tok := p.advance()
				if tok.Type == lexer.TokenIdent {
					p.recordRef(nil, mod, tok)
				}
			}
			continue
		}
		break
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected signal identifier after %q", kind))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		// Optional unpacked array dimensions: e.g. reg [7:0] mem [0:255]; int q[$]; int map[string];
		var unpackedDims []*ast.BitRange
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if r != nil {
				unpackedDims = append(unpackedDims, r)
			}
			endPos = p.prev().Range.End
		}

		// Optional inline initialization: e.g. wire a = b;
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}

		decl := &ast.NetDecl{
			Kind:         kind,
			Signed:       signed,
			Width:        width,
			UnpackedDims: unpackedDims,
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

	isTypeParam := false
	if p.cur().Type == lexer.TokenTypeKw {
		isTypeParam = true
		p.advance()
	} else if isParamTypeKeyword(p.cur().Type) {
		p.advance()
	} else if p.isUserTypeBeforeIdent() {
		_, _ = p.parseScopedTypeName(nil, mod)
	}

	signed := false
	if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
		signed = p.cur().Type == lexer.TokenSigned
		p.advance()
	}

	var width *ast.BitRange
	for p.cur().Type == lexer.TokenLBracket {
		r := p.parseBitRange(mod)
		if width == nil {
			width = r
		}
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected identifier after %q", kind))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		if isTypeParam && p.typeNames != nil {
			p.typeNames[nameTok.Literal] = true
		}
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
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	// Optional drive strength `(strong1, pull0)` and/or delay `#1` or `#(1, 2)`
	for {
		if p.isDriveOrChargeStrength() {
			p.parseBalancedParens(nil, nil)
			continue
		}
		if p.cur().Type == lexer.TokenHash {
			p.advance() // '#'
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(&stmt.Refs, mod)
			} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
				tok := p.advance()
				if tok.Type == lexer.TokenIdent {
					p.recordRef(&stmt.Refs, mod, tok)
				}
			} else {
				p.addError(p.cur().Range, "expected delay value after '#' in assign statement")
			}
			continue
		}
		break
	}

	for !p.isEOF() {
		// Collect LHS up to '='
		lhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenAssignOp, lexer.TokenComma, lexer.TokenSemicolon)
		if len(lhsTokens) == 0 {
			p.addError(p.cur().Range, "expected target signal after 'assign'")
			break
		}

		if p.cur().Type == lexer.TokenAssignOp {
			eqTok := p.advance() // '='
			stmt.Writes = append(stmt.Writes, extractLHSWrites(lhsTokens, eqTok.Literal)...)
			rhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenComma, lexer.TokenSemicolon)
			if len(rhsTokens) == 0 {
				p.addError(eqTok.Range, "expected expression after '=' in assign statement")
			}
		} else {
			p.addError(p.cur().Range, "expected '=' in assign statement")
			break
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
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

func (p *Parser) parseDefparamStmt(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'defparam'
	stmt := &ast.DefparamStmt{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	for !p.isEOF() {
		lhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenAssignOp, lexer.TokenComma, lexer.TokenSemicolon)
		if len(lhsTokens) == 0 {
			p.addError(p.cur().Range, "expected parameter path in defparam statement")
			break
		}

		if p.cur().Type == lexer.TokenAssignOp {
			eqTok := p.advance() // '='
			rhsTokens := p.collectTokensUntil(&stmt.Refs, mod, lexer.TokenComma, lexer.TokenSemicolon)
			if len(rhsTokens) == 0 {
				p.addError(eqTok.Range, "expected expression after '=' in defparam statement")
			}
		} else {
			p.addError(p.cur().Range, "expected '=' in defparam statement")
			break
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	if p.cur().Type == lexer.TokenSemicolon {
		semi := p.advance()
		stmt.Range.End = semi.Range.End
	} else {
		p.addError(p.prev().Range, "expected ';' after defparam statement")
		stmt.Range.End = p.prev().Range.End
	}

	mod.Items = append(mod.Items, stmt)
}

func (p *Parser) parseProceduralBlock(mod *ast.ModuleDecl) {
	startTok := p.advance() // 'always' or 'initial'
	block := &ast.ProceduralBlock{
		Kind:  startTok.Literal,
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	tempMod := &ast.ModuleDecl{}

	// Optional sensitivity list: @(...) or @*
	if p.cur().Type == lexer.TokenAt {
		p.advance() // '@'
		if p.cur().Type == lexer.TokenStar {
			p.advance()
		} else if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(nil, tempMod)
		} else if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			p.recordRef(nil, tempMod, tok)
		} else {
			p.addError(p.cur().Range, "expected sensitivity list after '@'")
		}
	}

	p.parseStatement(block, tempMod)
	block.Range.End = p.prev().Range.End
	block.Locals = tempMod.Items
	block.Refs = tempMod.Refs
	mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
	mod.Items = append(mod.Items, block)
}

func (p *Parser) parseFunctionDecl(mod *ast.ModuleDecl) {
	p.parseFunctionDeclOpts(mod, false)
}

func (p *Parser) parseFunctionDeclOpts(mod *ast.ModuleDecl, isPrototype bool) {
	startTok := p.advance() // consume 'function'
	fn := &ast.FunctionDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenAutomatic {
		fn.Automatic = true
		p.advance()
	} else if p.cur().Type == lexer.TokenStatic {
		p.advance()
	}

	if p.cur().Type == lexer.TokenSigned {
		fn.Signed = true
		p.advance()
	}

	if p.cur().Type == lexer.TokenInteger || p.cur().Type == lexer.TokenReg ||
		p.cur().Type == lexer.TokenReal || p.cur().Type == lexer.TokenTime || p.cur().Type == lexer.TokenRealtime ||
		isSVDataType(p.cur().Type) || p.cur().Type == lexer.TokenVoid {
		if p.cur().Type != lexer.TokenReg {
			fn.ReturnType = p.cur().Literal
		}
		p.advance()
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			fn.Signed = p.cur().Type == lexer.TokenSigned
			p.advance()
		}
	} else if p.isUserTypeBeforeIdent() {
		fn.ReturnType, _ = p.parseScopedTypeName(nil, mod)
		if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
			_ = p.parseTypeParamArgs(nil, mod)
		}
	}

	for p.cur().Type == lexer.TokenLBracket {
		r := p.parseBitRange(mod)
		if fn.Width == nil {
			fn.Width = r
		}
	}

	if p.cur().Type == lexer.TokenIdent || p.cur().Type == lexer.TokenNew {
		nameTok := p.advance()
		if p.cur().Type == lexer.TokenColonColon && (p.peekN(1).Type == lexer.TokenIdent || p.peekN(1).Type == lexer.TokenNew) {
			p.advance() // '::'
			nameTok = p.advance()
		}
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

	if isPrototype {
		fn.Params = tempMod.Ports
		fn.Range.End = p.prev().Range.End
		mod.Items = append(mod.Items, fn)
		return
	}

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
		case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout, lexer.TokenRef:
			p.parsePortDeclItem(tempMod)
		case lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger,
			lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
			lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent:
			p.parseNetDeclItem(tempMod)
		case lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenAutomatic:
			p.parseVarWithModifiers(tempMod)
		case lexer.TokenParameter, lexer.TokenLocalparam:
			p.parseParamDeclItem(tempMod)
		case lexer.TokenImport:
			p.parseImportDecl(tempMod, nil)
		case lexer.TokenLet:
			p.parseLetDecl(tempMod, nil)
		case lexer.TokenTypedef:
			p.parseTypedefDecl(tempMod, nil)
		case lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion:
			p.parseEnumOrStructVarDecl(tempMod)
		default:
			block := &ast.ProceduralBlock{
				Kind:  "function",
				Range: ast.Range{File: p.cur().Range.File, Start: p.cur().Range.Start, End: p.cur().Range.End},
			}
			p.parseStatement(block, tempMod)
		}
	}

	if p.cur().Type == lexer.TokenEndfunction {
		p.advance()
		p.consumeOptionalEndLabel()
		fn.Range.End = p.prev().Range.End
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
	mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
	mod.Items = append(mod.Items, fn)
}

func (p *Parser) parseTaskDecl(mod *ast.ModuleDecl) {
	p.parseTaskDeclOpts(mod, false)
}

func (p *Parser) parseTaskDeclOpts(mod *ast.ModuleDecl, isPrototype bool) {
	startTok := p.advance() // consume 'task'
	task := &ast.TaskDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenAutomatic {
		task.Automatic = true
		p.advance()
	} else if p.cur().Type == lexer.TokenStatic {
		p.advance()
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		if p.cur().Type == lexer.TokenColonColon && p.peekN(1).Type == lexer.TokenIdent {
			p.advance() // '::'
			nameTok = p.advance()
		}
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

	if isPrototype {
		task.Params = tempMod.Ports
		task.Range.End = p.prev().Range.End
		mod.Items = append(mod.Items, task)
		return
	}

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
		case lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout, lexer.TokenRef:
			p.parsePortDeclItem(tempMod)
		case lexer.TokenWire, lexer.TokenReg, lexer.TokenInteger,
			lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
			lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent:
			p.parseNetDeclItem(tempMod)
		case lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenAutomatic:
			p.parseVarWithModifiers(tempMod)
		case lexer.TokenParameter, lexer.TokenLocalparam:
			p.parseParamDeclItem(tempMod)
		case lexer.TokenImport:
			p.parseImportDecl(tempMod, nil)
		case lexer.TokenLet:
			p.parseLetDecl(tempMod, nil)
		case lexer.TokenTypedef:
			p.parseTypedefDecl(tempMod, nil)
		case lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion:
			p.parseEnumOrStructVarDecl(tempMod)
		default:
			block := &ast.ProceduralBlock{
				Kind:  "task",
				Range: ast.Range{File: p.cur().Range.File, Start: p.cur().Range.Start, End: p.cur().Range.End},
			}
			p.parseStatement(block, tempMod)
		}
	}

	if p.cur().Type == lexer.TokenEndtask {
		p.advance()
		p.consumeOptionalEndLabel()
		task.Range.End = p.prev().Range.End
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
	mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
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
		p.advance()
		p.consumeOptionalEndLabel()
		gen.Range.End = p.prev().Range.End
	} else {
		p.addError(startTok.Range, "expected 'endgenerate' to close 'generate' block")
		gen.Range.End = p.prev().Range.End
	}

	gen.Items = subMod.Items
	gen.Refs = subMod.Refs
	mod.Refs = append(mod.Refs, subMod.Refs...)
	mod.MemberRefs = append(mod.MemberRefs, subMod.MemberRefs...)
	mod.Items = append(mod.Items, gen)
}

func (p *Parser) parseSpecifyBlock(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'specify'
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndspecify &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenPrimitive {
		tok := p.cur()
		switch tok.Type {
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			p.advance()
		case lexer.TokenSemicolon:
			p.advance()
		case lexer.TokenSpecparam:
			p.parseParamDeclItem(mod)
		case lexer.TokenSystemIdent:
			// System timing check: `$setuphold(posedge clk, posedge d, tSU, tHD, notifier);`
			p.advance()
			if p.cur().Type == lexer.TokenLParen {
				p.parseSpecifyParens(mod)
			}
			p.expectSemicolon("specify timing check")
		case lexer.TokenIf:
			// State-dependent path: `if (cond) (a => y) = (t1, t2);`
			p.advance()
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(nil, mod)
			}
			p.parseSpecifyPathDecl(mod)
		case lexer.TokenLParen:
			// Module path declaration: `(posedge clk => (q +: d)) = (1.0, 1.2);`
			p.parseSpecifyPathDecl(mod)
		case lexer.TokenIdent:
			switch tok.Literal {
			case "pulsestyle_onevent", "pulsestyle_ondetect", "showcancelled", "noshowcancelled":
				p.advance()
				for !p.isEOF() && p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenEndspecify {
					if p.cur().Type == lexer.TokenIdent {
						p.recordRef(nil, mod, p.advance())
					} else {
						p.advance()
					}
				}
				p.expectSemicolon(tok.Literal + " declaration")
			case "ifnone":
				p.advance()
				p.parseSpecifyPathDecl(mod)
			default:
				p.parseSpecifyPathDecl(mod)
			}
		default:
			p.advance()
		}
	}

	if p.cur().Type == lexer.TokenEndspecify {
		p.advance()
	} else {
		p.addError(startTok.Range, "expected 'endspecify' to close 'specify' block")
	}
}

// parseSpecifyPathDecl parses a path declaration inside `specify ... endspecify` up to ';',
// recording signal and specparam identifier references in mod.Refs.
func (p *Parser) parseSpecifyPathDecl(mod *ast.ModuleDecl) {
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenSemicolon &&
		p.cur().Type != lexer.TokenEndspecify &&
		p.cur().Type != lexer.TokenEndmodule {
		if p.cur().Type == lexer.TokenLParen {
			p.parseSpecifyParens(mod)
			continue
		}
		tok := p.advance()
		if tok.Type == lexer.TokenIdent && tok.Literal != "edge" {
			p.recordRef(nil, mod, tok)
		}
	}
	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
	}
}

// parseSpecifyParens parses balanced `(...)` inside a specify block (path or timing check),
// recording signal/specparam/notifier identifier references while skipping `edge [01, 10]` descriptors.
func (p *Parser) parseSpecifyParens(mod *ast.ModuleDecl) {
	p.advance() // '('
	depth := 1
	inEdgeBracket := false
	for !p.isEOF() && depth > 0 {
		tok := p.cur()
		if tok.Type == lexer.TokenSemicolon || tok.Type == lexer.TokenEndspecify || tok.Type == lexer.TokenEndmodule {
			break
		}
		p.advance()
		switch tok.Type {
		case lexer.TokenLParen:
			depth++
		case lexer.TokenRParen:
			depth--
		case lexer.TokenLBracket:
			if inEdgeBracket {
				// stay in edge bracket
			}
		case lexer.TokenRBracket:
			inEdgeBracket = false
		case lexer.TokenIdent:
			if tok.Literal == "edge" && p.cur().Type == lexer.TokenLBracket {
				inEdgeBracket = true
			} else if !inEdgeBracket {
				p.recordRef(nil, mod, tok)
			}
		}
	}
}

func (p *Parser) parsePrimitiveDecl() {
	startTok := p.advance() // consume 'primitive'
	primName := ""
	if p.cur().Type == lexer.TokenIdent {
		primName = p.advance().Literal
		if p.primitives != nil {
			p.primitives[primName] = true
		}
	} else {
		p.addError(p.cur().Range, "expected primitive name after 'primitive'")
	}

	if p.cur().Type == lexer.TokenLParen {
		p.parseBalancedParens(nil, nil)
	}
	p.expectSemicolon("primitive header")

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndprimitive &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenPrimitive {
		if p.cur().Type == lexer.TokenTable {
			tableTok := p.advance() // 'table'
			for !p.isEOF() &&
				p.cur().Type != lexer.TokenEndtable &&
				p.cur().Type != lexer.TokenEndprimitive &&
				p.cur().Type != lexer.TokenModule &&
				p.cur().Type != lexer.TokenPrimitive {
				if p.cur().Type == lexer.TokenIllegal {
					p.addError(p.cur().Range, p.cur().Literal)
				}
				p.advance()
			}
			if p.cur().Type == lexer.TokenEndtable {
				p.advance()
			} else {
				p.addError(tableTok.Range, "expected 'endtable' to close 'table'")
			}
			continue
		}
		if p.cur().Type == lexer.TokenIllegal {
			p.addError(p.cur().Range, p.cur().Literal)
		}
		p.advance()
	}

	if p.cur().Type == lexer.TokenEndprimitive {
		p.advance()
		p.consumeOptionalEndLabel()
	} else {
		p.addError(startTok.Range, fmt.Sprintf("expected 'endprimitive' to close primitive %q", primName))
	}
}

func (p *Parser) parseConfigDecl() *ast.ConfigDecl {
	startTok := p.advance() // consume 'config'
	cfg := &ast.ConfigDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}
	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		cfg.Name = nameTok.Literal
		cfg.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected config name after 'config'")
	}
	p.expectSemicolon("config header")

	parseLibCell := func() (string, lexer.Token) {
		first := p.advance()
		if p.cur().Type == lexer.TokenDot && p.peekN(1).Type == lexer.TokenIdent {
			p.advance() // '.'
			cellTok := p.advance()
			return first.Literal + "." + cellTok.Literal, cellTok
		}
		return first.Literal, first
	}

	skipToConfigSemi := func() {
		for !p.isEOF() && p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenEndconfig {
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(nil, nil)
				continue
			}
			p.advance()
		}
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
		}
	}

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndconfig &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenMacromodule &&
		p.cur().Type != lexer.TokenPrimitive &&
		p.cur().Type != lexer.TokenConfig {
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
		if tok.Type == lexer.TokenLocalparam {
			skipToConfigSemi()
			continue
		}
		if tok.Type == lexer.TokenDefault {
			// `default liblist { lib_id };`
			p.advance()
			skipToConfigSemi()
			continue
		}
		if tok.Type == lexer.TokenIdent {
			switch tok.Literal {
			case "design":
				// `design [lib.]cell { [lib.]cell };`
				p.advance()
				for !p.isEOF() && p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenEndconfig {
					if p.cur().Type == lexer.TokenIdent {
						fullName, cellTok := parseLibCell()
						cfg.Design = append(cfg.Design, fullName)
						cfg.Refs = append(cfg.Refs, ast.IdentifierRef{Name: cellTok.Literal, Range: cellTok.Range})
					} else {
						p.advance()
					}
				}
				if p.cur().Type == lexer.TokenSemicolon {
					p.advance()
				}
				continue

			case "instance":
				// `instance top.u1.u2 ( liblist ... | use [lib.]cell[:config] [#(...)] );`
				p.advance()
				if p.cur().Type == lexer.TokenIdent {
					rootTok := p.advance()
					cfg.Refs = append(cfg.Refs, ast.IdentifierRef{Name: rootTok.Literal, Range: rootTok.Range})
					for p.cur().Type == lexer.TokenDot && p.peekN(1).Type == lexer.TokenIdent {
						p.advance() // '.'
						p.advance() // inst name
					}
				}
				if p.cur().Type == lexer.TokenIdent && p.cur().Literal == "use" {
					p.advance()
					if p.cur().Type == lexer.TokenIdent {
						_, cellTok := parseLibCell()
						cfg.Refs = append(cfg.Refs, ast.IdentifierRef{Name: cellTok.Literal, Range: cellTok.Range})
					}
				}
				skipToConfigSemi()
				continue

			case "cell":
				// `cell [lib.]cell ( liblist ... | use [lib.]cell[:config] [#(...)] );`
				p.advance()
				if p.cur().Type == lexer.TokenIdent {
					_, cellTok := parseLibCell()
					cfg.Refs = append(cfg.Refs, ast.IdentifierRef{Name: cellTok.Literal, Range: cellTok.Range})
				}
				if p.cur().Type == lexer.TokenIdent && p.cur().Literal == "use" {
					p.advance()
					if p.cur().Type == lexer.TokenIdent {
						_, targetTok := parseLibCell()
						cfg.Refs = append(cfg.Refs, ast.IdentifierRef{Name: targetTok.Literal, Range: targetTok.Range})
					}
				}
				skipToConfigSemi()
				continue
			}
		}
		p.advance()
	}

	if p.cur().Type == lexer.TokenEndconfig {
		p.advance()
		p.consumeOptionalEndLabel()
		cfg.Range.End = p.prev().Range.End
	} else {
		p.addError(startTok.Range, fmt.Sprintf("expected 'endconfig' to close config %q", cfg.Name))
		cfg.Range.End = p.prev().Range.End
	}
	return cfg
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
			p.consumeOptionalEndLabel()
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
		if p.cur().Type == lexer.TokenInside {
			p.advance()
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
	mod.MemberRefs = append(mod.MemberRefs, subMod.MemberRefs...)
	mod.Items = append(mod.Items, gen)
}

func (p *Parser) parseStatement(block *ast.ProceduralBlock, mod *ast.ModuleDecl) {
	if p.isEOF() {
		return
	}
	if p.isAttrStart() {
		p.skipAttributes()
		if p.isEOF() || p.cur().Type == lexer.TokenEnd || isForkJoinEnd(p.cur().Type) || p.cur().Type == lexer.TokenEndcase || p.cur().Type == lexer.TokenEndfunction || p.cur().Type == lexer.TokenEndmodule {
			return
		}
	}

	switch p.cur().Type {
	case lexer.TokenSemicolon:
		p.advance()

	case lexer.TokenReg, lexer.TokenInteger,
		lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
		lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent:
		// Block-local variable declaration (e.g., `begin : blk integer i; reg [7:0] tmp; ... end`)
		p.parseNetDeclItem(mod)

	case lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenAutomatic,
		lexer.TokenLocal, lexer.TokenProtected, lexer.TokenRand, lexer.TokenRandc,
		lexer.TokenVirtual, lexer.TokenPure, lexer.TokenExtern:
		p.parseVarWithModifiers(mod)
	case lexer.TokenClass:
		if cls := p.parseClassDecl(); cls != nil {
			mod.Items = append(mod.Items, cls)
		}
	case lexer.TokenConstraint:
		p.parseConstraintDecl(mod)
	case lexer.TokenCovergroup:
		p.parseCovergroupDecl(mod)
	case lexer.TokenBind:
		p.parseBindStmt(mod, nil)
	case lexer.TokenChecker:
		if chk := p.parseModuleDecl(); chk != nil {
			mod.Items = append(mod.Items, chk)
		}
	case lexer.TokenLet:
		p.parseLetDecl(mod, nil)

	case lexer.TokenParameter, lexer.TokenLocalparam:
		// Block-local parameter/localparam declaration
		p.parseParamDeclItem(mod)

	case lexer.TokenImport:
		p.parseImportDecl(mod, nil)
	case lexer.TokenExport:
		p.parseDPIExportDecl(mod, nil)
	case lexer.TokenTypedef, lexer.TokenNettype:
		p.parseTypedefDecl(mod, nil)

	case lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion:
		p.parseEnumOrStructVarDecl(mod)

	case lexer.TokenBegin, lexer.TokenFork:
		startTok := p.advance() // 'begin' or 'fork'
		isFork := startTok.Type == lexer.TokenFork
		if p.cur().Type == lexer.TokenColon {
			p.advance() // ':'
			if p.cur().Type == lexer.TokenIdent {
				lbl := p.advance()
				if block.Label == "" {
					block.Label = lbl.Literal
				}
				block.Labels = append(block.Labels, lbl.Literal)
			} else {
				p.addError(p.cur().Range, "expected block label identifier after ':'")
			}
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEnd &&
			!isForkJoinEnd(p.cur().Type) &&
			p.cur().Type != lexer.TokenEndfunction &&
			p.cur().Type != lexer.TokenEndtask &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			p.parseStatement(block, mod)
		}

		if !isFork && p.cur().Type == lexer.TokenEnd {
			p.advance()
			p.consumeOptionalEndLabel()
		} else if isFork && isForkJoinEnd(p.cur().Type) {
			p.advance()
			p.consumeOptionalEndLabel()
		} else if isFork {
			p.addError(startTok.Range, "expected 'join', 'join_any', or 'join_none' to close 'fork' block")
		} else {
			p.addError(startTok.Range, "expected 'end' to close 'begin' block")
		}

	case lexer.TokenUnique, lexer.TokenUnique0, lexer.TokenPriority:
		p.advance()
		p.parseStatement(block, mod)

	case lexer.TokenAssert, lexer.TokenAssume, lexer.TokenCover, lexer.TokenRestrict, lexer.TokenExpect:
		p.parseAssertStmt(block, mod, nil)

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
		if p.cur().Type == lexer.TokenInside {
			p.advance()
		}

		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEndcase &&
			p.cur().Type != lexer.TokenEnd &&
			!isForkJoinEnd(p.cur().Type) &&
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

	case lexer.TokenRandcase:
		rcTok := p.advance()
		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEndcase &&
			p.cur().Type != lexer.TokenEnd &&
			!isForkJoinEnd(p.cur().Type) &&
			p.cur().Type != lexer.TokenEndfunction &&
			p.cur().Type != lexer.TokenEndtask &&
			p.cur().Type != lexer.TokenEndmodule &&
			p.cur().Type != lexer.TokenModule {
			tokensBeforeColon := p.collectTokensUntil(&block.Refs, mod, lexer.TokenColon, lexer.TokenEndcase)
			if p.cur().Type == lexer.TokenColon {
				p.advance() // ':'
				p.parseStatement(block, mod)
			} else if len(tokensBeforeColon) == 0 {
				break
			} else {
				p.addError(p.cur().Range, "expected ':' after randcase weight expression")
				p.syncToStatementEnd()
			}
		}
		if p.cur().Type == lexer.TokenEndcase {
			p.advance()
		} else {
			p.addError(rcTok.Range, "expected 'endcase' to close 'randcase'")
		}

	case lexer.TokenRandsequence:
		p.parseRandsequenceStmt(block, mod)

	case lexer.TokenFor, lexer.TokenWhile, lexer.TokenRepeat, lexer.TokenWait:
		loopTok := p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParensOpts(&block.Refs, mod, loopTok.Type == lexer.TokenFor)
		} else {
			p.addError(loopTok.Range, fmt.Sprintf("expected '(' after %q", loopTok.Literal))
		}
		p.parseStatement(block, mod)

	case lexer.TokenForeach:
		feTok := p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseForeachHeader(&block.Refs, mod)
		} else {
			p.addError(feTok.Range, "expected '(' after 'foreach'")
		}
		p.parseStatement(block, mod)

	case lexer.TokenForever:
		p.advance() // 'forever'
		p.parseStatement(block, mod)

	case lexer.TokenDo:
		doTok := p.advance() // 'do'
		p.parseStatement(block, mod)
		if p.cur().Type == lexer.TokenWhile {
			p.advance() // 'while'
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(&block.Refs, mod)
			} else {
				p.addError(p.cur().Range, "expected '(' after 'while' in do-while statement")
			}
			p.expectSemicolon("do-while statement")
		} else {
			p.addError(doTok.Range, "expected 'while' after 'do' statement")
		}

	case lexer.TokenBreak, lexer.TokenContinue:
		kwTok := p.advance()
		p.expectSemicolon(kwTok.Literal + " statement")

	case lexer.TokenReturn:
		p.advance()
		if p.cur().Type != lexer.TokenSemicolon {
			p.collectTokensUntil(&block.Refs, mod, lexer.TokenSemicolon)
		}
		p.expectSemicolon("return statement")

	case lexer.TokenPlusPlus, lexer.TokenMinusMinus:
		// Prefix increment/decrement statement: ++i; or --i;
		opTok := p.advance()
		lhsTokens := p.collectTokensUntil(&block.Refs, mod, lexer.TokenSemicolon)
		if block != nil {
			block.Writes = append(block.Writes, extractLHSWrites(lhsTokens, opTok.Literal)...)
		}
		p.expectSemicolon("increment/decrement statement")

	case lexer.TokenDisable:
		disableTok := p.advance() // 'disable'
		if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
			for p.cur().Type == lexer.TokenDot || p.cur().Type == lexer.TokenLBracket {
				if p.cur().Type == lexer.TokenDot {
					p.advance()
					if p.cur().Type == lexer.TokenIdent {
						p.advance()
					}
				} else {
					_ = p.parseBitRange(mod)
				}
			}
		} else {
			p.addError(disableTok.Range, "expected block or task identifier after 'disable'")
		}
		p.expectSemicolon("disable statement")

	case lexer.TokenHash:
		// Delay control: #10 stmt; or #10ns stmt; or #(10) stmt;
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else if p.cur().Type == lexer.TokenNumber {
			p.advance()
			if p.cur().Type == lexer.TokenIdent && isTimeUnit(p.cur().Literal) {
				p.advance()
			}
		} else if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			if !isTimeUnit(tok.Literal) {
				p.recordRef(&block.Refs, mod, tok)
			}
		}
		p.parseStatement(block, mod)

	case lexer.TokenHashHash:
		// Cycle delay control: ##1; or ##5 cb.req <= 1; or ##(N) stmt;
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		} else if p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
		} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			if tok.Type == lexer.TokenIdent {
				p.recordRef(&block.Refs, mod, tok)
			}
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

	case lexer.TokenAssign, lexer.TokenForce:
		// Procedural continuous assignment: assign q = d; or force sig = val;
		kwTok := p.advance()
		for !p.isEOF() {
			lhs := p.collectTokensUntil(&block.Refs, mod, lexer.TokenAssignOp, lexer.TokenComma, lexer.TokenSemicolon)
			if len(lhs) == 0 {
				p.addError(kwTok.Range, fmt.Sprintf("expected target signal after %q", kwTok.Literal))
				break
			}
			if p.cur().Type == lexer.TokenAssignOp {
				eqTok := p.advance()
				rhs := p.collectTokensUntil(&block.Refs, mod, lexer.TokenComma, lexer.TokenSemicolon)
				if len(rhs) == 0 {
					p.addError(eqTok.Range, fmt.Sprintf("expected expression after '=' in %s statement", kwTok.Literal))
				}
			} else {
				p.addError(p.cur().Range, fmt.Sprintf("expected '=' in %s statement", kwTok.Literal))
				break
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}
			break
		}
		p.expectSemicolon(kwTok.Literal + " statement")

	case lexer.TokenDeassign, lexer.TokenRelease:
		// Procedural deassign / release: deassign q; or release sig;
		kwTok := p.advance()
		for !p.isEOF() {
			lhs := p.collectTokensUntil(&block.Refs, mod, lexer.TokenComma, lexer.TokenSemicolon)
			if len(lhs) == 0 {
				p.addError(kwTok.Range, fmt.Sprintf("expected target signal after %q", kwTok.Literal))
				break
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}
			break
		}
		p.expectSemicolon(kwTok.Literal + " statement")

	case lexer.TokenArrow:
		// Event trigger: -> ev; or -> u_sub.ev;
		arrowTok := p.advance()
		if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			p.recordRef(&block.Refs, mod, tok)
			for p.cur().Type == lexer.TokenDot || p.cur().Type == lexer.TokenLBracket {
				if p.cur().Type == lexer.TokenDot {
					p.advance()
					if p.cur().Type == lexer.TokenIdent {
						p.advance()
					}
				} else {
					_ = p.parseBitRange(mod)
				}
			}
		} else {
			p.addError(arrowTok.Range, "expected event identifier after '->'")
		}
		p.expectSemicolon("event trigger")

	case lexer.TokenThis, lexer.TokenSuper:
		if p.isTaskCallStart() {
			p.parseTaskCallStmt(block, mod)
			return
		}
		fallthrough

	case lexer.TokenIdent:
		// Labeled assertion inside procedural block: `chk_a: assert (...)`
		if p.peekN(1).Type == lexer.TokenColon && isAssertionKeyword(p.peekN(2).Type) {
			lblTok := p.advance()
			p.advance() // ':'
			p.parseAssertStmt(block, mod, &lblTok)
			return
		}
		// User-defined type variable declaration inside block/function/task: e.g. state_t next_st;
		if p.isUserTypeVarDecl() {
			p.parseUserTypeVarDecl(mod)
			return
		}
		// Task call statement: my_task(a, b); or my_task; or u_sub.my_task(a, b);
		if p.isTaskCallStart() {
			p.parseTaskCallStmt(block, mod)
			return
		}
		fallthrough

	case lexer.TokenLBrace:
		// Procedural assignment or postfix increment/decrement:
		// a = b; a <= b; a += b; i++; {a, b} <= c;
		lhsTokens := p.collectTokensUntil(&block.Refs, mod,
			lexer.TokenAssignOp, lexer.TokenLeq,
			lexer.TokenPlusPlus, lexer.TokenMinusMinus,
			lexer.TokenPlusEq, lexer.TokenMinusEq, lexer.TokenMulEq, lexer.TokenDivEq, lexer.TokenModEq,
			lexer.TokenAndEq, lexer.TokenOrEq, lexer.TokenXorEq, lexer.TokenShlEq, lexer.TokenShrEq,
			lexer.TokenSemicolon)
		if p.cur().Type == lexer.TokenPlusPlus || p.cur().Type == lexer.TokenMinusMinus {
			opTok := p.advance()
			if block != nil {
				block.Writes = append(block.Writes, extractLHSWrites(lhsTokens, opTok.Literal)...)
			}
			p.expectSemicolon("increment/decrement statement")
		} else if isProceduralAssignOp(p.cur().Type) {
			opTok := p.advance()
			if block != nil {
				block.Writes = append(block.Writes, extractLHSWrites(lhsTokens, opTok.Literal)...)
			}
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
	var namedParamRefs []ast.IdentifierRef
	// Optional drive strength `(strong1, pull0)` and/or parameter/delay `#(...)` or `#VALUE`
	for {
		if p.isDriveOrChargeStrength() {
			p.parseBalancedParens(nil, nil)
			continue
		}
		if p.cur().Type == lexer.TokenHash {
			p.advance() // '#'
			if p.cur().Type == lexer.TokenLParen {
				p.parseInstConnections(&paramRefs, &namedParamRefs, nil, mod)
			} else if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
				tok := p.advance()
				if tok.Type == lexer.TokenIdent {
					p.recordRef(&paramRefs, mod, tok)
				}
			} else {
				p.addError(p.cur().Range, "expected '(' after '#' in module instantiation")
			}
			continue
		}
		break
	}

	if (p.cur().Type == lexer.TokenLParen || p.cur().Type == lexer.TokenLBracket) &&
		(isGatePrimitive(modNameTok.Literal) || p.primitives[modNameTok.Literal]) {
		for !p.isEOF() {
			if p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(mod)
			}
			if p.cur().Type != lexer.TokenLParen {
				p.addError(p.cur().Range, fmt.Sprintf("expected '(' in %s gate instantiation", modNameTok.Literal))
				p.syncToStatementEnd()
				return
			}
			p.parseInstConnections(nil, nil, nil, mod)
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}
			break
		}
		p.expectSemicolon("gate instantiation")
		return
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
			ParamRefs:         append([]ast.IdentifierRef(nil), namedParamRefs...),
		}

		// Optional instance array range: e.g. sub_mod u_sub [3:0] (...);
		if p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
		}

		if p.cur().Type == lexer.TokenLParen {
			p.parseInstConnections(&inst.Refs, &inst.PortRefs, inst, mod)
			inst.Range.End = p.prev().Range.End
		} else if p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenComma {
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

// parseInstConnections parses (.port(expr), ...) or positional (expr, ...) while recording
// local signal references, named port/parameter references, implicit `.sig` / `.*`, and member accesses.
func (p *Parser) parseInstConnections(refs *[]ast.IdentifierRef, namedRefs *[]ast.IdentifierRef, inst *ast.ModuleInst, mod *ast.ModuleDecl) {
	openTok := p.advance() // '('
	depth := 1
	prevWasDot := false
	scopedPrefix := ""
	lastBase := ""
	bracketDepth := 0
	for !p.isEOF() && depth > 0 {
		if p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		if p.isAttrStart() {
			p.parseAttribute()
			continue
		}
		if p.cur().Type == lexer.TokenDot && depth == 1 {
			dotTok := p.advance() // '.'
			if p.cur().Type == lexer.TokenStar {
				// SystemVerilog wildcard port connection: .*
				starTok := p.advance()
				if inst != nil {
					inst.HasWildcard = true
					inst.WildcardRange = ast.Range{
						File:  dotTok.Range.File,
						Start: dotTok.Range.Start,
						End:   starTok.Range.End,
					}
				}
			} else if p.cur().Type == lexer.TokenIdent {
				portTok := p.advance()
				if namedRefs != nil {
					*namedRefs = append(*namedRefs, ast.IdentifierRef{
						Name:  portTok.Literal,
						Range: portTok.Range,
					})
				}
				// SystemVerilog implicit named port connection `.sig` (no '(' follows):
				// record local signal reference to `sig`.
				if p.cur().Type != lexer.TokenLParen {
					p.recordRef(refs, mod, portTok)
					if inst != nil {
						inst.ImplicitPortRefs = append(inst.ImplicitPortRefs, ast.IdentifierRef{
							Name:  portTok.Literal,
							Range: portTok.Range,
						})
					}
				}
			}
			prevWasDot = false
			scopedPrefix = ""
			lastBase = ""
			continue
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLParen:
			depth++
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRParen:
			depth--
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenColonColon:
			// keep scopedPrefix for next ident
			lastBase = ""
		case lexer.TokenThis, lexer.TokenSuper:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = tok.Literal
			}
		case lexer.TokenIdent:
			if !prevWasDot {
				if p.cur().Type == lexer.TokenColonColon {
					scopedPrefix = tok.Literal
					lastBase = ""
				} else if scopedPrefix != "" {
					p.recordScopedRef(refs, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
					scopedPrefix = ""
					lastBase = ""
				} else {
					p.recordRef(refs, mod, tok)
					if bracketDepth == 0 {
						lastBase = tok.Literal
					}
				}
			} else {
				if lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(mod, lastBase, tok)
					lastBase = lastBase + "." + tok.Literal
				} else {
					lastBase = ""
				}
				scopedPrefix = ""
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
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
	prevWasDot := false
	scopedPrefix := ""

	nestedBracketDepth := 0
	for !p.isEOF() && (p.cur().Type != lexer.TokenRBracket || nestedBracketDepth > 0) {
		if p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		tok := p.advance()
		if tok.Type == lexer.TokenLBracket {
			nestedBracketDepth++
		} else if tok.Type == lexer.TokenRBracket && nestedBracketDepth > 0 {
			nestedBracketDepth--
		}
		if tok.Type == lexer.TokenColon && nestedBracketDepth == 0 {
			inLSB = true
			prevWasDot = false
			scopedPrefix = ""
			continue
		}
		if tok.Type == lexer.TokenColonColon {
			// keep scopedPrefix
		} else if tok.Type == lexer.TokenIdent && !prevWasDot {
			if p.cur().Type == lexer.TokenColonColon {
				scopedPrefix = tok.Literal
			} else if scopedPrefix != "" {
				p.recordScopedRef(nil, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
				scopedPrefix = ""
			} else {
				p.recordRef(nil, mod, tok)
			}
		} else {
			scopedPrefix = ""
		}
		prevWasDot = tok.Type == lexer.TokenDot
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
	patternDepth := 0
	prevWasDot := false
	prevWasTick := false
	scopedPrefix := ""

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
			scopedPrefix = ""
		case lexer.TokenRParen:
			if parenDepth > 0 {
				parenDepth--
			}
			scopedPrefix = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenLBrace:
			braceDepth++
			if prevWasTick {
				patternDepth++
			}
			scopedPrefix = ""
		case lexer.TokenRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
			if patternDepth > 0 && braceDepth < patternDepth {
				patternDepth--
			}
			scopedPrefix = ""
		case lexer.TokenColonColon:
			// keep scopedPrefix
		case lexer.TokenIdent:
			// Skip member labels like `addr:` inside assignment patterns `'{addr: 8'h10}`
			if !prevWasDot && !(patternDepth > 0 && p.cur().Type == lexer.TokenColon) {
				if p.cur().Type == lexer.TokenColonColon {
					scopedPrefix = tok.Literal
				} else if scopedPrefix != "" {
					p.recordScopedRef(nil, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
					scopedPrefix = ""
				} else {
					p.recordRef(nil, mod, tok)
				}
			} else {
				scopedPrefix = ""
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
		default:
			scopedPrefix = ""
		}
		prevWasDot = tok.Type == lexer.TokenDot
		prevWasTick = tok.Type == lexer.TokenTick
	}

	return strings.Join(parts, " "), endPos
}

func (p *Parser) collectTokensUntil(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl, stopTokens ...lexer.TokenType) []lexer.Token {
	var collected []lexer.Token
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	patternDepth := 0
	prevWasDot := false
	prevWasTick := false
	scopedPrefix := ""
	lastBase := ""

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
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRParen:
			if parenDepth > 0 {
				parenDepth--
			}
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenLBrace:
			braceDepth++
			if prevWasTick {
				patternDepth++
			}
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRBrace:
			if braceDepth > 0 {
				braceDepth--
			}
			if patternDepth > 0 && braceDepth < patternDepth {
				patternDepth--
			}
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenColonColon:
			// keep scopedPrefix
			lastBase = ""
		case lexer.TokenThis, lexer.TokenSuper:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = tok.Literal
			}
		case lexer.TokenIdent:
			if !prevWasDot && !(patternDepth > 0 && p.cur().Type == lexer.TokenColon) {
				if p.cur().Type == lexer.TokenColonColon {
					scopedPrefix = tok.Literal
					lastBase = ""
				} else if scopedPrefix != "" {
					p.recordScopedRef(refs, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
					scopedPrefix = ""
					lastBase = ""
				} else {
					p.recordRef(refs, mod, tok)
					if bracketDepth == 0 {
						lastBase = tok.Literal
					}
				}
			} else {
				if prevWasDot && lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(mod, lastBase, tok)
					lastBase = lastBase + "." + tok.Literal
				} else {
					lastBase = ""
				}
				scopedPrefix = ""
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
		prevWasTick = tok.Type == lexer.TokenTick
	}

	return collected
}

func (p *Parser) parseBalancedParens(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl) {
	p.parseBalancedParensOpts(refs, mod, false)
}

func (p *Parser) parseBalancedParensOpts(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl, allowSemicolon bool) {
	openTok := p.advance() // '('
	depth := 1
	prevWasDot := false
	scopedPrefix := ""
	lastBase := ""
	bracketDepth := 0
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
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRParen:
			depth--
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenGenvar, lexer.TokenInteger,
			lexer.TokenInt, lexer.TokenByte, lexer.TokenShortint, lexer.TokenLongint, lexer.TokenLogic, lexer.TokenBit:
			scopedPrefix = ""
			lastBase = ""
			if allowSemicolon && mod != nil && p.cur().Type == lexer.TokenIdent {
				nameTok := p.advance()
				mod.Items = append(mod.Items, &ast.NetDecl{
					Kind:      tok.Literal,
					Name:      nameTok.Literal,
					NameRange: nameTok.Range,
					Range:     ast.Range{File: tok.Range.File, Start: tok.Range.Start, End: nameTok.Range.End},
				})
			}
		case lexer.TokenColonColon:
			// keep scopedPrefix
			lastBase = ""
		case lexer.TokenThis, lexer.TokenSuper:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = tok.Literal
			}
		case lexer.TokenIdent:
			if !prevWasDot {
				if p.cur().Type == lexer.TokenColonColon {
					scopedPrefix = tok.Literal
					lastBase = ""
				} else if scopedPrefix != "" {
					p.recordScopedRef(refs, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
					scopedPrefix = ""
					lastBase = ""
				} else {
					p.recordRef(refs, mod, tok)
					if bracketDepth == 0 {
						lastBase = tok.Literal
					}
				}
			} else {
				if lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(mod, lastBase, tok)
					lastBase = lastBase + "." + tok.Literal
				} else {
					lastBase = ""
				}
				scopedPrefix = ""
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}
	if depth != 0 {
		p.addError(openTok.Range, "unclosed '('")
	}
}

func (p *Parser) recordRef(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl, tok lexer.Token) {
	p.recordScopedRef(itemRefs, mod, tok.Literal, tok.Range)
}

func (p *Parser) recordScopedRef(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl, name string, rng ast.Range) {
	ref := ast.IdentifierRef{
		Name:  name,
		Range: rng,
	}
	if itemRefs != nil {
		*itemRefs = append(*itemRefs, ref)
	}
	if mod != nil {
		mod.Refs = append(mod.Refs, ref)
	}
}

func (p *Parser) recordMemberRef(mod *ast.ModuleDecl, baseName string, memberTok lexer.Token) {
	if mod == nil || baseName == "" || memberTok.Literal == "" {
		return
	}
	mod.MemberRefs = append(mod.MemberRefs, ast.MemberRef{
		BaseName:   baseName,
		MemberName: memberTok.Literal,
		Range:      memberTok.Range,
	})
}

func extractLHSWrites(lhsTokens []lexer.Token, op string) []ast.WriteRef {
	var writes []ast.WriteRef
	bracketDepth := 0
	parenDepth := 0
	prevWasDot := false

	for i, tok := range lhsTokens {
		switch tok.Type {
		case lexer.TokenLBracket:
			bracketDepth++
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case lexer.TokenLParen:
			parenDepth++
		case lexer.TokenRParen:
			if parenDepth > 0 {
				parenDepth--
			}
		case lexer.TokenIdent:
			if bracketDepth == 0 && parenDepth == 0 && !prevWasDot {
				// Skip package prefix `pkg::`
				if i+1 < len(lhsTokens) && lhsTokens[i+1].Type == lexer.TokenColonColon {
					break
				}
				// Check if followed (after optional `[...]` index) by `.member`
				j := i + 1
				for j < len(lhsTokens) && lhsTokens[j].Type == lexer.TokenLBracket {
					d := 1
					j++
					for j < len(lhsTokens) && d > 0 {
						if lhsTokens[j].Type == lexer.TokenLBracket {
							d++
						} else if lhsTokens[j].Type == lexer.TokenRBracket {
							d--
						}
						j++
					}
				}
				isMember := j < len(lhsTokens) && lhsTokens[j].Type == lexer.TokenDot
				writes = append(writes, ast.WriteRef{
					Name:          tok.Literal,
					Op:            op,
					IsMemberWrite: isMember,
					Range:         tok.Range,
				})
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}
	return writes
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
		case lexer.TokenEnd, lexer.TokenJoin, lexer.TokenJoinAny, lexer.TokenJoinNone, lexer.TokenEndcase, lexer.TokenEndfunction, lexer.TokenEndtask, lexer.TokenEndclass, lexer.TokenEndgroup, lexer.TokenEndclocking, lexer.TokenEndgenerate, lexer.TokenEndspecify, lexer.TokenEndmodule, lexer.TokenEndprogram, lexer.TokenEndchecker, lexer.TokenEndinterface, lexer.TokenEndpackage, lexer.TokenModule, lexer.TokenMacromodule, lexer.TokenProgram, lexer.TokenChecker, lexer.TokenLet, lexer.TokenInterface, lexer.TokenPackage, lexer.TokenClass, lexer.TokenCovergroup, lexer.TokenClocking, lexer.TokenPrimitive,
			lexer.TokenInput, lexer.TokenOutput, lexer.TokenInout, lexer.TokenRef, lexer.TokenModport,
			lexer.TokenWire, lexer.TokenUwire, lexer.TokenInterconnect, lexer.TokenReg,
			lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
			lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenLocal, lexer.TokenProtected, lexer.TokenRand, lexer.TokenRandc, lexer.TokenVirtual, lexer.TokenPure, lexer.TokenExtern, lexer.TokenConstraint,
			lexer.TokenInteger, lexer.TokenGenvar,
			lexer.TokenTri, lexer.TokenTri0, lexer.TokenTri1, lexer.TokenTriand, lexer.TokenTrior, lexer.TokenTrireg,
			lexer.TokenSupply0, lexer.TokenSupply1, lexer.TokenWand, lexer.TokenWor,
			lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent,
			lexer.TokenParameter, lexer.TokenLocalparam, lexer.TokenDefparam, lexer.TokenTypedef, lexer.TokenNettype, lexer.TokenImport, lexer.TokenExport,
			lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion,
			lexer.TokenAssign, lexer.TokenDeassign, lexer.TokenForce, lexer.TokenRelease, lexer.TokenBind,
			lexer.TokenAlways, lexer.TokenAlwaysComb, lexer.TokenAlwaysFF, lexer.TokenAlwaysLatch, lexer.TokenInitial, lexer.TokenFinal,
			lexer.TokenAssert, lexer.TokenAssume, lexer.TokenCover, lexer.TokenRestrict,
			lexer.TokenProperty, lexer.TokenEndproperty, lexer.TokenSequence, lexer.TokenEndsequence,
			lexer.TokenFunction, lexer.TokenTask, lexer.TokenGenerate, lexer.TokenSpecify:
			return
		default:
			p.advance()
		}
	}
}

func isPortDirection(t lexer.TokenType) bool {
	return t == lexer.TokenInput || t == lexer.TokenOutput || t == lexer.TokenInout || t == lexer.TokenRef
}

func isGatePrimitive(name string) bool {
	switch name {
	case "and", "nand", "or", "nor", "xor", "xnor",
		"buf", "not", "bufif0", "bufif1", "notif0", "notif1",
		"pullup", "pulldown",
		"nmos", "pmos", "rnmos", "rpmos", "cmos", "rcmos",
		"tran", "rtran", "tranif0", "tranif1", "rtranif0", "rtranif1":
		return true
	default:
		return false
	}
}

func (p *Parser) isDriveOrChargeStrength() bool {
	if p.cur().Type != lexer.TokenLParen {
		return false
	}
	return isStrengthSpec(p.peekN(1).Literal)
}

func isStrengthSpec(lit string) bool {
	switch lit {
	case "supply0", "supply1", "strong0", "strong1", "pull0", "pull1",
		"weak0", "weak1", "highz0", "highz1", "small", "medium", "large":
		return true
	default:
		return false
	}
}

func isSVDataType(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal:
		return true
	default:
		return false
	}
}

func isParamTypeKeyword(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenInteger, lexer.TokenReal, lexer.TokenShortreal, lexer.TokenTime, lexer.TokenRealtime,
		lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle:
		return true
	default:
		return false
	}
}

func isForkJoinEnd(t lexer.TokenType) bool {
	return t == lexer.TokenJoin || t == lexer.TokenJoinAny || t == lexer.TokenJoinNone
}

func isProceduralAssignOp(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenAssignOp, lexer.TokenLeq,
		lexer.TokenPlusEq, lexer.TokenMinusEq, lexer.TokenMulEq, lexer.TokenDivEq, lexer.TokenModEq,
		lexer.TokenAndEq, lexer.TokenOrEq, lexer.TokenXorEq, lexer.TokenShlEq, lexer.TokenShrEq:
		return true
	default:
		return false
	}
}

func isPortNetType(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenWire, lexer.TokenUwire, lexer.TokenInterconnect, lexer.TokenReg, lexer.TokenInteger,
		lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
		lexer.TokenTri, lexer.TokenTri0, lexer.TokenTri1, lexer.TokenTriand, lexer.TokenTrior, lexer.TokenTrireg,
		lexer.TokenSupply0, lexer.TokenSupply1, lexer.TokenWand, lexer.TokenWor,
		lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime:
		return true
	default:
		return false
	}
}

func isStatementBoundary(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenSemicolon, lexer.TokenEnd, lexer.TokenJoin, lexer.TokenJoinAny, lexer.TokenJoinNone, lexer.TokenEndcase, lexer.TokenEndfunction, lexer.TokenEndtask, lexer.TokenEndclass, lexer.TokenEndgroup, lexer.TokenEndclocking, lexer.TokenEndproperty, lexer.TokenEndsequence, lexer.TokenEndgenerate, lexer.TokenEndspecify, lexer.TokenEndmodule, lexer.TokenEndprogram, lexer.TokenEndchecker, lexer.TokenEndinterface, lexer.TokenEndpackage, lexer.TokenModule, lexer.TokenMacromodule, lexer.TokenProgram, lexer.TokenChecker, lexer.TokenLet, lexer.TokenInterface, lexer.TokenPackage, lexer.TokenClass, lexer.TokenCovergroup, lexer.TokenClocking, lexer.TokenPrimitive, lexer.TokenModport,
		lexer.TokenAlways, lexer.TokenAlwaysComb, lexer.TokenAlwaysFF, lexer.TokenAlwaysLatch, lexer.TokenInitial, lexer.TokenFinal,
		lexer.TokenAssert, lexer.TokenAssume, lexer.TokenCover, lexer.TokenRestrict, lexer.TokenProperty, lexer.TokenSequence, lexer.TokenConstraint,
		lexer.TokenAssign, lexer.TokenDefparam, lexer.TokenBind, lexer.TokenFunction, lexer.TokenTask, lexer.TokenGenerate, lexer.TokenSpecify:
		return true
	default:
		return false
	}
}

func isAssertionKeyword(t lexer.TokenType) bool {
	return t == lexer.TokenAssert || t == lexer.TokenAssume || t == lexer.TokenCover || t == lexer.TokenRestrict || t == lexer.TokenExpect
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
	// `(*)` or `(* )` is a wildcard sensitivity list, not an attribute instance.
	if p.peekN(2).Type == lexer.TokenRParen {
		return false
	}
	return true
}

func (p *Parser) consumeOptionalEndLabel() {
	if p.cur().Type == lexer.TokenColon {
		p.advance() // ':'
		if p.cur().Type == lexer.TokenIdent {
			p.advance()
		}
	}
}

// isUserTypeBeforeIdent reports whether the current TokenIdent (or scoped `pkg::type_t`,
// optionally parameterized `box #(packet)`) is a user-defined type name followed
// (after optional packed dimensions `[...]`) by another TokenIdent.
func (p *Parser) isUserTypeBeforeIdent() bool {
	if p.cur().Type != lexer.TokenIdent {
		return false
	}
	offset := 1
	if p.peekN(offset).Type == lexer.TokenColonColon && p.peekN(offset+1).Type == lexer.TokenIdent {
		offset += 2
	}
	if p.peekN(offset).Type == lexer.TokenHash && p.peekN(offset+1).Type == lexer.TokenLParen {
		depth := 1
		offset += 2
		for depth > 0 {
			pt := p.peekN(offset)
			if pt.Type == lexer.TokenEOF || pt.Type == lexer.TokenSemicolon {
				return false
			}
			if pt.Type == lexer.TokenLParen {
				depth++
			} else if pt.Type == lexer.TokenRParen {
				depth--
			}
			offset++
		}
	}
	for p.peekN(offset).Type == lexer.TokenLBracket {
		depth := 1
		offset++
		for depth > 0 {
			bt := p.peekN(offset)
			if bt.Type == lexer.TokenEOF || bt.Type == lexer.TokenSemicolon {
				return false
			}
			if bt.Type == lexer.TokenLBracket {
				depth++
			} else if bt.Type == lexer.TokenRBracket {
				depth--
			}
			offset++
		}
	}
	return p.peekN(offset).Type == lexer.TokenIdent
}

// isUserTypeVarDecl reports whether the current TokenIdent (or `pkg::type_t`, optionally `#(...)`)
// begins a variable declaration, e.g. `state_t cur, next;`, `box #(packet) b1;`, `T tmp = '0;`.
func (p *Parser) isUserTypeVarDecl() bool {
	if p.cur().Type != lexer.TokenIdent {
		return false
	}
	offset := 1
	if p.peekN(offset).Type == lexer.TokenColonColon && p.peekN(offset+1).Type == lexer.TokenIdent {
		offset += 2
	}
	// Skip optional class/type parameter overrides: `box #(packet, 16) b1;`
	if p.peekN(offset).Type == lexer.TokenHash && p.peekN(offset+1).Type == lexer.TokenLParen {
		depth := 1
		offset += 2
		for depth > 0 {
			pt := p.peekN(offset)
			if pt.Type == lexer.TokenEOF || pt.Type == lexer.TokenSemicolon {
				return false
			}
			if pt.Type == lexer.TokenLParen {
				depth++
			} else if pt.Type == lexer.TokenRParen {
				depth--
			}
			offset++
		}
	}
	// Skip optional packed dimensions after type name: `pkt_t [3:0] var_name`
	for p.peekN(offset).Type == lexer.TokenLBracket {
		depth := 1
		offset++
		for depth > 0 {
			bt := p.peekN(offset)
			if bt.Type == lexer.TokenEOF || bt.Type == lexer.TokenSemicolon {
				return false
			}
			if bt.Type == lexer.TokenLBracket {
				depth++
			} else if bt.Type == lexer.TokenRBracket {
				depth--
			}
			offset++
		}
	}
	if p.peekN(offset).Type != lexer.TokenIdent {
		return false
	}
	offset++ // variable name
	// Skip optional unpacked dimensions after variable name: `var_name [0:15]`
	for p.peekN(offset).Type == lexer.TokenLBracket {
		depth := 1
		offset++
		for depth > 0 {
			bt := p.peekN(offset)
			if bt.Type == lexer.TokenEOF || bt.Type == lexer.TokenSemicolon {
				return false
			}
			if bt.Type == lexer.TokenLBracket {
				depth++
			} else if bt.Type == lexer.TokenRBracket {
				depth--
			}
			offset++
		}
	}
	next := p.peekN(offset).Type
	return next == lexer.TokenSemicolon || next == lexer.TokenComma || next == lexer.TokenAssignOp
}

// parseScopedTypeName parses `type_t` or `pkg::type_t`, records a reference, and returns the name and leading token.
func (p *Parser) parseScopedTypeName(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl) (string, lexer.Token) {
	firstTok := p.advance()
	if p.cur().Type == lexer.TokenColonColon && p.peekN(1).Type == lexer.TokenIdent {
		p.advance() // '::'
		symTok := p.advance()
		fullName := firstTok.Literal + "::" + symTok.Literal
		p.recordScopedRef(itemRefs, mod, fullName, symTok.Range)
		return fullName, firstTok
	}
	p.recordRef(itemRefs, mod, firstTok)
	return firstTok.Literal, firstTok
}

// parseVarWithModifiers parses variable, class, method, or constraint declarations prefixed with
// SystemVerilog modifiers (`const`, `var`, `static`, `automatic`, `local`, `protected`, `rand`, `randc`,
// `virtual`, `pure`, `extern`).
func (p *Parser) parseVarWithModifiers(mod *ast.ModuleDecl) {
	firstTok := p.cur()
	var mods []string
	hasVirtual := false
	hasPure := false
	hasExtern := false
	for {
		switch p.cur().Type {
		case lexer.TokenConst, lexer.TokenVar, lexer.TokenStatic, lexer.TokenAutomatic,
			lexer.TokenLocal, lexer.TokenProtected, lexer.TokenRand, lexer.TokenRandc:
			mods = append(mods, p.advance().Literal)
			continue
		case lexer.TokenVirtual:
			hasVirtual = true
			mods = append(mods, p.advance().Literal)
			continue
		case lexer.TokenPure:
			hasPure = true
			mods = append(mods, p.advance().Literal)
			continue
		case lexer.TokenExtern:
			hasExtern = true
			mods = append(mods, p.advance().Literal)
			continue
		}
		break
	}

	switch p.cur().Type {
	case lexer.TokenClass:
		if cls := p.parseClassDeclWithVirtual(hasVirtual, firstTok.Range.Start); cls != nil && mod != nil {
			mod.Items = append(mod.Items, cls)
		}
	case lexer.TokenFunction:
		p.parseFunctionDeclOpts(mod, hasPure || hasExtern)
	case lexer.TokenTask:
		p.parseTaskDeclOpts(mod, hasPure || hasExtern)
	case lexer.TokenConstraint:
		p.parseConstraintDecl(mod)
	case lexer.TokenCovergroup:
		p.parseCovergroupDecl(mod)
	case lexer.TokenBind:
		p.parseBindStmt(mod, nil)
	case lexer.TokenChecker:
		if chk := p.parseModuleDecl(); chk != nil {
			mod.Items = append(mod.Items, chk)
		}
	case lexer.TokenLet:
		p.parseLetDecl(mod, nil)
	case lexer.TokenEnum, lexer.TokenStruct, lexer.TokenUnion:
		p.parseEnumOrStructVarDecl(mod)
	case lexer.TokenWire, lexer.TokenUwire, lexer.TokenInterconnect, lexer.TokenReg,
		lexer.TokenLogic, lexer.TokenBit, lexer.TokenByte, lexer.TokenShortint, lexer.TokenInt, lexer.TokenLongint, lexer.TokenStringKw, lexer.TokenChandle, lexer.TokenShortreal,
		lexer.TokenInteger, lexer.TokenGenvar,
		lexer.TokenTri, lexer.TokenTri0, lexer.TokenTri1, lexer.TokenTriand, lexer.TokenTrior, lexer.TokenTrireg,
		lexer.TokenSupply0, lexer.TokenSupply1, lexer.TokenWand, lexer.TokenWor,
		lexer.TokenReal, lexer.TokenTime, lexer.TokenRealtime, lexer.TokenEvent:
		p.parseNetDeclItem(mod)
	case lexer.TokenInterface:
		// `virtual interface bus_if [.master] vif;`
		p.advance() // consume 'interface'
		p.parseVirtualInterfaceVarDecl(mod, firstTok)
	default:
		if hasVirtual && p.cur().Type == lexer.TokenIdent {
			// `virtual bus_if [#(...)] [.master] vif;`
			p.parseVirtualInterfaceVarDecl(mod, firstTok)
			return
		}
		if p.cur().Type == lexer.TokenIdent && p.isUserTypeVarDecl() {
			p.parseUserTypeVarDecl(mod)
			return
		}
		// Implicit logic variable after `var`/`const`/`static`, e.g. `var [7:0] x = 0;` or `const MAX = 8;`
		kind := strings.Join(mods, " ")
		signed := false
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			signed = p.cur().Type == lexer.TokenSigned
			p.advance()
		}
		var width *ast.BitRange
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if width == nil {
				width = r
			}
		}
		for !p.isEOF() {
			if p.cur().Type != lexer.TokenIdent {
				p.addError(p.cur().Range, fmt.Sprintf("expected variable identifier after %q", kind))
				p.syncToStatementEnd()
				return
			}
			nameTok := p.advance()
			endPos := nameTok.Range.End
			for p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(mod)
				endPos = p.prev().Range.End
			}
			if p.cur().Type == lexer.TokenAssignOp {
				p.advance()
				_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
			}
			if mod != nil {
				mod.Items = append(mod.Items, &ast.NetDecl{
					Kind:      kind,
					Signed:    signed,
					Width:     width,
					Name:      nameTok.Literal,
					NameRange: nameTok.Range,
					Range:     ast.Range{File: firstTok.Range.File, Start: firstTok.Range.Start, End: endPos},
				})
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}
			break
		}
		p.expectSemicolon(kind + " declaration")
	}
}

func (p *Parser) parseVirtualInterfaceVarDecl(mod *ast.ModuleDecl, firstTok lexer.Token) {
	if p.cur().Type != lexer.TokenIdent {
		p.addError(p.cur().Range, "expected interface type name after 'virtual'")
		p.syncToStatementEnd()
		return
	}
	ifTok := p.advance()
	p.recordRef(nil, mod, ifTok)
	if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
		p.advance()
		p.parseBalancedParens(nil, mod)
	}
	if p.cur().Type == lexer.TokenDot && p.peekN(1).Type == lexer.TokenIdent {
		p.advance() // '.'
		mpTok := p.advance()
		p.recordMemberRef(mod, ifTok.Literal, mpTok)
	}
	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected virtual interface variable name")
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End
		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}
		if mod != nil {
			mod.Items = append(mod.Items, &ast.NetDecl{
				Kind:      ifTok.Literal,
				Name:      nameTok.Literal,
				NameRange: nameTok.Range,
				Range:     ast.Range{File: firstTok.Range.File, Start: firstTok.Range.Start, End: endPos},
			})
		}
		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}
	p.expectSemicolon("virtual interface declaration")
}

func (p *Parser) parseClassDecl() *ast.ClassDecl {
	startPos := p.cur().Range.Start
	isVirtual := false
	if p.cur().Type == lexer.TokenVirtual {
		isVirtual = true
		p.advance()
	}
	return p.parseClassDeclWithVirtual(isVirtual, startPos)
}

func (p *Parser) parseClassDeclWithVirtual(isVirtual bool, startPos ast.Position) *ast.ClassDecl {
	classTok := p.advance() // consume 'class'
	if p.cur().Type == lexer.TokenAutomatic || p.cur().Type == lexer.TokenStatic {
		p.advance()
	}
	cls := &ast.ClassDecl{
		Virtual: isVirtual,
		Range:   ast.Range{File: classTok.Range.File, Start: startPos, End: classTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		cls.Name = nameTok.Literal
		cls.NameRange = nameTok.Range
		if p.typeNames != nil {
			p.typeNames[cls.Name] = true
		}
	} else {
		p.addError(p.cur().Range, "expected class name after 'class'")
	}

	dummyMod := &ast.ModuleDecl{
		Name:      cls.Name,
		NameRange: cls.NameRange,
		Range:     cls.Range,
	}

	// Optional parameter port list: #(...)
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseModuleParamPortList(dummyMod)
		} else {
			p.addError(p.cur().Range, "expected '(' after '#' in class header")
		}
	}

	// Optional `extends BaseClass [#(...)] [(...)]`
	if p.cur().Type == lexer.TokenExtends {
		p.advance() // 'extends'
		if p.cur().Type == lexer.TokenIdent {
			baseName, baseTok := p.parseScopedTypeName(nil, dummyMod)
			cls.BaseClass = baseName
			cls.BaseClassRange = baseTok.Range
			if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
				cls.BaseTypeArgs = p.parseTypeParamArgs(nil, dummyMod)
			}
			if p.cur().Type == lexer.TokenLParen {
				p.parseBalancedParens(nil, dummyMod)
			}
		} else {
			p.addError(p.cur().Range, "expected base class name after 'extends'")
		}
	}

	p.expectSemicolon("class header")

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndclass &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenEndpackage &&
		p.cur().Type != lexer.TokenEndinterface &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage {
		p.parseModuleItem(dummyMod)
	}

	if p.cur().Type == lexer.TokenEndclass {
		p.advance()
		p.consumeOptionalEndLabel()
		cls.Range.End = p.prev().Range.End
	} else {
		p.addError(classTok.Range, fmt.Sprintf("expected 'endclass' to close class %q", cls.Name))
		cls.Range.End = p.prev().Range.End
	}

	cls.Params = dummyMod.Params
	cls.Items = dummyMod.Items
	cls.Refs = dummyMod.Refs
	cls.MemberRefs = dummyMod.MemberRefs
	return cls
}

func (p *Parser) parseConstraintDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'constraint'
	c := &ast.ConstraintDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		if p.cur().Type == lexer.TokenColonColon && p.peekN(1).Type == lexer.TokenIdent {
			p.advance() // '::'
			nameTok = p.advance()
		}
		c.Name = nameTok.Literal
		c.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected constraint name after 'constraint'")
	}

	// Prototype constraint: `extern constraint c_name;`
	if p.cur().Type == lexer.TokenSemicolon {
		semi := p.advance()
		c.Range.End = semi.Range.End
		if mod != nil {
			mod.Items = append(mod.Items, c)
		}
		return
	}

	if p.cur().Type != lexer.TokenLBrace {
		p.addError(p.cur().Range, fmt.Sprintf("expected '{' in constraint %q", c.Name))
		p.syncToStatementEnd()
		return
	}

	openTok := p.advance() // '{'
	depth := 1
	prevWasDot := false
	scopedPrefix := ""
	lastBase := ""
	bracketDepth := 0

	for !p.isEOF() && depth > 0 {
		if p.cur().Type == lexer.TokenEndclass || p.cur().Type == lexer.TokenEndmodule {
			break
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLBrace:
			depth++
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRBrace:
			depth--
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenColonColon:
			lastBase = ""
		case lexer.TokenThis, lexer.TokenSuper:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = tok.Literal
			}
		case lexer.TokenIdent:
			// Check for `foreach (arr[i])` loop variable inside constraint!
			if prevWasDot {
				if lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(mod, lastBase, tok)
					lastBase = lastBase + "." + tok.Literal
				} else {
					lastBase = ""
				}
				scopedPrefix = ""
			} else if p.cur().Type == lexer.TokenColonColon {
				scopedPrefix = tok.Literal
				lastBase = ""
			} else if scopedPrefix != "" {
				p.recordScopedRef(&c.Refs, mod, scopedPrefix+"::"+tok.Literal, tok.Range)
				scopedPrefix = ""
				lastBase = ""
			} else {
				p.recordRef(&c.Refs, mod, tok)
				if bracketDepth == 0 {
					lastBase = tok.Literal
				}
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}

	if depth != 0 {
		p.addError(openTok.Range, fmt.Sprintf("expected '}' to close constraint %q", c.Name))
	}
	c.Range.End = p.prev().Range.End
	if mod != nil {
		mod.Items = append(mod.Items, c)
	}
}

// parseForeachHeader parses `(arr[i, j])` in a SystemVerilog `foreach` statement,
// recording the array root as a reference and declaring loop index variables as block locals.
func (p *Parser) parseForeachHeader(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl) {
	openTok := p.advance() // '('
	prevWasDot := false
	inBrackets := false
	bracketDepth := 0

	for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
		if p.cur().Type == lexer.TokenSemicolon ||
			p.cur().Type == lexer.TokenEndmodule ||
			p.cur().Type == lexer.TokenEndfunction ||
			p.cur().Type == lexer.TokenEndtask {
			break
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLBracket:
			bracketDepth++
			inBrackets = true
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			if bracketDepth == 0 {
				inBrackets = false
			}
		case lexer.TokenIdent:
			if inBrackets && bracketDepth == 1 {
				// Loop variable declaration inside `foreach (arr[i, j])`
				if mod != nil {
					mod.Items = append(mod.Items, &ast.NetDecl{
						Kind:      "int",
						Name:      tok.Literal,
						NameRange: tok.Range,
						Range:     tok.Range,
					})
				}
			} else if !prevWasDot {
				p.recordRef(refs, mod, tok)
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}

	if p.cur().Type == lexer.TokenRParen {
		p.advance()
	} else {
		p.addError(openTok.Range, "expected ')' to close 'foreach' header")
	}
}

func (p *Parser) parseUserTypeVarDecl(mod *ast.ModuleDecl) {
	typeName, typeTok := p.parseScopedTypeName(nil, mod)

	var typeArgs []ast.TypeParamArg
	if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
		typeArgs = p.parseTypeParamArgs(nil, mod)
	}

	var width *ast.BitRange
	for p.cur().Type == lexer.TokenLBracket {
		r := p.parseBitRange(mod)
		if width == nil {
			width = r
		}
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected variable identifier after type %q", typeName))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		var unpackedDims []*ast.BitRange
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			if r != nil {
				unpackedDims = append(unpackedDims, r)
			}
			endPos = p.prev().Range.End
		}
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}

		if mod != nil {
			mod.Items = append(mod.Items, &ast.NetDecl{
				Kind:         typeName,
				TypeArgs:     typeArgs,
				Width:        width,
				UnpackedDims: unpackedDims,
				Name:      nameTok.Literal,
				NameRange: nameTok.Range,
				Range:     ast.Range{File: typeTok.Range.File, Start: typeTok.Range.Start, End: endPos},
			})
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon("variable declaration")
}

// parseTypeParamArgs parses `#(arg1, .P(arg2), ...)` in a parameterized class or type instantiation.
func (p *Parser) parseTypeParamArgs(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl) []ast.TypeParamArg {
	p.advance() // '#'
	if p.cur().Type != lexer.TokenLParen {
		return nil
	}
	p.advance() // '('

	var args []ast.TypeParamArg
	for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
		startPos := p.cur().Range.Start
		paramName := ""
		if p.cur().Type == lexer.TokenDot && p.peekN(1).Type == lexer.TokenIdent {
			p.advance() // '.'
			paramName = p.advance().Literal
			if p.cur().Type == lexer.TokenLParen {
				p.advance() // '('
				typeStr, endPos := p.parseSingleTypeParamValue(itemRefs, mod, lexer.TokenRParen)
				if p.cur().Type == lexer.TokenRParen {
					endPos = p.advance().Range.End
				}
				args = append(args, ast.TypeParamArg{
					ParamName: paramName,
					TypeName:  typeStr,
					Range:     ast.Range{File: p.cur().Range.File, Start: startPos, End: endPos},
				})
			}
		} else {
			typeStr, endPos := p.parseSingleTypeParamValue(itemRefs, mod, lexer.TokenComma, lexer.TokenRParen)
			args = append(args, ast.TypeParamArg{
				TypeName: typeStr,
				Range:    ast.Range{File: p.cur().Range.File, Start: startPos, End: endPos},
			})
		}
		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}
	if p.cur().Type == lexer.TokenRParen {
		p.advance()
	}
	return args
}

// parseSingleTypeParamValue parses one type or value expression inside `#(...)` up to stop tokens,
// recording identifier references and returning the normalized type/value string.
func (p *Parser) parseSingleTypeParamValue(itemRefs *[]ast.IdentifierRef, mod *ast.ModuleDecl, stops ...lexer.TokenType) (string, ast.Position) {
	isStop := func(t lexer.TokenType) bool {
		for _, s := range stops {
			if t == s {
				return true
			}
		}
		return false
	}

	var parts []string
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	endPos := p.cur().Range.Start

	for !p.isEOF() {
		tok := p.cur()
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && isStop(tok.Type) {
			break
		}
		if tok.Type == lexer.TokenSemicolon || tok.Type == lexer.TokenEndmodule || tok.Type == lexer.TokenEndclass {
			break
		}
		if tok.Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenColonColon && p.peekN(2).Type == lexer.TokenIdent {
			scoped, _ := p.parseScopedTypeName(itemRefs, mod)
			parts = append(parts, scoped)
			endPos = p.prev().Range.End
			continue
		}
		p.advance()
		endPos = tok.Range.End
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
			p.recordRef(itemRefs, mod, tok)
		}
		parts = append(parts, tok.Literal)
	}
	return strings.TrimSpace(strings.Join(parts, " ")), endPos
}

func (p *Parser) parsePropertyOrSequenceDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // 'property' or 'sequence'
	endKw := lexer.TokenEndproperty
	if startTok.Type == lexer.TokenSequence {
		endKw = lexer.TokenEndsequence
	}

	prop := &ast.PropertyDecl{
		Kind:  startTok.Literal,
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		prop.Name = nameTok.Literal
		prop.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, fmt.Sprintf("expected %s name after %q", prop.Kind, prop.Kind))
	}

	tempMod := &ast.ModuleDecl{}

	// Optional formal argument list: `(clk, req, ack)` or `(logic clk, int delay = 1)`
	if p.cur().Type == lexer.TokenLParen {
		p.parseSVAFormalPortList(prop, tempMod)
	}

	p.expectSemicolon(prop.Kind + " header")

	prevWasDot := false
	scopedPrefix := ""
	lastBase := ""
	bracketDepth := 0

	for !p.isEOF() &&
		p.cur().Type != endKw &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenEndinterface &&
		p.cur().Type != lexer.TokenEndpackage &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage {
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
			prevWasDot = false
			scopedPrefix = ""
			lastBase = ""
			continue
		}
		// Optional local variable declaration inside property/sequence (e.g. `int v;`, `logic [7:0] x;`)
		if isPortNetType(p.cur().Type) && (p.peekN(1).Type == lexer.TokenIdent || p.peekN(1).Type == lexer.TokenLBracket || p.peekN(1).Type == lexer.TokenSigned || p.peekN(1).Type == lexer.TokenUnsigned) {
			p.parseNetDeclItem(tempMod)
			prevWasDot = false
			scopedPrefix = ""
			lastBase = ""
			continue
		}
		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenColonColon:
			lastBase = ""
		case lexer.TokenThis, lexer.TokenSuper:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = tok.Literal
			}
		case lexer.TokenIdent:
			if !prevWasDot {
				if p.cur().Type == lexer.TokenColonColon {
					scopedPrefix = tok.Literal
					lastBase = ""
				} else if scopedPrefix != "" {
					p.recordScopedRef(&prop.Refs, tempMod, scopedPrefix+"::"+tok.Literal, tok.Range)
					scopedPrefix = ""
					lastBase = ""
				} else {
					p.recordRef(&prop.Refs, tempMod, tok)
					if bracketDepth == 0 {
						lastBase = tok.Literal
					}
				}
			} else {
				if lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(tempMod, lastBase, tok)
				}
				scopedPrefix = ""
				lastBase = ""
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}

	if p.cur().Type == endKw {
		p.advance()
		p.consumeOptionalEndLabel()
		prop.Range.End = p.prev().Range.End
	} else {
		p.addError(startTok.Range, fmt.Sprintf("expected '%s' to close %s %q", endKw, prop.Kind, prop.Name))
		prop.Range.End = p.prev().Range.End
	}

	prop.Locals = tempMod.Items
	if mod != nil {
		mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
		mod.Items = append(mod.Items, prop)
	}
}

func (p *Parser) parseSVAFormalPortList(prop *ast.PropertyDecl, tempMod *ast.ModuleDecl) {
	openTok := p.advance() // '('
	curNetType := ""
	curSigned := false
	var curWidth *ast.BitRange

	for !p.isEOF() && p.cur().Type != lexer.TokenRParen && p.cur().Type != lexer.TokenSemicolon {
		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		startPos := p.cur().Range.Start
		if isPortDirection(p.cur().Type) || p.cur().Type == lexer.TokenLocalparam {
			p.advance()
		}
		if isPortNetType(p.cur().Type) || p.cur().Type == lexer.TokenProperty || p.cur().Type == lexer.TokenSequence || p.cur().Type == lexer.TokenEvent {
			curNetType = p.advance().Literal
			curSigned = false
			curWidth = nil
		} else if p.isUserTypeBeforeIdent() {
			curNetType, _ = p.parseScopedTypeName(nil, tempMod)
			curSigned = false
			curWidth = nil
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			curSigned = p.cur().Type == lexer.TokenSigned
			p.advance()
		}
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(tempMod)
			if curWidth == nil {
				curWidth = r
			}
		}
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected formal argument name in "+prop.Kind)
			p.syncToParamOrPortDelimiter()
			if p.cur().Type == lexer.TokenComma {
				p.advance()
			}
			continue
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End
		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(tempMod)
			endPos = p.prev().Range.End
		}
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(tempMod, lexer.TokenComma, lexer.TokenRParen)
		}
		prop.Params = append(prop.Params, &ast.PortDecl{
			Direction: "input",
			NetType:   curNetType,
			Signed:    curSigned,
			Width:     curWidth,
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			Range:     ast.Range{File: nameTok.Range.File, Start: startPos, End: endPos},
		})
		if p.cur().Type == lexer.TokenComma {
			p.advance()
		}
	}

	if p.cur().Type == lexer.TokenRParen {
		p.advance()
	} else {
		p.addError(openTok.Range, "expected ')' to close "+prop.Kind+" argument list")
	}
}

func (p *Parser) parseAssertItem(mod *ast.ModuleDecl, labelTok *lexer.Token) {
	dummyBlock := &ast.ProceduralBlock{Kind: "assert"}
	stmt := p.parseAssertStmt(dummyBlock, mod, labelTok)
	if stmt != nil && mod != nil {
		mod.Items = append(mod.Items, stmt)
	}
}

func (p *Parser) parseAssertStmt(block *ast.ProceduralBlock, mod *ast.ModuleDecl, labelTok *lexer.Token) *ast.AssertStmt {
	kwTok := p.advance() // assert / assume / cover / restrict
	startPos := kwTok.Range.Start
	stmt := &ast.AssertStmt{
		Kind:       kwTok.Literal,
		Concurrent: kwTok.Type == lexer.TokenExpect,
	}
	if labelTok != nil {
		startPos = labelTok.Range.Start
		stmt.Label = labelTok.Literal
		stmt.LabelRange = labelTok.Range
		if block != nil && labelTok.Literal != "" {
			block.Labels = append(block.Labels, labelTok.Literal)
		}
	}

	// Optional concurrent (`property` / `sequence`) or deferred (`#0` / `final`) qualifier
	if p.cur().Type == lexer.TokenProperty || p.cur().Type == lexer.TokenSequence {
		stmt.Concurrent = true
		p.advance()
	} else if p.cur().Type == lexer.TokenFinal {
		p.advance()
	} else if p.cur().Type == lexer.TokenHash {
		p.advance()
		if p.cur().Type == lexer.TokenNumber || p.cur().Type == lexer.TokenIdent {
			p.advance()
		}
	}

	var targetRefs *[]ast.IdentifierRef
	if block != nil {
		targetRefs = &block.Refs
	} else {
		targetRefs = &stmt.Refs
	}

	if p.cur().Type == lexer.TokenLParen {
		p.parseBalancedParens(targetRefs, mod)
	} else {
		p.addError(kwTok.Range, fmt.Sprintf("expected '(' after %q", kwTok.Literal))
	}

	// Action block: `;` OR `[pass_stmt] [else fail_stmt]`
	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
	} else if p.cur().Type == lexer.TokenElse {
		p.advance() // 'else'
		actionBlk := block
		if actionBlk == nil {
			actionBlk = &ast.ProceduralBlock{Kind: "assert"}
		}
		p.parseStatement(actionBlk, mod)
	} else if !p.isEOF() {
		actionBlk := block
		if actionBlk == nil {
			actionBlk = &ast.ProceduralBlock{Kind: "assert"}
		}
		p.parseStatement(actionBlk, mod)
		if p.cur().Type == lexer.TokenElse {
			p.advance() // 'else'
			p.parseStatement(actionBlk, mod)
		}
	}

	stmt.Range = ast.Range{File: kwTok.Range.File, Start: startPos, End: p.prev().Range.End}
	return stmt
}

func (p *Parser) parseInterfaceDecl() *ast.InterfaceDecl {
	startTok := p.advance() // consume 'interface'
	if p.cur().Type == lexer.TokenAutomatic || p.cur().Type == lexer.TokenStatic {
		p.advance()
	}
	iface := &ast.InterfaceDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		iface.Name = nameTok.Literal
		iface.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected interface name after 'interface'")
	}

	dummyMod := &ast.ModuleDecl{
		Name:      iface.Name,
		NameRange: iface.NameRange,
		Range:     iface.Range,
	}

	// Optional SystemVerilog header package imports: `interface bus_if import pkg::*; #(...) (...);`
	for p.cur().Type == lexer.TokenImport {
		p.parseImportDecl(dummyMod, nil)
	}

	// Optional parameter port list: #(...)
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseModuleParamPortList(dummyMod)
		} else {
			p.addError(p.cur().Range, "expected '(' after '#' in interface header")
		}
	}

	// Optional port list: (...)
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(dummyMod)
	}

	if p.cur().Type == lexer.TokenSemicolon {
		p.advance()
	} else {
		p.addError(p.prev().Range, "expected ';' after interface header")
	}

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndinterface &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenMacromodule &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage &&
		p.cur().Type != lexer.TokenPrimitive {
		p.parseModuleItem(dummyMod)
	}

	if p.cur().Type == lexer.TokenEndinterface {
		p.advance()
		p.consumeOptionalEndLabel()
		iface.Range.End = p.prev().Range.End
	} else {
		p.addError(p.prev().Range, fmt.Sprintf("expected 'endinterface' to close interface %q", iface.Name))
		iface.Range.End = p.prev().Range.End
	}

	iface.Imports = dummyMod.Imports
	iface.Params = dummyMod.Params
	iface.Ports = dummyMod.Ports
	iface.Items = dummyMod.Items
	iface.Refs = dummyMod.Refs
	iface.MemberRefs = dummyMod.MemberRefs
	for _, item := range dummyMod.Items {
		if mp, ok := item.(*ast.ModportDecl); ok {
			iface.Modports = append(iface.Modports, mp)
		}
	}
	return iface
}

func isTimeUnit(lit string) bool {
	switch lit {
	case "step", "fs", "ps", "ns", "us", "ms", "s":
		return true
	default:
		return false
	}
}

func (p *Parser) parseClockingDecl(mod *ast.ModuleDecl, isDefault, isGlobal bool, startPos ast.Position) {
	clkTok := p.advance() // consume 'clocking'
	cb := &ast.ClockingDecl{
		Default: isDefault,
		Global:  isGlobal,
		Range:   ast.Range{File: clkTok.Range.File, Start: startPos, End: clkTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		cb.Name = nameTok.Literal
		cb.NameRange = nameTok.Range
	}

	// Reference to an existing clocking block: `default clocking cb;`
	if isDefault && cb.Name != "" && p.cur().Type == lexer.TokenSemicolon {
		semi := p.advance()
		cb.Range.End = semi.Range.End
		p.recordScopedRef(&cb.Refs, mod, cb.Name, cb.NameRange)
		return
	}

	// Clocking event: `@(posedge clk)` or `@clk`
	if p.cur().Type == lexer.TokenAt {
		p.advance() // '@'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&cb.Refs, mod)
		} else if p.cur().Type == lexer.TokenIdent {
			evTok := p.advance()
			p.recordRef(&cb.Refs, mod, evTok)
		} else {
			p.addError(p.cur().Range, "expected clocking event after '@'")
		}
	} else {
		p.addError(p.cur().Range, "expected '@' clocking event in clocking header")
	}

	p.expectSemicolon("clocking header")

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndclocking &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenEndprogram &&
		p.cur().Type != lexer.TokenEndchecker &&
		p.cur().Type != lexer.TokenEndinterface &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenProgram &&
		p.cur().Type != lexer.TokenChecker &&
		p.cur().Type != lexer.TokenInterface {
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
			continue
		}
		if p.cur().Type == lexer.TokenDefault {
			// `default input #1step output #1ns;`
			p.advance()
			for !p.isEOF() && p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenEndclocking {
				if isPortDirection(p.cur().Type) {
					p.advance()
				}
				p.skipClockingSkew(&cb.Refs, mod)
				if p.cur().Type != lexer.TokenSemicolon && !isPortDirection(p.cur().Type) && p.cur().Type != lexer.TokenEndclocking {
					p.advance()
				}
			}
			p.expectSemicolon("clocking default skew")
			continue
		}
		if p.cur().Type == lexer.TokenInput || p.cur().Type == lexer.TokenOutput || p.cur().Type == lexer.TokenInout {
			dirTok := p.advance()
			dir := dirTok.Literal
			if dir == "input" && p.cur().Type == lexer.TokenOutput {
				p.advance()
				dir = "inout"
			}
			p.skipClockingSkew(&cb.Refs, mod)
			if dir == "inout" && p.cur().Type == lexer.TokenOutput {
				p.advance()
				p.skipClockingSkew(&cb.Refs, mod)
			}
			for !p.isEOF() {
				if p.cur().Type != lexer.TokenIdent {
					p.addError(p.cur().Range, fmt.Sprintf("expected signal identifier after %q in clocking block", dir))
					p.syncToStatementEnd()
					break
				}
				sigTok := p.advance()
				endPos := sigTok.Range.End
				if p.cur().Type == lexer.TokenAssignOp {
					p.advance() // '=' hierarchical expr
					_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
				} else {
					p.recordRef(&cb.Refs, mod, sigTok)
				}
				cb.Signals = append(cb.Signals, ast.ClockingSignal{
					Direction: dir,
					Name:      sigTok.Literal,
					NameRange: sigTok.Range,
					Range:     ast.Range{File: dirTok.Range.File, Start: dirTok.Range.Start, End: endPos},
				})
				if p.cur().Type == lexer.TokenComma {
					p.advance()
					continue
				}
				break
			}
			p.expectSemicolon("clocking signal declaration")
			continue
		}
		if p.cur().Type == lexer.TokenProperty || p.cur().Type == lexer.TokenSequence {
			p.parsePropertyOrSequenceDecl(mod)
			continue
		}
		if isAssertionKeyword(p.cur().Type) {
			p.parseAssertItem(mod, nil)
			continue
		}
		p.addError(p.cur().Range, fmt.Sprintf("unexpected token %q in clocking block", p.cur().Literal))
		p.advance()
		p.syncToStatementEnd()
	}

	if p.cur().Type == lexer.TokenEndclocking {
		p.advance()
		p.consumeOptionalEndLabel()
		cb.Range.End = p.prev().Range.End
	} else {
		p.addError(clkTok.Range, fmt.Sprintf("expected 'endclocking' to close clocking block %q", cb.Name))
		cb.Range.End = p.prev().Range.End
	}

	if mod != nil {
		mod.Items = append(mod.Items, cb)
	}
}

func (p *Parser) skipClockingSkew(refs *[]ast.IdentifierRef, mod *ast.ModuleDecl) {
	if p.cur().Type == lexer.TokenPosedge || p.cur().Type == lexer.TokenNegedge {
		p.advance()
	}
	if p.cur().Type == lexer.TokenHash {
		p.advance() // '#'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(refs, mod)
		} else if p.cur().Type == lexer.TokenNumber {
			p.advance()
			if p.cur().Type == lexer.TokenIdent && isTimeUnit(p.cur().Literal) {
				p.advance()
			}
		} else if p.cur().Type == lexer.TokenIdent {
			tok := p.advance()
			if !isTimeUnit(tok.Literal) {
				p.recordRef(refs, mod, tok)
			}
		}
	}
}

func (p *Parser) parseBindStmt(mod *ast.ModuleDecl, file *ast.SourceFile) {
	startTok := p.advance() // consume 'bind'
	stmt := &ast.BindStmt{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type != lexer.TokenIdent {
		p.addError(p.cur().Range, "expected target module or instance name after 'bind'")
		p.syncToStatementEnd()
		return
	}
	targetTok := p.advance()
	stmt.TargetName = targetTok.Literal
	stmt.TargetNameRange = targetTok.Range

	// Optional hierarchical path on target: `bind top.u_sub[0].u_leaf ...`
	for p.cur().Type == lexer.TokenDot || p.cur().Type == lexer.TokenLBracket {
		stmt.IsHierarchical = true
		if p.cur().Type == lexer.TokenDot {
			p.advance() // '.'
			if p.cur().Type == lexer.TokenIdent {
				p.advance()
			}
		} else {
			_ = p.parseBitRange(nil)
		}
	}

	// Optional target instance list: `bind dut_mod : u_dut0, u_dut1 checker_mod u_chk (...);`
	if p.cur().Type == lexer.TokenColon {
		p.advance() // ':'
		for !p.isEOF() {
			if p.cur().Type != lexer.TokenIdent {
				p.addError(p.cur().Range, "expected target instance name after ':' in bind statement")
				p.syncToStatementEnd()
				return
			}
			instTok := p.advance()
			stmt.TargetInstances = append(stmt.TargetInstances, ast.IdentifierRef{
				Name:  instTok.Literal,
				Range: instTok.Range,
			})
			for p.cur().Type == lexer.TokenDot || p.cur().Type == lexer.TokenLBracket {
				if p.cur().Type == lexer.TokenDot {
					p.advance()
					if p.cur().Type == lexer.TokenIdent {
						p.advance()
					}
				} else {
					_ = p.parseBitRange(nil)
				}
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}
			break
		}
	}

	// Bound instantiation: `checker_mod [#(...)] u_chk (...);`
	if p.cur().Type != lexer.TokenIdent {
		p.addError(p.cur().Range, "expected module or interface instantiation in bind statement")
		p.syncToStatementEnd()
		return
	}

	bindMod := &ast.ModuleDecl{}
	p.parseModuleInstOrUnknown(bindMod)
	for _, item := range bindMod.Items {
		if inst, ok := item.(*ast.ModuleInst); ok {
			stmt.Inst = inst
			break
		}
	}
	stmt.Range.End = p.prev().Range.End

	if mod != nil {
		mod.Items = append(mod.Items, stmt)
	} else if file != nil {
		file.Binds = append(file.Binds, stmt)
	}
}

func (p *Parser) parseCovergroupDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'covergroup'
	cg := &ast.CovergroupDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		cg.Name = nameTok.Literal
		cg.NameRange = nameTok.Range
		if p.typeNames != nil {
			p.typeNames[cg.Name] = true
		}
	}

	tempMod := &ast.ModuleDecl{}

	// Optional formal argument list: `(ref logic [7:0] addr, input int limit = 10)`
	if p.cur().Type == lexer.TokenLParen {
		p.parseModulePortList(tempMod)
		cg.Params = tempMod.Ports
	}

	// Optional sampling event: `@(posedge clk iff rst_n)` or `@clk` or `@@(begin t_a)`
	for p.cur().Type == lexer.TokenAt {
		p.advance() // '@'
		if p.cur().Type == lexer.TokenAt {
			p.advance() // second '@'
		}
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&cg.Refs, tempMod)
		} else if p.cur().Type == lexer.TokenIdent {
			evTok := p.advance()
			p.recordRef(&cg.Refs, tempMod, evTok)
		}
	}

	p.expectSemicolon("covergroup header")

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndgroup &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenEndprogram &&
		p.cur().Type != lexer.TokenEndchecker &&
		p.cur().Type != lexer.TokenEndinterface &&
		p.cur().Type != lexer.TokenEndpackage &&
		p.cur().Type != lexer.TokenEndclass &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenProgram &&
		p.cur().Type != lexer.TokenChecker &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage {
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
			continue
		}

		// `option.xxx = expr;` or `type_option.xxx = expr;`
		if p.cur().Type == lexer.TokenIdent &&
			(p.cur().Literal == "option" || p.cur().Literal == "type_option") &&
			p.peekN(1).Type == lexer.TokenDot {
			p.advance() // option / type_option
			p.advance() // '.'
			if p.cur().Type == lexer.TokenIdent {
				p.advance()
			}
			if p.cur().Type == lexer.TokenAssignOp {
				p.advance()
				p.collectTokensUntil(&cg.Refs, tempMod, lexer.TokenSemicolon)
			}
			p.expectSemicolon("covergroup option")
			continue
		}

		// Optional data type before labeled coverpoint: e.g. `logic [7:0] cp_a: coverpoint a;`
		if isPortNetType(p.cur().Type) || p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			p.advance()
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(tempMod)
			}
		}

		// Labeled coverpoint or cross: `cp_addr: coverpoint addr ...` or `cr_ab: cross cp_a, cp_b ...`
		if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenColon &&
			(p.peekN(2).Type == lexer.TokenCoverpoint || p.peekN(2).Type == lexer.TokenCross) {
			lblTok := p.advance()
			p.advance() // ':'
			p.parseCoverpointOrCross(cg, tempMod, &lblTok)
			continue
		}

		// Unlabeled coverpoint or cross
		if p.cur().Type == lexer.TokenCoverpoint || p.cur().Type == lexer.TokenCross {
			p.parseCoverpointOrCross(cg, tempMod, nil)
			continue
		}

		p.addError(p.cur().Range, fmt.Sprintf("unexpected token %q in covergroup %q", p.cur().Literal, cg.Name))
		p.advance()
		p.syncToStatementEnd()
	}

	if p.cur().Type == lexer.TokenEndgroup {
		p.advance()
		p.consumeOptionalEndLabel()
		cg.Range.End = p.prev().Range.End
	} else {
		p.addError(startTok.Range, fmt.Sprintf("expected 'endgroup' to close covergroup %q", cg.Name))
		cg.Range.End = p.prev().Range.End
	}

	if mod != nil {
		mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
		mod.Items = append(mod.Items, cg)
	}
}

func (p *Parser) parseCoverpointOrCross(cg *ast.CovergroupDecl, tempMod *ast.ModuleDecl, labelTok *lexer.Token) {
	kwTok := p.advance() // 'coverpoint' or 'cross'
	startPos := kwTok.Range.Start
	if labelTok != nil {
		startPos = labelTok.Range.Start
	}

	// Collect target expression(s) and optional `iff (cond)` up to `{` or `;`
	p.collectTokensUntil(&cg.Refs, tempMod, lexer.TokenLBrace, lexer.TokenSemicolon, lexer.TokenEndgroup)

	if p.cur().Type == lexer.TokenLBrace {
		p.parseCoverBinsBody(cg, tempMod)
		// Optional trailing ';' after '}'
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
		}
	} else {
		p.expectSemicolon(kwTok.Literal)
	}

	if labelTok != nil && labelTok.Literal != "" {
		cg.Points = append(cg.Points, ast.CoverpointDecl{
			Kind:      kwTok.Literal,
			Name:      labelTok.Literal,
			NameRange: labelTok.Range,
			Range:     ast.Range{File: kwTok.Range.File, Start: startPos, End: p.prev().Range.End},
		})
	}
}

func (p *Parser) parseCoverBinsBody(cg *ast.CovergroupDecl, tempMod *ast.ModuleDecl) {
	openTok := p.advance() // '{'
	depth := 1
	prevWasDot := false
	scopedPrefix := ""
	lastBase := ""
	bracketDepth := 0

	for !p.isEOF() && depth > 0 {
		if p.cur().Type == lexer.TokenEndgroup ||
			p.cur().Type == lexer.TokenEndmodule ||
			p.cur().Type == lexer.TokenEndprogram ||
			p.cur().Type == lexer.TokenEndchecker ||
			p.cur().Type == lexer.TokenEndinterface ||
			p.cur().Type == lexer.TokenEndclass {
			break
		}
		if p.cur().Type == lexer.TokenWildcard {
			p.advance()
			continue
		}
		if p.cur().Type == lexer.TokenBins || p.cur().Type == lexer.TokenIgnoreBins || p.cur().Type == lexer.TokenIllegalBins {
			p.advance() // bins / ignore_bins / illegal_bins
			// Consume declared bin name (not a signal reference)
			if p.cur().Type == lexer.TokenIdent {
				p.advance()
			}
			// Optional bin array `[N]` or `[]`
			if p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(tempMod)
			}
			prevWasDot = false
			scopedPrefix = ""
			lastBase = ""
			continue
		}

		tok := p.advance()
		switch tok.Type {
		case lexer.TokenLBrace:
			depth++
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenRBrace:
			depth--
			scopedPrefix = ""
			lastBase = ""
		case lexer.TokenLBracket:
			bracketDepth++
			scopedPrefix = ""
		case lexer.TokenRBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			scopedPrefix = ""
		case lexer.TokenDot:
			scopedPrefix = ""
		case lexer.TokenColonColon:
			lastBase = ""
		case lexer.TokenIdent:
			if tok.Literal == "option" || tok.Literal == "type_option" || tok.Literal == "item" {
				scopedPrefix = ""
				lastBase = ""
				break
			}
			if prevWasDot {
				if lastBase != "" && bracketDepth == 0 {
					p.recordMemberRef(tempMod, lastBase, tok)
				}
				scopedPrefix = ""
				lastBase = ""
			} else if p.cur().Type == lexer.TokenColonColon {
				scopedPrefix = tok.Literal
				lastBase = ""
			} else if scopedPrefix != "" {
				p.recordScopedRef(&cg.Refs, tempMod, scopedPrefix+"::"+tok.Literal, tok.Range)
				scopedPrefix = ""
				lastBase = ""
			} else {
				p.recordRef(&cg.Refs, tempMod, tok)
				if bracketDepth == 0 {
					lastBase = tok.Literal
				}
			}
		case lexer.TokenIllegal:
			p.addError(tok.Range, tok.Literal)
			scopedPrefix = ""
			lastBase = ""
		default:
			scopedPrefix = ""
			if bracketDepth == 0 {
				lastBase = ""
			}
		}
		prevWasDot = tok.Type == lexer.TokenDot
	}

	if depth != 0 {
		p.addError(openTok.Range, "expected '}' to close coverpoint/cross bins")
	}
}

func (p *Parser) parseModportDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'modport'

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected modport name after 'modport'")
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		mp := &ast.ModportDecl{
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: nameTok.Range.End},
		}

		if p.cur().Type != lexer.TokenLParen {
			p.addError(p.cur().Range, fmt.Sprintf("expected '(' after modport name %q", mp.Name))
			p.syncToStatementEnd()
			return
		}
		openTok := p.advance() // '('

		curDir := ""
		for !p.isEOF() && p.cur().Type != lexer.TokenRParen && p.cur().Type != lexer.TokenSemicolon && p.cur().Type != lexer.TokenEndinterface {
			p.skipAttributes()
			if p.cur().Type == lexer.TokenRParen {
				break
			}
			if p.cur().Type == lexer.TokenComma {
				p.advance()
				continue
			}

			if isPortDirection(p.cur().Type) || p.cur().Type == lexer.TokenImport || p.cur().Type == lexer.TokenExport || p.cur().Type == lexer.TokenClocking {
				curDir = p.advance().Literal
			}

			// Handle task/function prototype inside modport: `import function void foo(input int x)` or `export task bar()`
			if (curDir == "import" || curDir == "export") &&
				(p.cur().Type == lexer.TokenFunction || p.cur().Type == lexer.TokenTask) {
				p.advance() // function / task
				if p.cur().Type == lexer.TokenAutomatic {
					p.advance()
				}
				// Skip optional return type tokens up to function/task name + '(' or ',' / ')'
				var lastIdent lexer.Token
				for !p.isEOF() &&
					p.cur().Type != lexer.TokenLParen &&
					p.cur().Type != lexer.TokenComma &&
					p.cur().Type != lexer.TokenRParen &&
					p.cur().Type != lexer.TokenSemicolon {
					tok := p.advance()
					if tok.Type == lexer.TokenIdent {
						lastIdent = tok
					}
				}
				if p.cur().Type == lexer.TokenLParen {
					p.parseBalancedParens(nil, nil)
				}
				if lastIdent.Literal != "" {
					mp.Ports = append(mp.Ports, ast.ModportPort{
						Direction: curDir,
						Name:      lastIdent.Literal,
						NameRange: lastIdent.Range,
					})
				}
				if p.cur().Type == lexer.TokenComma {
					p.advance()
				}
				continue
			}

			// Handle explicit port expression in modport: `.port_name(expr)`
			if p.cur().Type == lexer.TokenDot {
				p.advance() // '.'
				if p.cur().Type == lexer.TokenIdent {
					portTok := p.advance()
					mp.Ports = append(mp.Ports, ast.ModportPort{
						Direction: curDir,
						Name:      portTok.Literal,
						NameRange: portTok.Range,
					})
				}
				if p.cur().Type == lexer.TokenLParen {
					p.parseBalancedParens(nil, mod)
				}
				if p.cur().Type == lexer.TokenComma {
					p.advance()
				}
				continue
			}

			if p.cur().Type == lexer.TokenIdent {
				sigTok := p.advance()
				p.recordRef(nil, mod, sigTok)
				mp.Ports = append(mp.Ports, ast.ModportPort{
					Direction: curDir,
					Name:      sigTok.Literal,
					NameRange: sigTok.Range,
				})
				if p.cur().Type == lexer.TokenComma {
					p.advance()
				}
				continue
			}

			p.addError(p.cur().Range, fmt.Sprintf("unexpected token %q in modport %q", p.cur().Literal, mp.Name))
			p.advance()
		}

		if p.cur().Type == lexer.TokenRParen {
			closeTok := p.advance()
			mp.Range.End = closeTok.Range.End
		} else {
			p.addError(openTok.Range, fmt.Sprintf("expected ')' to close modport %q", mp.Name))
			mp.Range.End = p.prev().Range.End
		}

		if mod != nil {
			mod.Items = append(mod.Items, mp)
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon("modport declaration")
}

func (p *Parser) parsePackageDecl() *ast.PackageDecl {
	startTok := p.advance() // consume 'package'
	if p.cur().Type == lexer.TokenAutomatic || p.cur().Type == lexer.TokenStatic {
		p.advance()
	}
	pkg := &ast.PackageDecl{
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}
	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		pkg.Name = nameTok.Literal
		pkg.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected package name after 'package'")
	}
	p.expectSemicolon("package header")

	dummyMod := &ast.ModuleDecl{}
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndpackage &&
		p.cur().Type != lexer.TokenModule &&
		p.cur().Type != lexer.TokenMacromodule &&
		p.cur().Type != lexer.TokenInterface &&
		p.cur().Type != lexer.TokenPackage {
		p.parseModuleItem(dummyMod)
	}

	if p.cur().Type == lexer.TokenEndpackage {
		p.advance()
		p.consumeOptionalEndLabel()
		pkg.Range.End = p.prev().Range.End
	} else {
		p.addError(startTok.Range, fmt.Sprintf("expected 'endpackage' to close package %q", pkg.Name))
		pkg.Range.End = p.prev().Range.End
	}

	pkg.Imports = dummyMod.Imports
	pkg.Params = dummyMod.Params
	pkg.Items = dummyMod.Items
	pkg.Refs = dummyMod.Refs
	pkg.MemberRefs = dummyMod.MemberRefs
	return pkg
}

func (p *Parser) parseImportDecl(mod *ast.ModuleDecl, file *ast.SourceFile) {
	startTok := p.advance() // consume 'import'

	// DPI-C import: `import "DPI-C" [pure|context] [c_name =] function|task ...;`
	if p.cur().Type == lexer.TokenString {
		p.parseDPIImportDecl(mod, file)
		return
	}

	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected package name after 'import'")
			p.syncToStatementEnd()
			return
		}
		pkgTok := p.advance()
		symName := ""
		var symRange ast.Range
		endPos := pkgTok.Range.End

		if p.cur().Type == lexer.TokenColonColon {
			p.advance() // '::'
			if p.cur().Type == lexer.TokenStar {
				starTok := p.advance()
				symName = "*"
				symRange = starTok.Range
				endPos = starTok.Range.End
			} else if p.cur().Type == lexer.TokenIdent {
				symTok := p.advance()
				symName = symTok.Literal
				symRange = symTok.Range
				endPos = symTok.Range.End
			} else {
				p.addError(p.cur().Range, "expected '*' or symbol name after '::' in import")
			}
		} else {
			p.addError(p.cur().Range, "expected '::' after package name in import")
		}

		imp := &ast.ImportDecl{
			PackageName:      pkgTok.Literal,
			PackageNameRange: pkgTok.Range,
			SymbolName:       symName,
			SymbolNameRange:  symRange,
			Range:            ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
		}
		if mod != nil {
			mod.Imports = append(mod.Imports, imp)
			mod.Items = append(mod.Items, imp)
		} else if file != nil {
			file.Imports = append(file.Imports, imp)
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon("import declaration")
}

func (p *Parser) parseDPIImportDecl(mod *ast.ModuleDecl, file *ast.SourceFile) {
	p.advance() // consume DPI spec string (e.g. "DPI-C" or "DPI")

	// Optional `pure` or `context` property
	if p.cur().Type == lexer.TokenPure || p.cur().Type == lexer.TokenContext {
		p.advance()
	}

	// Optional C identifier alias: `c_name =`
	if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenAssignOp {
		p.advance() // c_name
		p.advance() // '='
	}

	targetMod := mod
	if targetMod == nil {
		targetMod = &ast.ModuleDecl{}
	}

	switch p.cur().Type {
	case lexer.TokenFunction:
		p.parseFunctionDeclOpts(targetMod, true)
	case lexer.TokenTask:
		p.parseTaskDeclOpts(targetMod, true)
	default:
		p.addError(p.cur().Range, "expected 'function' or 'task' in DPI-C import declaration")
		p.syncToStatementEnd()
		return
	}

	if mod == nil && file != nil {
		for _, item := range targetMod.Items {
			switch d := item.(type) {
			case *ast.FunctionDecl:
				file.Functions = append(file.Functions, d)
			case *ast.TaskDecl:
				file.Tasks = append(file.Tasks, d)
			}
		}
		file.Refs = append(file.Refs, targetMod.Refs...)
	}
}

func (p *Parser) parseDPIExportDecl(mod *ast.ModuleDecl, file *ast.SourceFile) {
	startTok := p.advance() // consume 'export'

	// SystemVerilog package re-export: `export *::*;`, `export pkg::*;`, `export pkg::sym, ...;`
	if p.cur().Type == lexer.TokenStar || (p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenColonColon) {
		p.parsePackageExportDecl(startTok, mod, file)
		return
	}

	if p.cur().Type == lexer.TokenString {
		p.advance() // e.g. "DPI-C"
	} else {
		p.addError(p.cur().Range, "expected DPI specification string (e.g. \"DPI-C\") after 'export'")
		p.syncToStatementEnd()
		return
	}

	// Optional C identifier alias: `c_name =`
	if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenAssignOp {
		p.advance() // c_name
		p.advance() // '='
	}

	if p.cur().Type == lexer.TokenFunction || p.cur().Type == lexer.TokenTask {
		p.advance()
	} else {
		p.addError(p.cur().Range, "expected 'function' or 'task' in DPI-C export declaration")
		p.syncToStatementEnd()
		return
	}

	if p.cur().Type == lexer.TokenIdent {
		fnTok := p.advance()
		if mod != nil {
			p.recordRef(nil, mod, fnTok)
		} else if file != nil {
			file.Refs = append(file.Refs, ast.IdentifierRef{Name: fnTok.Literal, Range: fnTok.Range})
		}
	} else {
		p.addError(p.cur().Range, "expected function or task name in DPI-C export declaration")
	}

	p.expectSemicolon("DPI export declaration")
}

// parsePackageExportDecl parses SystemVerilog package re-export declarations:
// `export *::*;`, `export pkg::*;`, `export pkg::sym, ...;`
func (p *Parser) parsePackageExportDecl(startTok lexer.Token, mod *ast.ModuleDecl, file *ast.SourceFile) {
	for !p.isEOF() {
		var pkgTok lexer.Token
		if p.cur().Type == lexer.TokenStar {
			pkgTok = p.advance()
		} else if p.cur().Type == lexer.TokenIdent {
			pkgTok = p.advance()
		} else {
			p.addError(p.cur().Range, "expected package name or '*' after 'export'")
			p.syncToStatementEnd()
			return
		}

		symName := ""
		var symRange ast.Range
		endPos := pkgTok.Range.End

		if p.cur().Type == lexer.TokenColonColon {
			p.advance() // '::'
			if p.cur().Type == lexer.TokenStar {
				starTok := p.advance()
				symName = "*"
				symRange = starTok.Range
				endPos = starTok.Range.End
			} else if p.cur().Type == lexer.TokenIdent {
				symTok := p.advance()
				symName = symTok.Literal
				symRange = symTok.Range
				endPos = symTok.Range.End
			} else {
				p.addError(p.cur().Range, "expected '*' or symbol name after '::' in export")
			}
		} else {
			p.addError(p.cur().Range, "expected '::' after package name in export")
		}

		exp := &ast.ImportDecl{
			IsExport:         true,
			PackageName:      pkgTok.Literal,
			PackageNameRange: pkgTok.Range,
			SymbolName:       symName,
			SymbolNameRange:  symRange,
			Range:            ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
		}
		if mod != nil {
			mod.Imports = append(mod.Imports, exp)
			mod.Items = append(mod.Items, exp)
		} else if file != nil {
			file.Imports = append(file.Imports, exp)
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon("export declaration")
}

// parseLetDecl parses a SystemVerilog `let` declaration:
// `let name [( [type|untyped] arg [= default], ... )] = expr;`
func (p *Parser) parseLetDecl(mod *ast.ModuleDecl, file *ast.SourceFile) {
	startTok := p.advance() // consume 'let'
	fn := &ast.FunctionDecl{
		IsLet: true,
		Range: ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		fn.Name = nameTok.Literal
		fn.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected identifier after 'let'")
		p.syncToStatementEnd()
		return
	}

	tempMod := &ast.ModuleDecl{}

	// Optional formal argument list: `([type|untyped] arg [= default], ...)`
	if p.cur().Type == lexer.TokenLParen {
		p.advance() // '('
		for !p.isEOF() && p.cur().Type != lexer.TokenRParen {
			p.skipAttributes()
			if p.cur().Type == lexer.TokenRParen {
				break
			}
			argStart := p.cur().Range.Start
			argType := ""
			argSigned := false
			var argWidth *ast.BitRange

			if p.cur().Type == lexer.TokenUntyped || isPortNetType(p.cur().Type) || p.cur().Type == lexer.TokenEvent {
				argType = p.advance().Literal
				if isSVDataType(p.cur().Type) {
					p.advance()
				}
			} else if p.isUserTypeBeforeIdent() {
				argType, _ = p.parseScopedTypeName(nil, tempMod)
			}
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				argSigned = p.cur().Type == lexer.TokenSigned
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				r := p.parseBitRange(tempMod)
				if argWidth == nil {
					argWidth = r
				}
			}

			if p.cur().Type != lexer.TokenIdent {
				p.addError(p.cur().Range, "expected formal argument name in 'let' declaration")
				p.syncToParamOrPortDelimiter()
				if p.cur().Type == lexer.TokenComma {
					p.advance()
				}
				continue
			}
			argTok := p.advance()
			argEnd := argTok.Range.End

			for p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(tempMod)
				argEnd = p.prev().Range.End
			}
			if p.cur().Type == lexer.TokenAssignOp {
				p.advance() // '='
				_, argEnd = p.parseExprUntil(tempMod, lexer.TokenComma, lexer.TokenRParen)
			}

			fn.Params = append(fn.Params, &ast.PortDecl{
				Direction: "input",
				NetType:   argType,
				Signed:    argSigned,
				Width:     argWidth,
				Name:      argTok.Literal,
				NameRange: argTok.Range,
				Range:     ast.Range{File: argTok.Range.File, Start: argStart, End: argEnd},
			})

			if p.cur().Type == lexer.TokenComma {
				p.advance()
			} else if p.cur().Type != lexer.TokenRParen {
				p.addError(p.cur().Range, fmt.Sprintf("expected ',' or ')' in 'let' argument list, got %q", p.cur().Literal))
				p.syncToParamOrPortDelimiter()
				if p.cur().Type == lexer.TokenComma {
					p.advance()
				}
			}
		}
		if p.cur().Type == lexer.TokenRParen {
			p.advance()
		} else {
			p.addError(p.prev().Range, "expected ')' to close 'let' argument list")
		}
	}

	if p.cur().Type == lexer.TokenAssignOp {
		p.advance() // '='
		p.parseExprUntil(tempMod, lexer.TokenSemicolon)
	} else {
		p.addError(p.cur().Range, "expected '=' in 'let' declaration")
	}

	p.expectSemicolon("let declaration")
	fn.Range.End = p.prev().Range.End
	fn.Refs = tempMod.Refs

	if mod != nil {
		mod.MemberRefs = append(mod.MemberRefs, tempMod.MemberRefs...)
		mod.Items = append(mod.Items, fn)
	} else if file != nil {
		file.Functions = append(file.Functions, fn)
	}
}

// parseRandsequenceStmt parses a SystemVerilog `randsequence ([prod_name]) ... endsequence` statement.
func (p *Parser) parseRandsequenceStmt(block *ast.ProceduralBlock, mod *ast.ModuleDecl) {
	startTok := p.advance() // consume 'randsequence'

	if p.cur().Type == lexer.TokenLParen {
		p.advance() // '('
		if p.cur().Type == lexer.TokenIdent {
			p.advance() // optional start production name (local to randsequence)
		}
		if p.cur().Type == lexer.TokenRParen {
			p.advance()
		} else {
			p.addError(p.cur().Range, "expected ')' after randsequence header")
		}
	} else {
		p.addError(startTok.Range, "expected '(' after 'randsequence'")
	}

	for !p.isEOF() &&
		p.cur().Type != lexer.TokenEndsequence &&
		p.cur().Type != lexer.TokenEnd &&
		!isForkJoinEnd(p.cur().Type) &&
		p.cur().Type != lexer.TokenEndfunction &&
		p.cur().Type != lexer.TokenEndtask &&
		p.cur().Type != lexer.TokenEndmodule &&
		p.cur().Type != lexer.TokenModule {
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
			continue
		}

		// Optional production return type (`void`, `int`, `logic [7:0]`, user type, etc.)
		if p.cur().Type == lexer.TokenVoid || isPortNetType(p.cur().Type) || isSVDataType(p.cur().Type) {
			p.advance()
			if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
				p.advance()
			}
			for p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(mod)
			}
		} else if p.isUserTypeBeforeIdent() {
			_, _ = p.parseScopedTypeName(&block.Refs, mod)
			for p.cur().Type == lexer.TokenLBracket {
				_ = p.parseBitRange(mod)
			}
		}

		// Production name
		if p.cur().Type == lexer.TokenIdent {
			p.advance()
		} else {
			p.addError(p.cur().Range, "expected production name in 'randsequence'")
			p.syncToStatementEnd()
			continue
		}

		// Optional formal parameter list: `(int a, ...)`
		localParams := make(map[string]bool)
		if p.cur().Type == lexer.TokenLParen {
			paramMod := &ast.ModuleDecl{}
			p.parseModulePortList(paramMod)
			for _, port := range paramMod.Ports {
				if port.Name != "" {
					localParams[port.Name] = true
				}
			}
		}

		if p.cur().Type == lexer.TokenColon {
			p.advance() // ':'
		} else {
			p.addError(p.cur().Range, "expected ':' after randsequence production header")
			p.syncToStatementEnd()
			continue
		}

		// Parse `rs_rule { | rs_rule } ;`
		prodBlock := &ast.ProceduralBlock{Kind: block.Kind, Range: block.Range}
		prodMod := &ast.ModuleDecl{}
		for !p.isEOF() &&
			p.cur().Type != lexer.TokenSemicolon &&
			p.cur().Type != lexer.TokenEndsequence &&
			p.cur().Type != lexer.TokenEndmodule {
			// Optional `rand join [ (expr) ]`
			if p.cur().Type == lexer.TokenRand && p.peekN(1).Type == lexer.TokenJoin {
				p.advance() // 'rand'
				p.advance() // 'join'
				if p.cur().Type == lexer.TokenLParen {
					p.parseBalancedParens(&prodBlock.Refs, prodMod)
				}
			}

			// Production items up to `:=`, `|`, `;`, `endsequence`
			for !p.isEOF() &&
				p.cur().Type != lexer.TokenColonEq &&
				p.cur().Type != lexer.TokenPipe &&
				p.cur().Type != lexer.TokenSemicolon &&
				p.cur().Type != lexer.TokenEndsequence &&
				p.cur().Type != lexer.TokenEndmodule {
				if !p.parseRandsequenceItem(prodBlock, prodMod) {
					p.advance()
				}
			}

			// Optional weight specification `:= weight_expr [ { code_block } ]`
			if p.cur().Type == lexer.TokenColonEq {
				p.advance() // ':='
				p.collectTokensUntil(&prodBlock.Refs, prodMod, lexer.TokenLBrace, lexer.TokenPipe, lexer.TokenSemicolon, lexer.TokenEndsequence)
				if p.cur().Type == lexer.TokenLBrace {
					p.parseRandsequenceCodeBlock(prodBlock, prodMod)
				}
			}

			if p.cur().Type == lexer.TokenPipe {
				p.advance() // '|'
				continue
			}
			break
		}

		// Forward refs and writes from production body, filtering out production-local formal params.
		for _, r := range prodBlock.Refs {
			if !localParams[r.Name] {
				block.Refs = append(block.Refs, r)
			}
		}
		for _, r := range prodMod.Refs {
			if !localParams[r.Name] && mod != nil {
				mod.Refs = append(mod.Refs, r)
			}
		}
		block.Writes = append(block.Writes, prodBlock.Writes...)
		if mod != nil {
			mod.MemberRefs = append(mod.MemberRefs, prodMod.MemberRefs...)
		}

		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
		}
	}

	if p.cur().Type == lexer.TokenEndsequence {
		p.advance()
		p.consumeOptionalEndLabel()
	} else {
		p.addError(startTok.Range, "expected 'endsequence' to close 'randsequence'")
	}
}

func (p *Parser) parseRandsequenceCodeBlock(block *ast.ProceduralBlock, mod *ast.ModuleDecl) {
	startTok := p.advance() // '{'
	for !p.isEOF() &&
		p.cur().Type != lexer.TokenRBrace &&
		p.cur().Type != lexer.TokenEndsequence &&
		p.cur().Type != lexer.TokenEndmodule {
		p.parseStatement(block, mod)
	}
	if p.cur().Type == lexer.TokenRBrace {
		p.advance()
	} else {
		p.addError(startTok.Range, "expected '}' to close randsequence code block")
	}
}

func (p *Parser) parseRandsequenceItem(block *ast.ProceduralBlock, mod *ast.ModuleDecl) bool {
	switch p.cur().Type {
	case lexer.TokenLBrace:
		p.parseRandsequenceCodeBlock(block, mod)
		return true

	case lexer.TokenIf:
		p.advance() // 'if'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		}
		p.parseRandsequenceItem(block, mod)
		if p.cur().Type == lexer.TokenElse {
			p.advance()
			p.parseRandsequenceItem(block, mod)
		}
		return true

	case lexer.TokenRepeat:
		p.advance() // 'repeat'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		}
		p.parseRandsequenceItem(block, mod)
		return true

	case lexer.TokenCase:
		caseTok := p.advance() // 'case'
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		}
		for !p.isEOF() &&
			p.cur().Type != lexer.TokenEndcase &&
			p.cur().Type != lexer.TokenEndsequence &&
			p.cur().Type != lexer.TokenEndmodule {
			if p.cur().Type == lexer.TokenDefault {
				p.advance()
				if p.cur().Type == lexer.TokenColon {
					p.advance()
				}
				p.parseRandsequenceItem(block, mod)
				if p.cur().Type == lexer.TokenSemicolon {
					p.advance()
				}
				continue
			}
			toks := p.collectTokensUntil(&block.Refs, mod, lexer.TokenColon, lexer.TokenEndcase, lexer.TokenEndsequence)
			if p.cur().Type == lexer.TokenColon {
				p.advance() // ':'
				p.parseRandsequenceItem(block, mod)
				if p.cur().Type == lexer.TokenSemicolon {
					p.advance()
				}
			} else if len(toks) == 0 {
				break
			}
		}
		if p.cur().Type == lexer.TokenEndcase {
			p.advance()
		} else {
			p.addError(caseTok.Range, "expected 'endcase' to close randsequence 'case'")
		}
		return true

	case lexer.TokenIdent:
		// Production call: `prod_name [ ( args ) ]` (do not record prod_name as a signal ref)
		p.advance()
		if p.cur().Type == lexer.TokenLParen {
			p.parseBalancedParens(&block.Refs, mod)
		}
		return true

	default:
		return false
	}
}

func (p *Parser) parseTypedefDecl(mod *ast.ModuleDecl, file *ast.SourceFile) {
	startTok := p.advance() // consume 'typedef' or 'nettype'
	isNettype := startTok.Type == lexer.TokenNettype
	td := &ast.TypedefDecl{
		IsNettype: isNettype,
		Range:     ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: startTok.Range.End},
	}

	var baseParts []string
	switch p.cur().Type {
	case lexer.TokenEnum:
		baseParts = append(baseParts, p.advance().Literal)
		// Forward typedef: `typedef enum state_t;`
		if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenSemicolon {
			nameTok := p.advance()
			td.Name = nameTok.Literal
			td.NameRange = nameTok.Range
			td.BaseType = strings.Join(baseParts, " ")
			p.advance() // ';'
			td.Range.End = p.prev().Range.End
			p.registerTypedef(td, mod, file)
			return
		}
		if isPortNetType(p.cur().Type) || isParamTypeKeyword(p.cur().Type) {
			baseParts = append(baseParts, p.advance().Literal)
		} else if p.cur().Type == lexer.TokenIdent {
			tTok := p.advance()
			baseParts = append(baseParts, tTok.Literal)
			p.recordRef(&td.Refs, mod, tTok)
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			baseParts = append(baseParts, p.advance().Literal)
		}
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			baseParts = append(baseParts, r.Raw)
		}
		if p.cur().Type == lexer.TokenLBrace {
			td.EnumMembers = p.parseEnumBody(mod)
		} else {
			p.addError(p.cur().Range, "expected '{' in enum typedef")
		}

	case lexer.TokenClass:
		// Forward class typedef: `typedef class Packet;`
		baseParts = append(baseParts, p.advance().Literal)
		if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenSemicolon {
			nameTok := p.advance()
			td.Name = nameTok.Literal
			td.NameRange = nameTok.Range
			td.BaseType = "class"
			p.advance() // ';'
			td.Range.End = p.prev().Range.End
			p.registerTypedef(td, mod, file)
			return
		}

	case lexer.TokenStruct, lexer.TokenUnion:
		baseParts = append(baseParts, p.advance().Literal)
		// Forward typedef: `typedef struct pkt_t;`
		if p.cur().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenSemicolon {
			nameTok := p.advance()
			td.Name = nameTok.Literal
			td.NameRange = nameTok.Range
			td.BaseType = strings.Join(baseParts, " ")
			p.advance() // ';'
			td.Range.End = p.prev().Range.End
			p.registerTypedef(td, mod, file)
			return
		}
		if p.cur().Type == lexer.TokenPacked {
			baseParts = append(baseParts, p.advance().Literal)
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			baseParts = append(baseParts, p.advance().Literal)
		}
		if p.cur().Type == lexer.TokenLBrace {
			td.StructFields = p.parseStructBody()
		} else {
			p.addError(p.cur().Range, fmt.Sprintf("expected '{' in %s typedef", baseParts[0]))
		}

	default:
		// Simple type alias: `typedef logic [7:0] byte_t;`, `typedef state_t my_state_t;`,
		// or parameterized class specialization `typedef base_box #(packet, 16) pkt_box_t;`.
		if isPortNetType(p.cur().Type) || isParamTypeKeyword(p.cur().Type) {
			baseParts = append(baseParts, p.advance().Literal)
		} else if p.cur().Type == lexer.TokenIdent {
			tName, _ := p.parseScopedTypeName(&td.Refs, mod)
			baseParts = append(baseParts, tName)
			if p.cur().Type == lexer.TokenHash && p.peekN(1).Type == lexer.TokenLParen {
				td.TypeArgs = p.parseTypeParamArgs(&td.Refs, mod)
			}
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			baseParts = append(baseParts, p.advance().Literal)
		}
		for p.cur().Type == lexer.TokenLBracket {
			r := p.parseBitRange(mod)
			baseParts = append(baseParts, r.Raw)
		}
	}

	if p.cur().Type == lexer.TokenIdent {
		nameTok := p.advance()
		td.Name = nameTok.Literal
		td.NameRange = nameTok.Range
	} else {
		p.addError(p.cur().Range, "expected type name in typedef declaration")
		p.syncToStatementEnd()
		return
	}

	// Optional unpacked array dimensions after type name: `typedef logic [7:0] mem_t [0:3];`, `typedef int int_q_t[$];`
	for p.cur().Type == lexer.TokenLBracket {
		if r := p.parseBitRange(mod); r != nil {
			td.UnpackedDims = append(td.UnpackedDims, r)
		}
	}

	// Optional resolution function for nettype: `nettype real wreal_t with [pkg::]resolve_fn;`
	if isNettype && p.cur().Type == lexer.TokenWith {
		p.advance() // 'with'
		if p.cur().Type == lexer.TokenIdent {
			_, _ = p.parseScopedTypeName(&td.Refs, mod)
		} else {
			p.addError(p.cur().Range, "expected resolution function name after 'with' in nettype declaration")
		}
	}

	if isNettype {
		p.expectSemicolon("nettype declaration")
	} else {
		p.expectSemicolon("typedef declaration")
	}
	td.BaseType = strings.Join(baseParts, " ")
	td.Range.End = p.prev().Range.End
	p.registerTypedef(td, mod, file)
}

func (p *Parser) registerTypedef(td *ast.TypedefDecl, mod *ast.ModuleDecl, file *ast.SourceFile) {
	if td.Name != "" && p.typeNames != nil {
		p.typeNames[td.Name] = true
	}
	if mod != nil {
		mod.Items = append(mod.Items, td)
	} else if file != nil {
		file.Typedefs = append(file.Typedefs, td)
		file.Refs = append(file.Refs, td.Refs...)
	}
}

func (p *Parser) parseEnumOrStructVarDecl(mod *ast.ModuleDecl) {
	startTok := p.advance() // enum / struct / union
	kind := startTok.Literal
	var enumMembers []ast.EnumMember
	var structFields []ast.StructField

	if startTok.Type == lexer.TokenEnum {
		if isPortNetType(p.cur().Type) || isParamTypeKeyword(p.cur().Type) {
			p.advance()
		} else if p.cur().Type == lexer.TokenIdent {
			tTok := p.advance()
			p.recordRef(nil, mod, tTok)
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			p.advance()
		}
		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
		}
		if p.cur().Type == lexer.TokenLBrace {
			enumMembers = p.parseEnumBody(mod)
		} else {
			p.addError(p.cur().Range, "expected '{' in enum declaration")
		}
	} else {
		if p.cur().Type == lexer.TokenPacked {
			p.advance()
		}
		if p.cur().Type == lexer.TokenSigned || p.cur().Type == lexer.TokenUnsigned {
			p.advance()
		}
		if p.cur().Type == lexer.TokenLBrace {
			structFields = p.parseStructBody()
		} else {
			p.addError(p.cur().Range, fmt.Sprintf("expected '{' in %s declaration", kind))
		}
	}

	first := true
	for !p.isEOF() {
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, fmt.Sprintf("expected variable identifier after %s declaration", kind))
			p.syncToStatementEnd()
			return
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End

		for p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance()
			_, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenSemicolon)
		}

		if mod != nil {
			decl := &ast.NetDecl{
				Kind:         kind,
				Name:         nameTok.Literal,
				NameRange:    nameTok.Range,
				StructFields: structFields,
				Range:        ast.Range{File: startTok.Range.File, Start: startTok.Range.Start, End: endPos},
			}
			if first {
				decl.EnumMembers = enumMembers
				first = false
			}
			mod.Items = append(mod.Items, decl)
		}

		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		break
	}

	p.expectSemicolon(kind + " declaration")
}

func (p *Parser) parseEnumBody(mod *ast.ModuleDecl) []ast.EnumMember {
	openTok := p.advance() // '{'
	var members []ast.EnumMember

	for !p.isEOF() && p.cur().Type != lexer.TokenRBrace && p.cur().Type != lexer.TokenEndmodule {
		if p.cur().Type == lexer.TokenComma {
			p.advance()
			continue
		}
		if p.cur().Type != lexer.TokenIdent {
			p.addError(p.cur().Range, "expected enumerator name in enum")
			p.advance()
			continue
		}
		nameTok := p.advance()
		endPos := nameTok.Range.End
		// Optional enum range syntax: e.g. STATE[4] or STATE[0:3]
		if p.cur().Type == lexer.TokenLBracket {
			_ = p.parseBitRange(mod)
			endPos = p.prev().Range.End
		}
		valText := ""
		if p.cur().Type == lexer.TokenAssignOp {
			p.advance() // '='
			valText, endPos = p.parseExprUntil(mod, lexer.TokenComma, lexer.TokenRBrace)
		}
		members = append(members, ast.EnumMember{
			Name:      nameTok.Literal,
			NameRange: nameTok.Range,
			ValueText: valText,
			Range:     ast.Range{File: nameTok.Range.File, Start: nameTok.Range.Start, End: endPos},
		})
		if p.cur().Type == lexer.TokenComma {
			p.advance()
		}
	}

	if p.cur().Type == lexer.TokenRBrace {
		p.advance()
	} else {
		p.addError(openTok.Range, "expected '}' to close enum body")
	}
	return members
}

func (p *Parser) parseStructBody() []ast.StructField {
	openTok := p.advance() // '{'
	var fields []ast.StructField

	for !p.isEOF() && p.cur().Type != lexer.TokenRBrace && p.cur().Type != lexer.TokenEndmodule {
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
			continue
		}
		// Collect tokens of a single struct member declaration up to ';' (respecting nested braces/brackets).
		memberStart := p.cur().Range
		var toks []lexer.Token
		braceDepth := 0
		for !p.isEOF() {
			if braceDepth == 0 && (p.cur().Type == lexer.TokenSemicolon || p.cur().Type == lexer.TokenRBrace || p.cur().Type == lexer.TokenEndmodule) {
				break
			}
			tok := p.advance()
			if tok.Type == lexer.TokenLBrace {
				braceDepth++
			} else if tok.Type == lexer.TokenRBrace && braceDepth > 0 {
				braceDepth--
			} else if tok.Type == lexer.TokenIllegal {
				p.addError(tok.Range, tok.Literal)
			}
			toks = append(toks, tok)
		}
		if p.cur().Type == lexer.TokenSemicolon {
			p.advance()
		}
		if len(toks) >= 2 {
			fields = append(fields, extractStructFields(toks, memberStart)...)
		}
	}

	if p.cur().Type == lexer.TokenRBrace {
		p.advance()
	} else {
		p.addError(openTok.Range, "expected '}' to close struct/union body")
	}
	return fields
}

func extractStructFields(toks []lexer.Token, startRange ast.Range) []ast.StructField {
	// Skip top-level brace/bracket contents when finding field names at the end of a member declaration.
	// Find where the type specification ends and the comma-separated field name list begins.
	// Example: `logic [7:0] addr, data [0:3]` -> type="logic [7:0]", names=["addr", "data"].
	var out []ast.StructField
	// Walk backwards over top-level tokens to locate field names (separated by commas, optionally followed by `[...]` or `= expr`).
	// For simplicity and reliability, split top-level tokens into segments around `,` after the first field name.
	// First, find the index of the first field name: scan forward past type keywords, `pkg::type`, nested `{...}`, `signed`/`unsigned`, and packed `[...]`.
	i := 0
	n := len(toks)
	// Skip leading type tokens
	if i < n && (toks[i].Type == lexer.TokenStruct || toks[i].Type == lexer.TokenUnion || toks[i].Type == lexer.TokenEnum) {
		i++
		if i < n && toks[i].Type == lexer.TokenPacked {
			i++
		}
		if i < n && (toks[i].Type == lexer.TokenSigned || toks[i].Type == lexer.TokenUnsigned) {
			i++
		}
		if i < n && toks[i].Type == lexer.TokenLBrace {
			depth := 1
			i++
			for i < n && depth > 0 {
				if toks[i].Type == lexer.TokenLBrace {
					depth++
				} else if toks[i].Type == lexer.TokenRBrace {
					depth--
				}
				i++
			}
		}
	} else if i < n {
		// Built-in type keyword or user-defined type (`type_t` or `pkg::type_t`)
		i++
		if i+1 < n && toks[i].Type == lexer.TokenColonColon && toks[i+1].Type == lexer.TokenIdent {
			i += 2
		}
		if i < n && (toks[i].Type == lexer.TokenSigned || toks[i].Type == lexer.TokenUnsigned) {
			i++
		}
	}
	// Skip packed dimensions `[...]` belonging to the type
	for i < n && toks[i].Type == lexer.TokenLBracket {
		depth := 1
		i++
		for i < n && depth > 0 {
			if toks[i].Type == lexer.TokenLBracket {
				depth++
			} else if toks[i].Type == lexer.TokenRBracket {
				depth--
			}
			i++
		}
	}
	if i >= n {
		return nil
	}

	var typeParts []string
	for j := 0; j < i; j++ {
		if toks[j].Type == lexer.TokenLBrace {
			typeParts = append(typeParts, "{...}")
			depth := 1
			j++
			for j < i && depth > 0 {
				if toks[j].Type == lexer.TokenLBrace {
					depth++
				} else if toks[j].Type == lexer.TokenRBrace {
					depth--
				}
				j++
			}
			j--
			continue
		}
		typeParts = append(typeParts, toks[j].Literal)
	}
	typeDetail := strings.Join(typeParts, " ")

	for i < n {
		if toks[i].Type == lexer.TokenIdent {
			nameTok := toks[i]
			out = append(out, ast.StructField{
				TypeDetail: typeDetail,
				Name:       nameTok.Literal,
				NameRange:  nameTok.Range,
				Range:      ast.Range{File: startRange.File, Start: startRange.Start, End: nameTok.Range.End},
			})
			i++
			// Skip optional unpacked dimensions `[...]` and optional default `= expr` up to `,`
			depth := 0
			for i < n {
				if depth == 0 && toks[i].Type == lexer.TokenComma {
					i++
					break
				}
				switch toks[i].Type {
				case lexer.TokenLBracket, lexer.TokenLParen, lexer.TokenLBrace:
					depth++
				case lexer.TokenRBracket, lexer.TokenRParen, lexer.TokenRBrace:
					if depth > 0 {
						depth--
					}
				}
				i++
			}
			continue
		}
		i++
	}
	return out
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

// isTaskCallStart checks whether the current TokenIdent begins a task call statement
// such as `my_task(a, b);`, `my_task;`, or a hierarchical call `u_sub.my_task(a, b);`.
func (p *Parser) isTaskCallStart() bool {
	offset := 1
	for {
		t := p.peekN(offset)
		switch t.Type {
		case lexer.TokenLParen, lexer.TokenSemicolon:
			return true
		case lexer.TokenDot:
			nextType := p.peekN(offset + 1).Type
			if nextType != lexer.TokenIdent && nextType != lexer.TokenNew {
				return false
			}
			offset += 2
		case lexer.TokenLBracket:
			// Skip balanced [...] index on hierarchical path segment (e.g. g_blk[0].u_sub.my_task)
			depth := 1
			offset++
			for depth > 0 {
				bt := p.peekN(offset)
				if bt.Type == lexer.TokenEOF || bt.Type == lexer.TokenSemicolon {
					return false
				}
				if bt.Type == lexer.TokenLBracket {
					depth++
				} else if bt.Type == lexer.TokenRBracket {
					depth--
				}
				offset++
			}
		default:
			return false
		}
	}
}

func (p *Parser) parseTaskCallStmt(block *ast.ProceduralBlock, mod *ast.ModuleDecl) {
	// Record only the root identifier of a hierarchical task/method path (skip `this` and `super`).
	rootTok := p.advance()
	if rootTok.Type == lexer.TokenIdent {
		p.recordRef(&block.Refs, mod, rootTok)
	}
	recordedMember := false

	for !p.isEOF() {
		switch p.cur().Type {
		case lexer.TokenDot:
			p.advance() // '.'
			if p.cur().Type == lexer.TokenIdent || p.cur().Type == lexer.TokenNew {
				memberTok := p.advance()
				if !recordedMember {
					p.recordMemberRef(mod, rootTok.Literal, memberTok)
					recordedMember = true
				}
			}
		case lexer.TokenLBracket:
			_ = p.parseBitRange(mod)
		case lexer.TokenLParen:
			p.parseBalancedParens(&block.Refs, mod)
			p.expectSemicolon("task call")
			return
		case lexer.TokenSemicolon:
			p.advance()
			return
		default:
			p.expectSemicolon("task call")
			return
		}
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
