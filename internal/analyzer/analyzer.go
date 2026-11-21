package analyzer

import (
	"fmt"
	"strings"

	"kvx/internal/ast"
)

// SymbolKind classifies a Verilog symbol.
type SymbolKind string

const (
	KindModule   SymbolKind = "module"
	KindPort     SymbolKind = "port"
	KindNet      SymbolKind = "net"
	KindParam    SymbolKind = "parameter"
	KindInstance SymbolKind = "instance"
	KindFunction SymbolKind = "function"
)

// Symbol represents a named declaration in a Verilog source file.
type Symbol struct {
	Name           string
	Kind           SymbolKind
	TypeDetail     string
	Signature      string
	Range          ast.Range
	SelectionRange ast.Range
	TargetModule   string
	Children       []*Symbol
}

// SemanticDiagnostic represents a semantic warning or error (such as an undeclared signal).
type SemanticDiagnostic struct {
	Range   ast.Range
	Message string
	IsError bool
}

// FunctionScope holds the local symbol table for a function declaration.
type FunctionScope struct {
	Decl    *ast.FunctionDecl
	Symbol  *Symbol
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// ModuleScope holds the symbol table for a single module declaration.
type ModuleScope struct {
	Module    *ast.ModuleDecl
	Symbol    *Symbol
	Symbols   map[string]*Symbol
	Order     []*Symbol
	Functions []*FunctionScope
}

// Analysis holds the symbol tables and semantic diagnostics for a SourceFile.
type Analysis struct {
	File          *ast.SourceFile
	Modules       map[string]*ModuleScope
	ModuleOrder   []*ModuleScope
	GlobalSymbols map[string]*Symbol
	GlobalOrder   []*Symbol
	Diagnostics   []SemanticDiagnostic
}

// Analyze builds symbol tables and performs semantic checks on file.
func Analyze(file *ast.SourceFile) *Analysis {
	res := &Analysis{
		File:          file,
		Modules:       make(map[string]*ModuleScope),
		GlobalSymbols: make(map[string]*Symbol),
	}
	if file == nil {
		return res
	}

	for _, m := range file.Macros {
		if m.Name == "" {
			continue
		}
		sig := "`define " + m.Name
		if m.ValueText != "" {
			sig += " " + m.ValueText
		}
		sym := &Symbol{
			Name:           "`" + m.Name,
			Kind:           KindParam,
			TypeDetail:     "`define",
			Signature:      sig,
			Range:          m.Range,
			SelectionRange: m.NameRange,
		}
		res.GlobalSymbols["`"+m.Name] = sym
		res.GlobalSymbols[m.Name] = sym
		res.GlobalOrder = append(res.GlobalOrder, sym)
	}

	for _, d := range file.Params {
		if d.Name == "" {
			continue
		}
		detail := formatParamDetail(d)
		sig := detail + " " + d.Name
		if d.ValueText != "" {
			sig += " = " + d.ValueText
		}
		sym := &Symbol{
			Name:           d.Name,
			Kind:           KindParam,
			TypeDetail:     detail,
			Signature:      sig,
			Range:          d.Range,
			SelectionRange: d.NameRange,
		}
		res.GlobalSymbols[d.Name] = sym
		res.GlobalOrder = append(res.GlobalOrder, sym)
	}

	for _, mod := range file.Modules {
		scope := analyzeModule(mod, res.GlobalSymbols, &res.Diagnostics)
		res.ModuleOrder = append(res.ModuleOrder, scope)
		if mod.Name != "" {
			res.Modules[mod.Name] = scope
		}
	}

	return res
}

func analyzeModule(mod *ast.ModuleDecl, globals map[string]*Symbol, diags *[]SemanticDiagnostic) *ModuleScope {
	modSym := &Symbol{
		Name:           mod.Name,
		Kind:           KindModule,
		TypeDetail:     fmt.Sprintf("module (%d ports)", len(mod.Ports)),
		Signature:      formatModuleSignature(mod),
		Range:          mod.Range,
		SelectionRange: mod.NameRange,
	}

	scope := &ModuleScope{
		Module:  mod,
		Symbol:  modSym,
		Symbols: make(map[string]*Symbol),
	}

	addSymbol := func(sym *Symbol, inGenerate bool) {
		if sym.Name == "" {
			return
		}
		if existing, exists := scope.Symbols[sym.Name]; exists {
			// In non-ANSI Verilog, a port declaration without a net type (e.g., "output [7:0] y;")
			// may be followed by a net/reg declaration (e.g., "reg [7:0] y;"). Merge them cleanly.
			if existing.Kind == KindPort && sym.Kind == KindNet &&
				!strings.Contains(existing.TypeDetail, "wire") &&
				!strings.Contains(existing.TypeDetail, "reg") {
				existing.TypeDetail = formatPortDetail(existing.TypeDetail, sym.TypeDetail)
				existing.Signature = existing.TypeDetail + " " + existing.Name
				return
			}
			// Mutually exclusive generate branches (if/else/case) may declare the same local name.
			if !inGenerate && sym.SelectionRange.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		scope.Symbols[sym.Name] = sym
		scope.Order = append(scope.Order, sym)
		if sym.Range.File == "" {
			modSym.Children = append(modSym.Children, sym)
		}
	}

	var collectItems func(items []ast.ModuleItem, inGenerate bool)
	collectItems = func(items []ast.ModuleItem, inGenerate bool) {
		for _, item := range items {
			switch d := item.(type) {
			case *ast.ParamDecl:
				detail := formatParamDetail(d)
				sig := detail + " " + d.Name
				if d.ValueText != "" {
					sig += " = " + d.ValueText
				}
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindParam,
					TypeDetail:     detail,
					Signature:      sig,
					Range:          d.Range,
					SelectionRange: d.NameRange,
				}, inGenerate)

			case *ast.PortDecl:
				detail := formatPortDeclDetail(d)
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindPort,
					TypeDetail:     detail,
					Signature:      detail + " " + d.Name,
					Range:          d.Range,
					SelectionRange: d.NameRange,
				}, inGenerate)

			case *ast.NetDecl:
				detail := formatNetDeclDetail(d)
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindNet,
					TypeDetail:     detail,
					Signature:      detail + " " + d.Name,
					Range:          d.Range,
					SelectionRange: d.NameRange,
				}, inGenerate)

			case *ast.ModuleInst:
				detail := fmt.Sprintf("instance of %s", d.ModuleName)
				addSymbol(&Symbol{
					Name:           d.InstanceName,
					Kind:           KindInstance,
					TypeDetail:     detail,
					Signature:      fmt.Sprintf("%s %s", d.ModuleName, d.InstanceName),
					Range:          d.Range,
					SelectionRange: d.InstanceNameRange,
					TargetModule:   d.ModuleName,
				}, inGenerate)

			case *ast.FunctionDecl:
				fnScope := buildFunctionScope(d, diags)
				addSymbol(fnScope.Symbol, inGenerate)
				scope.Functions = append(scope.Functions, fnScope)

			case *ast.GenerateBlock:
				collectItems(d.Items, true)
			}
		}
	}

	collectItems(mod.Items, false)

	// Check identifier references inside functions against local, module, and global scopes.
	for _, fnScope := range scope.Functions {
		for _, ref := range fnScope.Decl.Refs {
			if strings.HasPrefix(ref.Name, "`") {
				continue
			}
			if _, ok := fnScope.Symbols[ref.Name]; ok {
				continue
			}
			if _, ok := scope.Symbols[ref.Name]; ok {
				continue
			}
			if _, ok := globals[ref.Name]; ok {
				continue
			}
			if ref.Range.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   ref.Range,
					Message: fmt.Sprintf("undeclared identifier %q", ref.Name),
					IsError: false,
				})
			}
		}
	}

	// Check identifier references against the module's symbol table and global symbols.
	for _, ref := range mod.Refs {
		if strings.HasPrefix(ref.Name, "`") {
			// Macro usage (e.g. `DATA_WIDTH) may come from an external `define.
			continue
		}
		if _, ok := scope.Symbols[ref.Name]; ok {
			continue
		}
		if _, ok := globals[ref.Name]; ok {
			continue
		}
		if ref.Range.File == "" {
			*diags = append(*diags, SemanticDiagnostic{
				Range:   ref.Range,
				Message: fmt.Sprintf("undeclared identifier %q", ref.Name),
				IsError: false,
			})
		}
	}

	return scope
}

// ModuleAt returns the ModuleScope enclosing pos, or nil if pos is outside all modules.
func (a *Analysis) ModuleAt(pos ast.Position) *ModuleScope {
	for _, scope := range a.ModuleOrder {
		if scope.Module.Range.Contains(pos) {
			return scope
		}
	}
	return nil
}

// SymbolAt resolves the Symbol under the cursor at pos (either a declaration or a reference).
func (a *Analysis) SymbolAt(pos ast.Position) *Symbol {
	if a.File != nil {
		// Check `include "..." directives.
		for _, inc := range a.File.Includes {
			if inc.Range.Contains(pos) && inc.ResolvedPath != "" {
				return &Symbol{
					Name:       inc.Path,
					Kind:       KindModule,
					TypeDetail: "`include",
					Signature:  fmt.Sprintf("`include %q", inc.Path),
					Range:      inc.Range,
					SelectionRange: ast.Range{
						File:  inc.ResolvedPath,
						Start: ast.Position{Line: 0, Column: 0},
						End:   ast.Position{Line: 0, Column: 0},
					},
				}
			}
		}

		// Check top-level macro and parameter declarations.
		for _, sym := range a.GlobalOrder {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}

		// Check macro references (`MACRO_NAME).
		for _, ref := range a.File.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := a.GlobalSymbols[ref.Name]; ok {
					return sym
				}
			}
		}
	}

	for _, scope := range a.ModuleOrder {
		mod := scope.Module
		if mod.NameRange.Contains(pos) {
			return scope.Symbol
		}
		if !mod.Range.Contains(pos) {
			continue
		}

		// 1. Check declarations in the module.
		for _, sym := range scope.Order {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}

		// 2. Check module instantiation type names (e.g. clicking on `sub_alu` in `sub_alu u_alu (...)`).
		if inst := findModuleInstAt(mod.Items, pos); inst != nil {
			if targetScope, exists := a.Modules[inst.ModuleName]; exists {
				return targetScope.Symbol
			}
			return &Symbol{
				Name:           inst.ModuleName,
				Kind:           KindModule,
				TypeDetail:     "module",
				Signature:      "module " + inst.ModuleName,
				Range:          inst.ModuleNameRange,
				SelectionRange: inst.ModuleNameRange,
			}
		}

		// 3. Check declarations and references inside functions.
		for _, fnScope := range scope.Functions {
			if !fnScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range fnScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range fnScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := fnScope.Symbols[ref.Name]; ok {
						return sym
					}
					if sym, ok := scope.Symbols[ref.Name]; ok {
						return sym
					}
					if sym, ok := a.GlobalSymbols[ref.Name]; ok {
						return sym
					}
				}
			}
		}

		// 4. Check identifier references in expressions, assignments, and procedural blocks.
		for _, ref := range mod.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := scope.Symbols[ref.Name]; ok {
					return sym
				}
				if sym, ok := a.GlobalSymbols[ref.Name]; ok {
					return sym
				}
			}
		}
	}
	return nil
}

// CompletionsAt returns symbols available in scope at pos.
func (a *Analysis) CompletionsAt(pos ast.Position) []*Symbol {
	var out []*Symbol
	if scope := a.ModuleAt(pos); scope != nil {
		for _, fnScope := range scope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				out = append(out, fnScope.Order...)
				break
			}
		}
		out = append(out, scope.Order...)
	}
	out = append(out, a.GlobalOrder...)
	for _, modScope := range a.ModuleOrder {
		if modScope.Symbol.Name != "" {
			out = append(out, modScope.Symbol)
		}
	}
	return out
}

func formatModuleSignature(mod *ast.ModuleDecl) string {
	var b strings.Builder
	b.WriteString("module ")
	b.WriteString(mod.Name)
	if len(mod.Ports) > 0 {
		b.WriteString(" (")
		for i, p := range mod.Ports {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatPortDeclDetail(p))
			b.WriteString(" ")
			b.WriteString(p.Name)
		}
		b.WriteString(")")
	}
	return b.String()
}

func formatPortDeclDetail(d *ast.PortDecl) string {
	parts := []string{d.Direction}
	if d.NetType != "" {
		parts = append(parts, d.NetType)
	}
	if d.Signed {
		parts = append(parts, "signed")
	}
	if d.Width != nil {
		parts = append(parts, d.Width.Raw)
	}
	return strings.Join(parts, " ")
}

func formatPortDetail(existingPortDetail, netDetail string) string {
	// If existingPortDetail is e.g. "output [3:0]" and netDetail is "reg [3:0]",
	// combine into "output reg [3:0]" if not already specifying wire/reg.
	if strings.Contains(existingPortDetail, "reg") || strings.Contains(existingPortDetail, "wire") {
		return existingPortDetail
	}
	if strings.HasPrefix(netDetail, "reg") {
		parts := strings.SplitN(existingPortDetail, " ", 2)
		if len(parts) == 2 {
			return parts[0] + " reg " + parts[1]
		}
		return existingPortDetail + " reg"
	}
	return existingPortDetail
}

func formatNetDeclDetail(d *ast.NetDecl) string {
	parts := []string{d.Kind}
	if d.Signed {
		parts = append(parts, "signed")
	}
	if d.Width != nil {
		parts = append(parts, d.Width.Raw)
	}
	return strings.Join(parts, " ")
}

func formatParamDetail(d *ast.ParamDecl) string {
	parts := []string{d.Kind}
	if d.Signed {
		parts = append(parts, "signed")
	}
	if d.Width != nil {
		parts = append(parts, d.Width.Raw)
	}
	return strings.Join(parts, " ")
}

func buildFunctionScope(fn *ast.FunctionDecl, diags *[]SemanticDiagnostic) *FunctionScope {
	sig := formatFunctionSignature(fn)
	fnSym := &Symbol{
		Name:           fn.Name,
		Kind:           KindFunction,
		TypeDetail:     formatFunctionReturnType(fn),
		Signature:      sig,
		Range:          fn.Range,
		SelectionRange: fn.NameRange,
	}

	fs := &FunctionScope{
		Decl:    fn,
		Symbol:  fnSym,
		Symbols: make(map[string]*Symbol),
	}
	if fn.Name != "" {
		// Inside a function, assigning to the function name sets the return value.
		fs.Symbols[fn.Name] = fnSym
	}

	addLocal := func(sym *Symbol) {
		if sym.Name == "" {
			return
		}
		if _, exists := fs.Symbols[sym.Name]; exists {
			if sym.SelectionRange.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		fs.Symbols[sym.Name] = sym
		fs.Order = append(fs.Order, sym)
		if sym.Range.File == "" {
			fnSym.Children = append(fnSym.Children, sym)
		}
	}

	for _, p := range fn.Params {
		detail := formatPortDeclDetail(p)
		addLocal(&Symbol{
			Name:           p.Name,
			Kind:           KindPort,
			TypeDetail:     detail,
			Signature:      detail + " " + p.Name,
			Range:          p.Range,
			SelectionRange: p.NameRange,
		})
	}

	for _, item := range fn.Locals {
		switch d := item.(type) {
		case *ast.ParamDecl:
			detail := formatParamDetail(d)
			s := detail + " " + d.Name
			if d.ValueText != "" {
				s += " = " + d.ValueText
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindParam,
				TypeDetail:     detail,
				Signature:      s,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
		case *ast.NetDecl:
			detail := formatNetDeclDetail(d)
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindNet,
				TypeDetail:     detail,
				Signature:      detail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
		}
	}

	return fs
}

func formatFunctionReturnType(fn *ast.FunctionDecl) string {
	parts := []string{"function"}
	if fn.Automatic {
		parts = append(parts, "automatic")
	}
	if fn.Signed {
		parts = append(parts, "signed")
	}
	if fn.ReturnType != "" {
		parts = append(parts, fn.ReturnType)
	}
	if fn.Width != nil {
		parts = append(parts, fn.Width.Raw)
	}
	return strings.Join(parts, " ")
}

func formatFunctionSignature(fn *ast.FunctionDecl) string {
	var b strings.Builder
	b.WriteString(formatFunctionReturnType(fn))
	b.WriteString(" ")
	b.WriteString(fn.Name)
	if len(fn.Params) > 0 {
		b.WriteString(" (")
		for i, p := range fn.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatPortDeclDetail(p))
			b.WriteString(" ")
			b.WriteString(p.Name)
		}
		b.WriteString(")")
	}
	return b.String()
}

func findModuleInstAt(items []ast.ModuleItem, pos ast.Position) *ast.ModuleInst {
	for _, item := range items {
		switch d := item.(type) {
		case *ast.ModuleInst:
			if d.ModuleNameRange.Contains(pos) {
				return d
			}
		case *ast.GenerateBlock:
			if found := findModuleInstAt(d.Items, pos); found != nil {
				return found
			}
		}
	}
	return nil
}

// FindModuleInstantiations returns the ModuleNameRange of every instantiation of moduleName in this file.
func (a *Analysis) FindModuleInstantiations(moduleName string) []ast.Range {
	if a == nil || moduleName == "" {
		return nil
	}
	var ranges []ast.Range
	for _, scope := range a.ModuleOrder {
		if scope.Module == nil {
			continue
		}
		collectModuleInstantiations(scope.Module.Items, moduleName, &ranges)
	}
	return ranges
}

func collectModuleInstantiations(items []ast.ModuleItem, moduleName string, out *[]ast.Range) {
	for _, item := range items {
		switch d := item.(type) {
		case *ast.ModuleInst:
			if d.ModuleName == moduleName {
				*out = append(*out, d.ModuleNameRange)
			}
		case *ast.GenerateBlock:
			collectModuleInstantiations(d.Items, moduleName, out)
		}
	}
}
