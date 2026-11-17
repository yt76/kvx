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

// ModuleScope holds the symbol table for a single module declaration.
type ModuleScope struct {
	Module  *ast.ModuleDecl
	Symbol  *Symbol
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// Analysis holds the symbol tables and semantic diagnostics for a SourceFile.
type Analysis struct {
	File        *ast.SourceFile
	Modules     map[string]*ModuleScope
	ModuleOrder []*ModuleScope
	Diagnostics []SemanticDiagnostic
}

// Analyze builds symbol tables and performs semantic checks on file.
func Analyze(file *ast.SourceFile) *Analysis {
	res := &Analysis{
		File:    file,
		Modules: make(map[string]*ModuleScope),
	}
	if file == nil {
		return res
	}

	for _, mod := range file.Modules {
		scope := analyzeModule(mod, &res.Diagnostics)
		res.ModuleOrder = append(res.ModuleOrder, scope)
		if mod.Name != "" {
			res.Modules[mod.Name] = scope
		}
	}

	return res
}

func analyzeModule(mod *ast.ModuleDecl, diags *[]SemanticDiagnostic) *ModuleScope {
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

	addSymbol := func(sym *Symbol) {
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
			*diags = append(*diags, SemanticDiagnostic{
				Range:   sym.SelectionRange,
				Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
				IsError: false,
			})
			return
		}
		scope.Symbols[sym.Name] = sym
		scope.Order = append(scope.Order, sym)
		modSym.Children = append(modSym.Children, sym)
	}

	for _, item := range mod.Items {
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
			})

		case *ast.PortDecl:
			detail := formatPortDeclDetail(d)
			addSymbol(&Symbol{
				Name:           d.Name,
				Kind:           KindPort,
				TypeDetail:     detail,
				Signature:      detail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})

		case *ast.NetDecl:
			detail := formatNetDeclDetail(d)
			addSymbol(&Symbol{
				Name:           d.Name,
				Kind:           KindNet,
				TypeDetail:     detail,
				Signature:      detail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})

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
			})
		}
	}

	// Check identifier references against the module's symbol table.
	for _, ref := range mod.Refs {
		if strings.HasPrefix(ref.Name, "`") {
			// Macro usage (e.g. `DATA_WIDTH) may come from an external `define.
			continue
		}
		if _, ok := scope.Symbols[ref.Name]; !ok {
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
		for _, item := range mod.Items {
			if inst, ok := item.(*ast.ModuleInst); ok {
				if inst.ModuleNameRange.Contains(pos) {
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
			}
		}

		// 3. Check identifier references in expressions, assignments, and procedural blocks.
		for _, ref := range mod.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := scope.Symbols[ref.Name]; ok {
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
		out = append(out, scope.Order...)
	}
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
