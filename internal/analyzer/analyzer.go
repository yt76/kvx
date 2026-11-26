package analyzer

import (
	"fmt"
	"strings"

	"kvx/internal/ast"
)

// SymbolKind classifies a Verilog symbol.
type SymbolKind string

const (
	KindModule    SymbolKind = "module"
	KindInterface SymbolKind = "interface"
	KindModport   SymbolKind = "modport"
	KindPort      SymbolKind = "port"
	KindNet       SymbolKind = "net"
	KindParam     SymbolKind = "parameter"
	KindInstance  SymbolKind = "instance"
	KindFunction   SymbolKind = "function"
	KindTask       SymbolKind = "task"
	KindProperty   SymbolKind = "property"
	KindClass      SymbolKind = "class"
	KindConstraint SymbolKind = "constraint"
	KindClocking   SymbolKind = "clocking"
	KindCovergroup SymbolKind = "covergroup"
	KindType       SymbolKind = "type"
	KindField      SymbolKind = "field"
	KindPackage    SymbolKind = "package"
)

// Symbol represents a named declaration in a Verilog source file.
type Symbol struct {
	Name           string
	Kind           SymbolKind
	TypeDetail     string
	TypeName       string
	Signature      string
	Range          ast.Range
	SelectionRange ast.Range
	TargetModule   string
	ModuleName     string
	PackageName    string
	InterfaceName  string
	ClassName      string
	Fields         []*Symbol
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

// TaskScope holds the local symbol table for a task declaration.
type TaskScope struct {
	Decl    *ast.TaskDecl
	Symbol  *Symbol
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// PropertyScope holds the local symbol table for a SystemVerilog property or sequence declaration.
type PropertyScope struct {
	Decl    *ast.PropertyDecl
	Symbol  *Symbol
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// CovergroupScope holds the local symbol table for a SystemVerilog covergroup declaration.
type CovergroupScope struct {
	Decl    *ast.CovergroupDecl
	Symbol  *Symbol
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// BlockScope holds the local symbol table for a procedural block (always / initial).
type BlockScope struct {
	Block   *ast.ProceduralBlock
	Symbols map[string]*Symbol
	Order   []*Symbol
}

// ClassScope holds the symbol table for a single SystemVerilog class declaration.
type ClassScope struct {
	Class       *ast.ClassDecl
	Symbol      *Symbol
	Symbols     map[string]*Symbol
	Order       []*Symbol
	Functions   []*FunctionScope
	Tasks       []*TaskScope
	Properties  []*PropertyScope
	Covergroups []*CovergroupScope
	Classes     []*ClassScope
}

// ModuleScope holds the symbol table for a single module declaration.
type ModuleScope struct {
	Module      *ast.ModuleDecl
	Symbol      *Symbol
	Symbols     map[string]*Symbol
	Order       []*Symbol
	Functions   []*FunctionScope
	Tasks       []*TaskScope
	Properties  []*PropertyScope
	Covergroups []*CovergroupScope
	Classes     []*ClassScope
	Blocks      []*BlockScope
}

// PackageScope holds the symbol table for a single SystemVerilog package declaration.
type PackageScope struct {
	Package     *ast.PackageDecl
	Symbol      *Symbol
	Symbols     map[string]*Symbol
	Order       []*Symbol
	Functions   []*FunctionScope
	Tasks       []*TaskScope
	Properties  []*PropertyScope
	Covergroups []*CovergroupScope
	Classes     []*ClassScope
}

// InterfaceScope holds the symbol table for a single SystemVerilog interface declaration.
type InterfaceScope struct {
	Interface   *ast.InterfaceDecl
	Symbol      *Symbol
	Symbols     map[string]*Symbol
	Order       []*Symbol
	Modports    map[string]*Symbol
	Functions   []*FunctionScope
	Tasks       []*TaskScope
	Properties  []*PropertyScope
	Covergroups []*CovergroupScope
	Classes     []*ClassScope
	Blocks      []*BlockScope
}

// WorkspaceContext provides cross-file modules, packages, interfaces, and classes for semantic analysis.
type WorkspaceContext struct {
	Modules    map[string]*ModuleScope
	Packages   map[string]*PackageScope
	Interfaces map[string]*InterfaceScope
	Classes    map[string]*ClassScope
}

// Analysis holds the symbol tables and semantic diagnostics for a SourceFile.
type Analysis struct {
	File           *ast.SourceFile
	Modules        map[string]*ModuleScope
	ModuleOrder    []*ModuleScope
	AllModules     map[string]*ModuleScope
	Interfaces     map[string]*InterfaceScope
	InterfaceOrder []*InterfaceScope
	AllInterfaces  map[string]*InterfaceScope
	Packages       map[string]*PackageScope
	PackageOrder   []*PackageScope
	AllPackages    map[string]*PackageScope
	Classes        map[string]*ClassScope
	ClassOrder     []*ClassScope
	AllClasses     map[string]*ClassScope
	GlobalSymbols  map[string]*Symbol
	GlobalOrder    []*Symbol
	Diagnostics    []SemanticDiagnostic
}

// Analyze builds symbol tables and performs semantic checks on file.
func Analyze(file *ast.SourceFile) *Analysis {
	return AnalyzeWithWorkspace(file, WorkspaceContext{})
}

// AnalyzeWithPackages builds symbol tables and performs semantic checks on file,
// resolving package imports and `pkg::sym` references against both local and external packages.
func AnalyzeWithPackages(file *ast.SourceFile, extPackages map[string]*PackageScope) *Analysis {
	return AnalyzeWithWorkspace(file, WorkspaceContext{Packages: extPackages})
}

// AnalyzeWithWorkspace builds symbol tables and performs semantic checks on file,
// resolving packages and interfaces against both local and workspace declarations.
func AnalyzeWithWorkspace(file *ast.SourceFile, ctx WorkspaceContext) *Analysis {
	allMods := make(map[string]*ModuleScope, len(ctx.Modules))
	for k, v := range ctx.Modules {
		allMods[k] = v
	}
	allPkgs := make(map[string]*PackageScope, len(ctx.Packages))
	for k, v := range ctx.Packages {
		allPkgs[k] = v
	}
	allIfaces := make(map[string]*InterfaceScope, len(ctx.Interfaces))
	for k, v := range ctx.Interfaces {
		allIfaces[k] = v
	}
	allClasses := make(map[string]*ClassScope, len(ctx.Classes))
	for k, v := range ctx.Classes {
		allClasses[k] = v
	}
	res := &Analysis{
		File:          file,
		Modules:       make(map[string]*ModuleScope),
		AllModules:    allMods,
		Interfaces:    make(map[string]*InterfaceScope),
		AllInterfaces: allIfaces,
		Packages:      make(map[string]*PackageScope),
		AllPackages:   allPkgs,
		Classes:       make(map[string]*ClassScope),
		AllClasses:    allClasses,
		GlobalSymbols: make(map[string]*Symbol),
	}
	if file == nil {
		return res
	}

	for name, clsScope := range allClasses {
		if clsScope != nil && clsScope.Symbol != nil {
			res.GlobalSymbols[name] = clsScope.Symbol
		}
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

	for _, td := range file.Typedefs {
		if td.Name != "" {
			detail := "typedef"
			sig := "typedef " + td.Name
			if td.BaseType != "" {
				detail = "typedef " + td.BaseType
				sig = "typedef " + td.BaseType + " " + td.Name
			}
			sym := &Symbol{
				Name:           td.Name,
				Kind:           KindType,
				TypeDetail:     detail,
				Signature:      sig,
				Range:          td.Range,
				SelectionRange: td.NameRange,
				Fields:         buildStructFieldSymbols(td.StructFields),
			}
			res.GlobalSymbols[td.Name] = sym
			res.GlobalOrder = append(res.GlobalOrder, sym)
		}
		for _, em := range td.EnumMembers {
			if em.Name == "" {
				continue
			}
			sig := "enum " + em.Name
			if em.ValueText != "" {
				sig += " = " + em.ValueText
			}
			sym := &Symbol{
				Name:           em.Name,
				Kind:           KindParam,
				TypeDetail:     "enum",
				Signature:      sig,
				Range:          em.Range,
				SelectionRange: em.NameRange,
			}
			res.GlobalSymbols[em.Name] = sym
			res.GlobalOrder = append(res.GlobalOrder, sym)
		}
	}

	for _, fn := range file.Functions {
		fnScope := buildFunctionScope(fn, &res.Diagnostics)
		if fnScope.Symbol.Name != "" {
			res.GlobalSymbols[fnScope.Symbol.Name] = fnScope.Symbol
			res.GlobalOrder = append(res.GlobalOrder, fnScope.Symbol)
		}
	}

	for _, tk := range file.Tasks {
		taskScope := buildTaskScope(tk, &res.Diagnostics)
		if taskScope.Symbol.Name != "" {
			res.GlobalSymbols[taskScope.Symbol.Name] = taskScope.Symbol
			res.GlobalOrder = append(res.GlobalOrder, taskScope.Symbol)
		}
	}

	for _, pkg := range file.Packages {
		pkgScope := analyzePackage(pkg, res.GlobalSymbols, res.AllPackages, res.AllInterfaces, &res.Diagnostics)
		res.PackageOrder = append(res.PackageOrder, pkgScope)
		if pkg.Name != "" {
			res.Packages[pkg.Name] = pkgScope
			res.AllPackages[pkg.Name] = pkgScope
		}
	}

	fileHasExtWildcard := false
	for _, imp := range file.Imports {
		if pkgScope, ok := res.AllPackages[imp.PackageName]; ok {
			if imp.SymbolName == "*" {
				for _, sym := range pkgScope.Order {
					if sym.Name != "" {
						if _, exists := res.GlobalSymbols[sym.Name]; !exists {
							res.GlobalSymbols[sym.Name] = sym
						}
					}
				}
			} else if sym, ok := pkgScope.Symbols[imp.SymbolName]; ok {
				if _, exists := res.GlobalSymbols[sym.Name]; !exists {
					res.GlobalSymbols[sym.Name] = sym
				}
			}
		} else if imp.SymbolName == "*" {
			fileHasExtWildcard = true
		}
	}

	for _, iface := range file.Interfaces {
		ifaceScope := analyzeInterface(iface, res.GlobalSymbols, res.AllPackages, res.AllInterfaces, fileHasExtWildcard, &res.Diagnostics)
		res.InterfaceOrder = append(res.InterfaceOrder, ifaceScope)
		if iface.Name != "" {
			res.Interfaces[iface.Name] = ifaceScope
			res.AllInterfaces[iface.Name] = ifaceScope
		}
	}

	for _, cls := range file.Classes {
		clsScope := analyzeClass(cls, res.GlobalSymbols, res.AllPackages, res.AllInterfaces, fileHasExtWildcard, &res.Diagnostics)
		res.ClassOrder = append(res.ClassOrder, clsScope)
		if cls.Name != "" {
			res.Classes[cls.Name] = clsScope
			res.AllClasses[cls.Name] = clsScope
			res.GlobalSymbols[cls.Name] = clsScope.Symbol
			res.GlobalOrder = append(res.GlobalOrder, clsScope.Symbol)
		}
	}

	for _, mod := range file.Modules {
		scope := analyzeModule(mod, res.GlobalSymbols, res.AllPackages, res.AllInterfaces, fileHasExtWildcard, &res.Diagnostics)
		res.ModuleOrder = append(res.ModuleOrder, scope)
		if mod.Name != "" {
			res.Modules[mod.Name] = scope
			res.AllModules[mod.Name] = scope
		}
	}

	for _, scope := range res.ModuleOrder {
		res.validateModuleInstantiations(scope)
	}

	for _, b := range file.Binds {
		res.validateBindStmt(b)
	}

	return res
}

func (a *Analysis) validateModuleInstantiations(scope *ModuleScope) {
	if scope == nil || scope.Module == nil {
		return
	}
	var checkItems func(items []ast.ModuleItem)
	checkItems = func(items []ast.ModuleItem) {
		for _, item := range items {
			switch d := item.(type) {
			case *ast.GenerateBlock:
				checkItems(d.Items)
			case *ast.ModuleInst:
				a.validateSingleInst(d, scope.Symbols)
			case *ast.BindStmt:
				a.validateBindStmt(d)
			}
		}
	}
	checkItems(scope.Module.Items)
}

func (a *Analysis) validateBindStmt(b *ast.BindStmt) {
	if b == nil || b.Inst == nil || b.Range.File != "" {
		return
	}
	var targetScopeSyms map[string]*Symbol
	if !b.IsHierarchical {
		if modScope, ok := a.AllModules[b.TargetName]; ok {
			targetScopeSyms = modScope.Symbols
		} else if ifaceScope, ok := a.AllInterfaces[b.TargetName]; ok {
			targetScopeSyms = ifaceScope.Symbols
		}
	}
	a.validateSingleInst(b.Inst, targetScopeSyms)

	// Validate signal expressions in the bound port connections against the target module/interface scope.
	if targetScopeSyms != nil {
		for _, ref := range b.Inst.Refs {
			if ref.Range.File != "" {
				continue
			}
			if _, resolved := resolveRefInScopes(ref.Name, nil, targetScopeSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); !resolved {
				a.Diagnostics = append(a.Diagnostics, SemanticDiagnostic{
					Range:   ref.Range,
					Message: fmt.Sprintf("undeclared identifier %q in bound target %q", ref.Name, b.TargetName),
					IsError: false,
				})
			}
		}
	}
}

func (a *Analysis) validateSingleInst(d *ast.ModuleInst, parentSyms map[string]*Symbol) {
	if d == nil || d.Range.File != "" {
		return
	}
	var targetSyms map[string]*Symbol
	var targetOrder []*Symbol
	targetKind := "module"
	if modScope, ok := a.AllModules[d.ModuleName]; ok {
		targetSyms = modScope.Symbols
		targetOrder = modScope.Order
	} else if ifaceScope, ok := a.AllInterfaces[d.ModuleName]; ok {
		targetSyms = ifaceScope.Symbols
		targetOrder = ifaceScope.Order
		targetKind = "interface"
	}

	connectedPorts := make(map[string]bool, len(d.PortRefs))
	for _, pref := range d.PortRefs {
		if pref.Range.File != "" {
			continue
		}
		if connectedPorts[pref.Name] {
			a.Diagnostics = append(a.Diagnostics, SemanticDiagnostic{
				Range:   pref.Range,
				Message: fmt.Sprintf("duplicate port connection %q in instance %q", pref.Name, d.InstanceName),
				IsError: false,
			})
		}
		connectedPorts[pref.Name] = true

		if targetSyms != nil {
			if sym, ok := targetSyms[pref.Name]; !ok || sym.Kind != KindPort {
				a.Diagnostics = append(a.Diagnostics, SemanticDiagnostic{
					Range:   pref.Range,
					Message: fmt.Sprintf("unknown port %q on %s %q", pref.Name, targetKind, d.ModuleName),
					IsError: false,
				})
			}
		}
	}

	if targetSyms != nil {
		for _, paramRef := range d.ParamRefs {
			if paramRef.Range.File != "" {
				continue
			}
			if sym, ok := targetSyms[paramRef.Name]; !ok || sym.Kind != KindParam {
				a.Diagnostics = append(a.Diagnostics, SemanticDiagnostic{
					Range:   paramRef.Range,
					Message: fmt.Sprintf("unknown parameter %q on %s %q", paramRef.Name, targetKind, d.ModuleName),
					IsError: false,
				})
			}
		}
	}

	if d.HasWildcard && targetOrder != nil && parentSyms != nil && d.WildcardRange.File == "" {
		for _, sym := range targetOrder {
			if sym.Kind != KindPort || sym.Name == "" || connectedPorts[sym.Name] {
				continue
			}
			if _, resolved := resolveRefInScopes(sym.Name, nil, parentSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); !resolved {
				a.Diagnostics = append(a.Diagnostics, SemanticDiagnostic{
					Range:   d.WildcardRange,
					Message: fmt.Sprintf("undeclared signal %q required by '.*' port connection of %s %q", sym.Name, targetKind, d.ModuleName),
					IsError: false,
				})
			}
		}
	}
}

func analyzeClass(cls *ast.ClassDecl, globals map[string]*Symbol, allPkgs map[string]*PackageScope, allIfaces map[string]*InterfaceScope, fileHasExtWildcard bool, diags *[]SemanticDiagnostic) *ClassScope {
	detail := "class"
	if cls.Virtual {
		detail = "virtual class"
	}
	if cls.BaseClass != "" {
		detail += " extends " + cls.BaseClass
	}
	sig := detail + " " + cls.Name
	if cls.BaseClass != "" {
		sig = "class " + cls.Name + " extends " + cls.BaseClass
		if cls.Virtual {
			sig = "virtual " + sig
		}
	}
	clsSym := &Symbol{
		Name:           cls.Name,
		Kind:           KindClass,
		TypeDetail:     detail,
		Signature:      sig,
		Range:          cls.Range,
		SelectionRange: cls.NameRange,
		ClassName:      cls.Name,
	}

	// Pre-populate inherited members from BaseClass (if known) into a copy of globals for method bodies,
	// and also inject `this` and `super` into the class scope.
	var baseSym *Symbol
	if cls.BaseClass != "" {
		if sym, ok := resolveRefInScopes(cls.BaseClass, nil, nil, globals, allPkgs, allIfaces, false); ok && sym != nil {
			baseSym = sym
		}
	}

	classGlobals := make(map[string]*Symbol, len(globals)+16)
	for k, v := range globals {
		classGlobals[k] = v
	}
	if baseSym != nil {
		for _, f := range baseSym.Fields {
			if f.Name != "" {
				classGlobals[f.Name] = f
			}
		}
	}

	dummyMod := &ast.ModuleDecl{
		Name:       cls.Name,
		NameRange:  cls.NameRange,
		Range:      cls.Range,
		Params:     cls.Params,
		Items:      cls.Items,
		Refs:       cls.Refs,
		MemberRefs: cls.MemberRefs,
	}

	// Note: inject `this` and `super` by adding synthetic NetDecls or symbols before checking refs.
	modScope := analyzeModuleWithClassSelf(dummyMod, clsSym, baseSym, classGlobals, allPkgs, allIfaces, fileHasExtWildcard, diags)

	var allMembers []*Symbol
	seenMember := make(map[string]bool)
	for _, sym := range modScope.Order {
		if sym.Name == "this" || sym.Name == "super" {
			continue
		}
		sym.ClassName = cls.Name
		seenMember[sym.Name] = true
		allMembers = append(allMembers, sym)
	}
	if baseSym != nil {
		for _, f := range baseSym.Fields {
			if f.Name != "" && !seenMember[f.Name] {
				seenMember[f.Name] = true
				allMembers = append(allMembers, f)
				if _, exists := modScope.Symbols[f.Name]; !exists {
					modScope.Symbols[f.Name] = f
				}
			}
		}
	}
	clsSym.Fields = allMembers
	clsSym.Children = modScope.Symbol.Children

	return &ClassScope{
		Class:      cls,
		Symbol:     clsSym,
		Symbols:    modScope.Symbols,
		Order:      allMembers,
		Functions:  modScope.Functions,
		Tasks:      modScope.Tasks,
		Properties: modScope.Properties,
		Classes:    modScope.Classes,
	}
}

func analyzeInterface(iface *ast.InterfaceDecl, globals map[string]*Symbol, allPkgs map[string]*PackageScope, allIfaces map[string]*InterfaceScope, fileHasExtWildcard bool, diags *[]SemanticDiagnostic) *InterfaceScope {
	detail := "interface"
	if len(iface.Ports) > 0 || len(iface.Modports) > 0 {
		detail = fmt.Sprintf("interface (%d ports, %d modports)", len(iface.Ports), len(iface.Modports))
	}
	ifaceSym := &Symbol{
		Name:           iface.Name,
		Kind:           KindInterface,
		TypeDetail:     detail,
		Signature:      formatInterfaceSignature(iface),
		Range:          iface.Range,
		SelectionRange: iface.NameRange,
		InterfaceName:  iface.Name,
	}

	dummyMod := &ast.ModuleDecl{
		Name:       iface.Name,
		NameRange:  iface.NameRange,
		Range:      iface.Range,
		Imports:    iface.Imports,
		Params:     iface.Params,
		Ports:      iface.Ports,
		Items:      iface.Items,
		Refs:       iface.Refs,
		MemberRefs: iface.MemberRefs,
	}
	modScope := analyzeModule(dummyMod, globals, allPkgs, allIfaces, fileHasExtWildcard, diags)

	modports := make(map[string]*Symbol)
	for _, sym := range modScope.Order {
		sym.InterfaceName = iface.Name
		for _, f := range sym.Fields {
			f.InterfaceName = iface.Name
		}
		if sym.Kind == KindModport && sym.Name != "" {
			modports[sym.Name] = sym
		}
	}
	ifaceSym.Children = modScope.Symbol.Children

	return &InterfaceScope{
		Interface:  iface,
		Symbol:     ifaceSym,
		Symbols:    modScope.Symbols,
		Order:      modScope.Order,
		Modports:   modports,
		Functions:  modScope.Functions,
		Tasks:      modScope.Tasks,
		Properties: modScope.Properties,
		Blocks:     modScope.Blocks,
	}
}

func analyzePackage(pkg *ast.PackageDecl, globals map[string]*Symbol, allPkgs map[string]*PackageScope, allIfaces map[string]*InterfaceScope, diags *[]SemanticDiagnostic) *PackageScope {
	pkgSym := &Symbol{
		Name:           pkg.Name,
		Kind:           KindPackage,
		TypeDetail:     "package",
		Signature:      "package " + pkg.Name,
		Range:          pkg.Range,
		SelectionRange: pkg.NameRange,
		PackageName:    pkg.Name,
	}

	dummyMod := &ast.ModuleDecl{
		Name:       pkg.Name,
		NameRange:  pkg.NameRange,
		Range:      pkg.Range,
		Params:     pkg.Params,
		Items:      pkg.Items,
		Refs:       pkg.Refs,
		MemberRefs: pkg.MemberRefs,
	}
	modScope := analyzeModule(dummyMod, globals, allPkgs, allIfaces, false, diags)

	// Tag all package symbols (and their struct fields) with PackageName for cross-file resolution.
	for _, sym := range modScope.Order {
		sym.PackageName = pkg.Name
		for _, f := range sym.Fields {
			f.PackageName = pkg.Name
		}
	}
	pkgSym.Children = modScope.Symbol.Children

	return &PackageScope{
		Package:    pkg,
		Symbol:     pkgSym,
		Symbols:    modScope.Symbols,
		Order:      modScope.Order,
		Functions:  modScope.Functions,
		Tasks:      modScope.Tasks,
		Properties: modScope.Properties,
		Classes:    modScope.Classes,
	}
}

func analyzeModule(mod *ast.ModuleDecl, globals map[string]*Symbol, allPkgs map[string]*PackageScope, allIfaces map[string]*InterfaceScope, fileHasExtWildcard bool, diags *[]SemanticDiagnostic) *ModuleScope {
	return analyzeModuleWithClassSelf(mod, nil, nil, globals, allPkgs, allIfaces, fileHasExtWildcard, diags)
}

func analyzeModuleWithClassSelf(mod *ast.ModuleDecl, selfClassSym, superClassSym *Symbol, globals map[string]*Symbol, allPkgs map[string]*PackageScope, allIfaces map[string]*InterfaceScope, fileHasExtWildcard bool, diags *[]SemanticDiagnostic) *ModuleScope {
	modKindLabel := "module"
	if mod.IsProgram {
		modKindLabel = "program"
	} else if mod.IsChecker {
		modKindLabel = "checker"
	}
	modSym := &Symbol{
		Name:           mod.Name,
		Kind:           KindModule,
		TypeDetail:     fmt.Sprintf("%s (%d ports)", modKindLabel, len(mod.Ports)),
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
			// and a net/reg declaration (e.g., "reg [7:0] y;") may appear in either order. Merge them cleanly.
			if existing.Kind == KindPort && sym.Kind == KindNet &&
				!strings.Contains(existing.TypeDetail, "wire") &&
				!strings.Contains(existing.TypeDetail, "reg") {
				existing.TypeDetail = formatPortDetail(existing.TypeDetail, sym.TypeDetail)
				existing.Signature = existing.TypeDetail + " " + existing.Name
				return
			}
			if existing.Kind == KindNet && sym.Kind == KindPort &&
				!strings.Contains(sym.TypeDetail, "wire") &&
				!strings.Contains(sym.TypeDetail, "reg") {
				existing.Kind = KindPort
				existing.TypeDetail = formatPortDetail(sym.TypeDetail, existing.TypeDetail)
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
		if mod.Name != "" {
			sym.ModuleName = mod.Name
		}
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
					TypeName:       d.NetType,
					Signature:      detail + " " + d.Name,
					Range:          d.Range,
					SelectionRange: d.NameRange,
					TargetModule:   d.InterfaceName,
				}, inGenerate)

			case *ast.ModportDecl:
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindModport,
					TypeDetail:     "modport",
					Signature:      formatModportSignature(d),
					Range:          d.Range,
					SelectionRange: d.NameRange,
				}, inGenerate)

			case *ast.ClockingDecl:
				if d.Name != "" {
					detail := "clocking"
					if d.Default {
						detail = "default clocking"
					} else if d.Global {
						detail = "global clocking"
					}
					cbFields := make([]*Symbol, 0, len(d.Signals))
					for _, s := range d.Signals {
						if s.Name == "" {
							continue
						}
						cbFields = append(cbFields, &Symbol{
							Name:           s.Name,
							Kind:           KindField,
							TypeDetail:     "clocking " + s.Direction,
							Signature:      s.Direction + " " + s.Name,
							Range:          s.Range,
							SelectionRange: s.NameRange,
						})
					}
					addSymbol(&Symbol{
						Name:           d.Name,
						Kind:           KindClocking,
						TypeDetail:     detail,
						Signature:      detail + " " + d.Name,
						Range:          d.Range,
						SelectionRange: d.NameRange,
						Fields:         cbFields,
						Children:       cbFields,
					}, inGenerate)
				}

			case *ast.ModuleDecl:
				// Nested program or module declaration
				subScope := analyzeModule(d, globals, allPkgs, allIfaces, fileHasExtWildcard, diags)
				addSymbol(subScope.Symbol, inGenerate)

			case *ast.TypedefDecl:
				detail := "typedef"
				sig := "typedef " + d.Name
				if d.BaseType != "" {
					detail = "typedef " + d.BaseType
					sig = "typedef " + d.BaseType + " " + d.Name
				}
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindType,
					TypeDetail:     detail,
					Signature:      sig,
					Range:          d.Range,
					SelectionRange: d.NameRange,
					Fields:         buildStructFieldSymbols(d.StructFields),
				}, inGenerate)
				for _, em := range d.EnumMembers {
					emSig := "enum " + em.Name
					if em.ValueText != "" {
						emSig += " = " + em.ValueText
					}
					addSymbol(&Symbol{
						Name:           em.Name,
						Kind:           KindParam,
						TypeDetail:     "enum",
						Signature:      emSig,
						Range:          em.Range,
						SelectionRange: em.NameRange,
					}, inGenerate)
				}

			case *ast.NetDecl:
				detail := formatNetDeclDetail(d)
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindNet,
					TypeDetail:     detail,
					TypeName:       d.Kind,
					Signature:      detail + " " + d.Name,
					Range:          d.Range,
					SelectionRange: d.NameRange,
					Fields:         buildStructFieldSymbols(d.StructFields),
				}, inGenerate)
				for _, em := range d.EnumMembers {
					emSig := "enum " + em.Name
					if em.ValueText != "" {
						emSig += " = " + em.ValueText
					}
					addSymbol(&Symbol{
						Name:           em.Name,
						Kind:           KindParam,
						TypeDetail:     "enum",
						Signature:      emSig,
						Range:          em.Range,
						SelectionRange: em.NameRange,
					}, inGenerate)
				}

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

			case *ast.TaskDecl:
				taskScope := buildTaskScope(d, diags)
				addSymbol(taskScope.Symbol, inGenerate)
				scope.Tasks = append(scope.Tasks, taskScope)

			case *ast.PropertyDecl:
				propScope := buildPropertyScope(d, diags)
				addSymbol(propScope.Symbol, inGenerate)
				scope.Properties = append(scope.Properties, propScope)

			case *ast.CovergroupDecl:
				cgScope := buildCovergroupScope(d, diags)
				if cgScope.Symbol.Name != "" {
					addSymbol(cgScope.Symbol, inGenerate)
				}
				scope.Covergroups = append(scope.Covergroups, cgScope)

			case *ast.ClassDecl:
				mergedGlobals := make(map[string]*Symbol, len(globals)+len(scope.Symbols))
				for k, v := range globals {
					mergedGlobals[k] = v
				}
				for k, v := range scope.Symbols {
					mergedGlobals[k] = v
				}
				clsScope := analyzeClass(d, mergedGlobals, allPkgs, allIfaces, fileHasExtWildcard, diags)
				addSymbol(clsScope.Symbol, inGenerate)
				scope.Classes = append(scope.Classes, clsScope)

			case *ast.ConstraintDecl:
				addSymbol(&Symbol{
					Name:           d.Name,
					Kind:           KindConstraint,
					TypeDetail:     "constraint",
					Signature:      "constraint " + d.Name,
					Range:          d.Range,
					SelectionRange: d.NameRange,
				}, inGenerate)

			case *ast.AssertStmt:
				if d.Label != "" {
					detail := d.Kind
					if d.Concurrent {
						detail += " property"
					}
					addSymbol(&Symbol{
						Name:           d.Label,
						Kind:           KindInstance,
						TypeDetail:     detail,
						Signature:      detail + " " + d.Label,
						Range:          d.Range,
						SelectionRange: d.LabelRange,
					}, inGenerate)
				}

			case *ast.ProceduralBlock:
				blkScope := buildBlockScope(d, diags)
				scope.Blocks = append(scope.Blocks, blkScope)
				for _, lbl := range d.Labels {
					if lbl == "" {
						continue
					}
					if _, exists := scope.Symbols[lbl]; !exists {
						scope.Symbols[lbl] = &Symbol{
							Name:           lbl,
							Kind:           KindInstance,
							TypeDetail:     "named block",
							Signature:      "begin : " + lbl,
							Range:          d.Range,
							SelectionRange: d.Range,
						}
					}
				}

			case *ast.GenerateBlock:
				if d.Label != "" {
					if _, exists := scope.Symbols[d.Label]; !exists {
						scope.Symbols[d.Label] = &Symbol{
							Name:           d.Label,
							Kind:           KindInstance,
							TypeDetail:     "generate block",
							Signature:      "generate " + d.Label,
							Range:          d.Range,
							SelectionRange: d.Range,
						}
					}
				}
				collectItems(d.Items, true)
			}
		}
	}

	hasExtWildcard := fileHasExtWildcard
	importPackage := func(imp *ast.ImportDecl) {
		if pkgScope, ok := allPkgs[imp.PackageName]; ok {
			if imp.SymbolName == "*" {
				for _, sym := range pkgScope.Order {
					if sym.Name != "" {
						if _, exists := scope.Symbols[sym.Name]; !exists {
							scope.Symbols[sym.Name] = sym
						}
					}
				}
			} else if sym, ok := pkgScope.Symbols[imp.SymbolName]; ok {
				if _, exists := scope.Symbols[sym.Name]; !exists {
					scope.Symbols[sym.Name] = sym
				}
			}
		} else if imp.SymbolName == "*" {
			hasExtWildcard = true
		}
	}

	for _, imp := range mod.Imports {
		importPackage(imp)
	}

	collectItems(mod.Items, false)

	if selfClassSym != nil {
		// Populate selfClassSym.Fields with declared members so `this.member` resolves inside class methods.
		selfFields := make([]*Symbol, 0, len(scope.Order))
		selfFields = append(selfFields, scope.Order...)
		if superClassSym != nil {
			for _, f := range superClassSym.Fields {
				if _, exists := scope.Symbols[f.Name]; !exists {
					selfFields = append(selfFields, f)
					scope.Symbols[f.Name] = f
				}
			}
		}
		selfClassSym.Fields = selfFields
		scope.Symbols["this"] = &Symbol{
			Name:           "this",
			Kind:           KindNet,
			TypeDetail:     selfClassSym.Name,
			TypeName:       selfClassSym.Name,
			Signature:      selfClassSym.Signature,
			Range:          selfClassSym.Range,
			SelectionRange: selfClassSym.SelectionRange,
			Fields:         selfFields,
		}
		if superClassSym != nil {
			scope.Symbols["super"] = &Symbol{
				Name:           "super",
				Kind:           KindNet,
				TypeDetail:     superClassSym.Name,
				TypeName:       superClassSym.Name,
				Signature:      superClassSym.Signature,
				Range:          superClassSym.Range,
				SelectionRange: superClassSym.SelectionRange,
				Fields:         superClassSym.Fields,
			}
		}
	}

	// Also apply any imports placed inside generate blocks or module items.
	for _, item := range mod.Items {
		if imp, ok := item.(*ast.ImportDecl); ok {
			importPackage(imp)
		}
	}

	checkRef := func(ref ast.IdentifierRef, localSyms map[string]*Symbol) {
		if _, resolved := resolveRefInScopes(ref.Name, localSyms, scope.Symbols, globals, allPkgs, allIfaces, hasExtWildcard); resolved {
			return
		}
		if ref.Range.File == "" {
			*diags = append(*diags, SemanticDiagnostic{
				Range:   ref.Range,
				Message: fmt.Sprintf("undeclared identifier %q", ref.Name),
				IsError: false,
			})
		}
	}

	// Check identifier references inside functions against local, module, and global scopes.
	for _, fnScope := range scope.Functions {
		for _, ref := range fnScope.Decl.Refs {
			checkRef(ref, fnScope.Symbols)
		}
	}

	// Check identifier references inside tasks against local, module, and global scopes.
	for _, taskScope := range scope.Tasks {
		for _, ref := range taskScope.Decl.Refs {
			checkRef(ref, taskScope.Symbols)
		}
	}

	// Check identifier references inside properties/sequences against local, module, and global scopes.
	for _, propScope := range scope.Properties {
		for _, ref := range propScope.Decl.Refs {
			checkRef(ref, propScope.Symbols)
		}
	}

	// Check identifier references inside covergroups against local, module, and global scopes.
	for _, cgScope := range scope.Covergroups {
		for _, ref := range cgScope.Decl.Refs {
			checkRef(ref, cgScope.Symbols)
		}
	}

	// Check identifier references inside procedural blocks against block-local, module, and global scopes.
	for _, blkScope := range scope.Blocks {
		for _, ref := range blkScope.Block.Refs {
			checkRef(ref, blkScope.Symbols)
		}
	}

	// Check identifier references against the module's symbol table and global symbols.
	for _, ref := range mod.Refs {
		checkRef(ref, nil)
	}

	// Validate member accesses against known structs and interfaces.
	for _, mref := range mod.MemberRefs {
		if mref.Range.File != "" {
			continue
		}
		var localSyms map[string]*Symbol
		pos := mref.Range.Start
		for _, fnScope := range scope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				localSyms = fnScope.Symbols
				break
			}
		}
		if localSyms == nil {
			for _, taskScope := range scope.Tasks {
				if taskScope.Decl.Range.Contains(pos) {
					localSyms = taskScope.Symbols
					break
				}
			}
		}
		if localSyms == nil {
			for _, blkScope := range scope.Blocks {
				if blkScope.Block.Range.Contains(pos) {
					localSyms = blkScope.Symbols
					break
				}
			}
		}
		baseSym, ok := resolveRefInScopes(mref.BaseName, localSyms, scope.Symbols, globals, allPkgs, allIfaces, false)
		if !ok || baseSym == nil {
			continue
		}
		// Check inline struct/union fields
		if len(baseSym.Fields) > 0 {
			if findFieldByName(baseSym.Fields, mref.MemberName) == nil {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   mref.Range,
					Message: fmt.Sprintf("unknown field %q of %q", mref.MemberName, mref.BaseName),
					IsError: false,
				})
			}
			continue
		}
		// Check known interface members (signals, modports, functions, tasks, params)
		ifaceName := baseSym.TargetModule
		if ifaceName == "" {
			ifaceName = baseSym.TypeName
		}
		if ifaceScope, isIface := allIfaces[ifaceName]; isIface {
			if _, exists := ifaceScope.Symbols[mref.MemberName]; !exists {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   mref.Range,
					Message: fmt.Sprintf("unknown member %q of interface %q", mref.MemberName, ifaceName),
					IsError: false,
				})
			}
			continue
		}
		// Check typed struct/union/class variable
		if baseSym.TypeName != "" {
			if typeSym, found := resolveRefInScopes(baseSym.TypeName, localSyms, scope.Symbols, globals, allPkgs, allIfaces, false); found && typeSym != nil && len(typeSym.Fields) > 0 {
				if findFieldByName(typeSym.Fields, mref.MemberName) == nil {
					kindLabel := "struct"
					if typeSym.Kind == KindClass {
						kindLabel = "class"
					}
					*diags = append(*diags, SemanticDiagnostic{
						Range:   mref.Range,
						Message: fmt.Sprintf("unknown field %q of %s %q", mref.MemberName, kindLabel, typeSym.Name),
						IsError: false,
					})
				}
			}
		}
	}

	validateModuleWrites(mod, scope, diags)

	return scope
}

func isNetOnlyKind(detail string) bool {
	// True for classic net types that cannot be assigned procedurally (`wire`, `tri`, `uwire`, `wand`, `wor`, `supply0`, `supply1`).
	fields := strings.Fields(detail)
	for _, f := range fields {
		switch f {
		case "reg", "logic", "bit", "byte", "shortint", "int", "longint", "integer", "real", "time", "realtime", "string":
			return false
		}
	}
	for _, f := range fields {
		switch f {
		case "wire", "uwire", "tri", "tri0", "tri1", "triand", "trior", "trireg", "wand", "wor", "supply0", "supply1":
			return true
		}
	}
	return false
}

func validateModuleWrites(mod *ast.ModuleDecl, scope *ModuleScope, diags *[]SemanticDiagnostic) {
	if mod == nil || scope == nil {
		return
	}

	type driverInfo struct {
		procID   int
		isSVProc bool // true for always_comb, always_latch, always_ff
		rng      ast.Range
	}
	drivers := make(map[string]driverInfo)
	nextProcID := 1

	recordDriver := func(w ast.WriteRef, procID int, isSVProc bool) {
		if w.IsMemberWrite || w.Range.File != "" {
			return
		}
		if prev, exists := drivers[w.Name]; exists {
			if prev.procID != procID && (prev.isSVProc || isSVProc) {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   w.Range,
					Message: fmt.Sprintf("multiple drivers to %q (also driven at line %d)", w.Name, prev.rng.Start.Line+1),
					IsError: false,
				})
			}
			return
		}
		drivers[w.Name] = driverInfo{
			procID:   procID,
			isSVProc: isSVProc,
			rng:      w.Range,
		}
	}

	var checkItems func(items []ast.ModuleItem, inGenerate bool)
	checkItems = func(items []ast.ModuleItem, inGenerate bool) {
		for _, item := range items {
			switch d := item.(type) {
			case *ast.GenerateBlock:
				checkItems(d.Items, true)
			case *ast.AssignStmt:
				procID := nextProcID
				nextProcID++
				for _, w := range d.Writes {
					if w.Range.File != "" {
						continue
					}
					if sym, ok := scope.Symbols[w.Name]; ok {
						if sym.Kind == KindPort && strings.HasPrefix(sym.TypeDetail, "input") {
							*diags = append(*diags, SemanticDiagnostic{
								Range:   w.Range,
								Message: fmt.Sprintf("cannot assign to input port %q", w.Name),
								IsError: false,
							})
						}
					}
					if !inGenerate {
						recordDriver(w, procID, false)
					}
				}
			}
		}
	}
	checkItems(mod.Items, false)

	for _, blkScope := range scope.Blocks {
		blk := blkScope.Block
		if blk == nil {
			continue
		}
		procID := nextProcID
		nextProcID++
		isSVProc := blk.Kind == "always_comb" || blk.Kind == "always_latch" || blk.Kind == "always_ff"

		for _, w := range blk.Writes {
			if w.Range.File != "" {
				continue
			}
			_, isLocal := blkScope.Symbols[w.Name]

			// 1. Check assignment to input port or net (`wire`)
			if !isLocal {
				if sym, ok := scope.Symbols[w.Name]; ok {
					if sym.Kind == KindPort && strings.HasPrefix(sym.TypeDetail, "input") {
						*diags = append(*diags, SemanticDiagnostic{
							Range:   w.Range,
							Message: fmt.Sprintf("cannot assign to input port %q", w.Name),
							IsError: false,
						})
					} else if !w.IsMemberWrite && isNetOnlyKind(sym.TypeDetail) {
						*diags = append(*diags, SemanticDiagnostic{
							Range:   w.Range,
							Message: fmt.Sprintf("cannot procedurally assign to net %q (%s)", w.Name, sym.TypeDetail),
							IsError: false,
						})
					}
				}
			}

			// 2. Check blocking vs non-blocking assignment rules in always_comb / always_latch / always_ff
			if (blk.Kind == "always_comb" || blk.Kind == "always_latch") && w.Op == "<=" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   w.Range,
					Message: fmt.Sprintf("non-blocking assignment '<=' in %s block (use '=')", blk.Kind),
					IsError: false,
				})
			} else if blk.Kind == "always_ff" && w.Op != "<=" && !isLocal {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   w.Range,
					Message: fmt.Sprintf("blocking assignment %q to %q in always_ff block (use '<=')", w.Op, w.Name),
					IsError: false,
				})
			}

			// 3. Check multiple drivers across always_comb / always_latch / always_ff and assign
			if !isLocal && isSVProc {
				recordDriver(w, procID, true)
			}
		}
	}
}

func findFieldByName(fields []*Symbol, name string) *Symbol {
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func resolveRefInScopes(
	name string,
	localSyms map[string]*Symbol,
	modSyms map[string]*Symbol,
	globals map[string]*Symbol,
	allPkgs map[string]*PackageScope,
	allIfaces map[string]*InterfaceScope,
	hasExtWildcard bool,
) (*Symbol, bool) {
	if strings.HasPrefix(name, "`") {
		return nil, true
	}
	if pkgName, symName, hasScope := strings.Cut(name, "::"); hasScope {
		if pkgScope, ok := allPkgs[pkgName]; ok {
			if sym, found := pkgScope.Symbols[symName]; found {
				return sym, true
			}
			return nil, false
		}
		// External package not loaded in single-file analysis: avoid false-positive warning.
		return nil, true
	}
	if localSyms != nil {
		if sym, ok := localSyms[name]; ok {
			return sym, true
		}
	}
	if modSyms != nil {
		if sym, ok := modSyms[name]; ok {
			return sym, true
		}
	}
	if globals != nil {
		if sym, ok := globals[name]; ok {
			return sym, true
		}
	}
	if allIfaces != nil {
		if ifaceScope, ok := allIfaces[name]; ok {
			return ifaceScope.Symbol, true
		}
	}
	if hasExtWildcard {
		return nil, true
	}
	return nil, false
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

// ClassAt returns the ClassScope enclosing pos (top-level, package, module, or interface class), or nil.
func (a *Analysis) ClassAt(pos ast.Position) *ClassScope {
	for _, clsScope := range a.ClassOrder {
		if clsScope.Class.Range.Contains(pos) {
			return clsScope
		}
	}
	for _, pkgScope := range a.PackageOrder {
		for _, clsScope := range pkgScope.Classes {
			if clsScope.Class.Range.Contains(pos) {
				return clsScope
			}
		}
	}
	for _, modScope := range a.ModuleOrder {
		for _, clsScope := range modScope.Classes {
			if clsScope.Class.Range.Contains(pos) {
				return clsScope
			}
		}
	}
	for _, ifaceScope := range a.InterfaceOrder {
		for _, clsScope := range ifaceScope.Classes {
			if clsScope.Class.Range.Contains(pos) {
				return clsScope
			}
		}
	}
	return nil
}

// InterfaceAt returns the InterfaceScope enclosing pos, or nil if pos is outside all interfaces.
func (a *Analysis) InterfaceAt(pos ast.Position) *InterfaceScope {
	for _, scope := range a.InterfaceOrder {
		if scope.Interface.Range.Contains(pos) {
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

		// Check top-level package imports.
		for _, imp := range a.File.Imports {
			if sym := a.resolveImportAt(imp, pos); sym != nil {
				return sym
			}
		}

		// Check top-level bind statements.
		for _, b := range a.File.Binds {
			if sym := a.resolveBindAt(b, pos); sym != nil {
				return sym
			}
		}
	}

	// Check package declarations and their contents.
	for _, pkgScope := range a.PackageOrder {
		pkg := pkgScope.Package
		if pkg.NameRange.Contains(pos) {
			return pkgScope.Symbol
		}
		if !pkg.Range.Contains(pos) {
			continue
		}
		for _, sym := range pkgScope.Order {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}
		for _, ref := range pkg.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := resolveRefInScopes(ref.Name, nil, pkgScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
					return sym
				}
			}
		}
	}

	// Check class declarations (top-level or nested in package/module/interface) and their contents.
	if clsScope := a.ClassAt(pos); clsScope != nil {
		cls := clsScope.Class
		if cls.NameRange.Contains(pos) {
			return clsScope.Symbol
		}
		for _, sym := range clsScope.Order {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}
		for _, fnScope := range clsScope.Functions {
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
					if sym, ok := resolveRefInScopes(ref.Name, fnScope.Symbols, clsScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		for _, taskScope := range clsScope.Tasks {
			if !taskScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range taskScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range taskScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, taskScope.Symbols, clsScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		if sym, _ := a.ResolveMemberAt(pos, nil); sym != nil {
			return sym
		}
		for _, ref := range cls.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := resolveRefInScopes(ref.Name, nil, clsScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
					return sym
				}
			}
		}
	}

	// Check interface declarations and their contents.
	for _, ifaceScope := range a.InterfaceOrder {
		iface := ifaceScope.Interface
		if iface.NameRange.Contains(pos) {
			return ifaceScope.Symbol
		}
		if !iface.Range.Contains(pos) {
			continue
		}
		for _, imp := range iface.Imports {
			if sym := a.resolveImportAt(imp, pos); sym != nil {
				return sym
			}
		}
		for _, sym := range ifaceScope.Order {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}
		for _, fnScope := range ifaceScope.Functions {
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
					if sym, ok := resolveRefInScopes(ref.Name, fnScope.Symbols, ifaceScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		for _, taskScope := range ifaceScope.Tasks {
			if !taskScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range taskScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range taskScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, taskScope.Symbols, ifaceScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		for _, propScope := range ifaceScope.Properties {
			if !propScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range propScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range propScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, propScope.Symbols, ifaceScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		for _, blkScope := range ifaceScope.Blocks {
			if !blkScope.Block.Range.Contains(pos) {
				continue
			}
			for _, sym := range blkScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range blkScope.Block.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, blkScope.Symbols, ifaceScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}
		for _, ref := range iface.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := resolveRefInScopes(ref.Name, nil, ifaceScope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
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

		// 0. Check module imports (`import pkg::SYM;`).
		for _, imp := range mod.Imports {
			if sym := a.resolveImportAt(imp, pos); sym != nil {
				return sym
			}
		}

		// 0b. Check interface port type and modport references (`axi_if.master bus`).
		for _, port := range mod.Ports {
			if port.InterfaceName != "" && port.InterfaceNameRange.Contains(pos) {
				if ifaceScope, ok := a.AllInterfaces[port.InterfaceName]; ok {
					return ifaceScope.Symbol
				}
				return &Symbol{
					Name:           port.InterfaceName,
					Kind:           KindInterface,
					TypeDetail:     "interface",
					Signature:      "interface " + port.InterfaceName,
					Range:          port.InterfaceNameRange,
					SelectionRange: port.InterfaceNameRange,
					InterfaceName:  port.InterfaceName,
				}
			}
			if port.ModportName != "" && port.ModportNameRange.Contains(pos) {
				if ifaceScope, ok := a.AllInterfaces[port.InterfaceName]; ok {
					if mpSym, found := ifaceScope.Modports[port.ModportName]; found {
						return mpSym
					}
				}
				return &Symbol{
					Name:           port.ModportName,
					Kind:           KindModport,
					TypeDetail:     "modport",
					Signature:      "modport " + port.ModportName,
					Range:          port.ModportNameRange,
					SelectionRange: port.ModportNameRange,
					InterfaceName:  port.InterfaceName,
				}
			}
		}

		// 1. Check declarations in the module.
		for _, sym := range scope.Order {
			if sym.SelectionRange.Contains(pos) {
				return sym
			}
		}

		// 2. Check module/interface instantiation type names and bind statements.
		if sym := a.findBindSymbolAt(mod.Items, pos); sym != nil {
			return sym
		}
		if inst := findModuleInstAt(mod.Items, pos); inst != nil {
			if targetScope, exists := a.Modules[inst.ModuleName]; exists {
				return targetScope.Symbol
			}
			if ifaceScope, exists := a.AllInterfaces[inst.ModuleName]; exists {
				return ifaceScope.Symbol
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
					if sym, ok := resolveRefInScopes(ref.Name, fnScope.Symbols, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}

		// 3b. Check declarations and references inside tasks.
		for _, taskScope := range scope.Tasks {
			if !taskScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range taskScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range taskScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, taskScope.Symbols, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}

		// 3b2. Check declarations and references inside properties and sequences.
		for _, propScope := range scope.Properties {
			if !propScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range propScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range propScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, propScope.Symbols, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}

		// 3b3. Check declarations and references inside covergroups.
		for _, cgScope := range scope.Covergroups {
			if !cgScope.Decl.Range.Contains(pos) {
				continue
			}
			for _, sym := range cgScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range cgScope.Decl.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, cgScope.Symbols, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}

		// 3c. Check declarations and references inside procedural blocks.
		for _, blkScope := range scope.Blocks {
			if !blkScope.Block.Range.Contains(pos) {
				continue
			}
			for _, sym := range blkScope.Order {
				if sym.SelectionRange.Contains(pos) {
					return sym
				}
			}
			for _, ref := range blkScope.Block.Refs {
				if ref.Range.Contains(pos) {
					if sym, ok := resolveRefInScopes(ref.Name, blkScope.Symbols, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
						return sym
					}
				}
			}
		}

		// 3d. Check named port/parameter connections (`.a(din)`, `.clk`, `.*`, `#(.WIDTH(8))`)
		// and member references (`bus.valid`, `pkt.addr`) before plain `mod.Refs` so implicit `.clk`
		// resolves to the target module's port when known.
		if sym, _ := a.ResolveInstPortAt(pos, nil); sym != nil {
			return sym
		}
		if sym, _ := a.ResolveMemberAt(pos, nil); sym != nil {
			return sym
		}

		// 4. Check identifier references in expressions, assignments, and procedural blocks.
		for _, ref := range mod.Refs {
			if ref.Range.Contains(pos) {
				if sym, ok := resolveRefInScopes(ref.Name, nil, scope.Symbols, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
					return sym
				}
			}
		}
	}

	return nil
}

// ResolveMemberAt resolves a dot-qualified member reference (`bus.valid`, `pkt.addr`, `u_sub.y`) at pos.
// It returns the resolved member symbol and, if the member belongs to a module (potentially external), that module name.
func (a *Analysis) ResolveMemberAt(pos ast.Position, extModuleLookup func(string) *ModuleScope) (*Symbol, string) {
	if a == nil {
		return nil, ""
	}
	var mref *ast.MemberRef
	var localSyms, modSyms map[string]*Symbol

	if clsScope := a.ClassAt(pos); clsScope != nil {
		modSyms = clsScope.Symbols
		for i := range clsScope.Class.MemberRefs {
			if clsScope.Class.MemberRefs[i].Range.Contains(pos) {
				mref = &clsScope.Class.MemberRefs[i]
				break
			}
		}
		if mref != nil {
			for _, fnScope := range clsScope.Functions {
				if fnScope.Decl.Range.Contains(pos) {
					localSyms = fnScope.Symbols
					break
				}
			}
			if localSyms == nil {
				for _, taskScope := range clsScope.Tasks {
					if taskScope.Decl.Range.Contains(pos) {
						localSyms = taskScope.Symbols
						break
					}
				}
			}
		}
	} else if scope := a.ModuleAt(pos); scope != nil {
		modSyms = scope.Symbols
		for i := range scope.Module.MemberRefs {
			if scope.Module.MemberRefs[i].Range.Contains(pos) {
				mref = &scope.Module.MemberRefs[i]
				break
			}
		}
		if mref != nil {
			for _, fnScope := range scope.Functions {
				if fnScope.Decl.Range.Contains(pos) {
					localSyms = fnScope.Symbols
					break
				}
			}
			if localSyms == nil {
				for _, taskScope := range scope.Tasks {
					if taskScope.Decl.Range.Contains(pos) {
						localSyms = taskScope.Symbols
						break
					}
				}
			}
			if localSyms == nil {
				for _, blkScope := range scope.Blocks {
					if blkScope.Block.Range.Contains(pos) {
						localSyms = blkScope.Symbols
						break
					}
				}
			}
		}
	} else if ifaceScope := a.InterfaceAt(pos); ifaceScope != nil {
		modSyms = ifaceScope.Symbols
		for i := range ifaceScope.Interface.MemberRefs {
			if ifaceScope.Interface.MemberRefs[i].Range.Contains(pos) {
				mref = &ifaceScope.Interface.MemberRefs[i]
				break
			}
		}
	} else {
		for _, pkgScope := range a.PackageOrder {
			if !pkgScope.Package.Range.Contains(pos) {
				continue
			}
			modSyms = pkgScope.Symbols
			for i := range pkgScope.Package.MemberRefs {
				if pkgScope.Package.MemberRefs[i].Range.Contains(pos) {
					mref = &pkgScope.Package.MemberRefs[i]
					break
				}
			}
			break
		}
	}

	if mref == nil {
		return nil, ""
	}

	baseSym, ok := resolveRefInScopes(mref.BaseName, localSyms, modSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false)
	if !ok || baseSym == nil {
		return nil, ""
	}

	if len(baseSym.Fields) > 0 {
		if f := findFieldByName(baseSym.Fields, mref.MemberName); f != nil {
			return f, ""
		}
	}

	if baseSym.TargetModule != "" {
		if ifaceScope, found := a.AllInterfaces[baseSym.TargetModule]; found {
			if sym, exists := ifaceScope.Symbols[mref.MemberName]; exists {
				return sym, ""
			}
		}
		if modScope, found := a.AllModules[baseSym.TargetModule]; found {
			if sym, exists := modScope.Symbols[mref.MemberName]; exists {
				owner := ""
				if _, inSame := a.Modules[baseSym.TargetModule]; !inSame {
					owner = baseSym.TargetModule
				}
				return sym, owner
			}
		}
		if extModuleLookup != nil {
			if modScope := extModuleLookup(baseSym.TargetModule); modScope != nil {
				if sym, exists := modScope.Symbols[mref.MemberName]; exists {
					return sym, baseSym.TargetModule
				}
			}
		}
	}

	if baseSym.TypeName != "" {
		if ifaceScope, found := a.AllInterfaces[baseSym.TypeName]; found {
			if sym, exists := ifaceScope.Symbols[mref.MemberName]; exists {
				return sym, ""
			}
		}
		if typeSym, found := resolveRefInScopes(baseSym.TypeName, localSyms, modSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); found && typeSym != nil && len(typeSym.Fields) > 0 {
			if f := findFieldByName(typeSym.Fields, mref.MemberName); f != nil {
				return f, ""
			}
		}
	}

	return nil, ""
}

// ResolveInstPortAt resolves a named port, implicit `.sig`, wildcard `.*`, or parameter connection at pos.
func (a *Analysis) ResolveInstPortAt(pos ast.Position, extModuleLookup func(string) *ModuleScope) (*Symbol, string) {
	inst := a.InstantiationAt(pos)
	if inst == nil {
		return nil, ""
	}

	resolveTargetScope := func() (map[string]*Symbol, []*Symbol, *Symbol, string) {
		if ifaceScope, found := a.AllInterfaces[inst.ModuleName]; found {
			owner := ""
			if _, inSame := a.Interfaces[inst.ModuleName]; !inSame {
				owner = inst.ModuleName
			}
			return ifaceScope.Symbols, ifaceScope.Order, ifaceScope.Symbol, owner
		}
		if modScope, found := a.AllModules[inst.ModuleName]; found {
			owner := ""
			if _, inSame := a.Modules[inst.ModuleName]; !inSame {
				owner = inst.ModuleName
			}
			return modScope.Symbols, modScope.Order, modScope.Symbol, owner
		}
		if extModuleLookup != nil {
			if modScope := extModuleLookup(inst.ModuleName); modScope != nil {
				return modScope.Symbols, modScope.Order, modScope.Symbol, inst.ModuleName
			}
		}
		return nil, nil, nil, ""
	}

	if inst.HasWildcard && inst.WildcardRange.Contains(pos) {
		_, targetOrder, targetSym, owner := resolveTargetScope()
		if targetSym != nil {
			explicit := make(map[string]bool, len(inst.PortRefs))
			for _, p := range inst.PortRefs {
				explicit[p.Name] = true
			}
			var parts []string
			for _, sym := range targetOrder {
				if sym.Kind == KindPort && !explicit[sym.Name] {
					parts = append(parts, sym.Signature)
				}
			}
			sig := fmt.Sprintf(".* -> %s", targetSym.Signature)
			if len(parts) > 0 {
				sig = fmt.Sprintf(".* -> %s (%s)", inst.ModuleName, strings.Join(parts, ", "))
			}
			return &Symbol{
				Name:           ".*",
				Kind:           targetSym.Kind,
				TypeDetail:     ".* wildcard connection",
				Signature:      sig,
				Range:          inst.WildcardRange,
				SelectionRange: targetSym.SelectionRange,
				ModuleName:     targetSym.Name,
				InterfaceName:  targetSym.InterfaceName,
			}, owner
		}
		return nil, ""
	}

	targetName := ""
	for _, pref := range inst.PortRefs {
		if pref.Range.Contains(pos) {
			targetName = pref.Name
			break
		}
	}
	if targetName == "" {
		for _, pref := range inst.ParamRefs {
			if pref.Range.Contains(pos) {
				targetName = pref.Name
				break
			}
		}
	}
	if targetName == "" {
		return nil, ""
	}

	targetSyms, _, _, owner := resolveTargetScope()
	if targetSyms != nil {
		if sym, exists := targetSyms[targetName]; exists {
			return sym, owner
		}
	}
	return nil, ""
}

func (a *Analysis) resolveImportAt(imp *ast.ImportDecl, pos ast.Position) *Symbol {
	if imp.PackageNameRange.Contains(pos) {
		if pkgScope, ok := a.AllPackages[imp.PackageName]; ok {
			return pkgScope.Symbol
		}
	}
	if imp.SymbolName != "" && imp.SymbolName != "*" && imp.SymbolNameRange.Contains(pos) {
		if pkgScope, ok := a.AllPackages[imp.PackageName]; ok {
			if sym, found := pkgScope.Symbols[imp.SymbolName]; found {
				return sym
			}
		}
	}
	return nil
}

// PackageCompletions returns all symbols declared in pkgName (local or workspace package).
func (a *Analysis) PackageCompletions(pkgName string) []*Symbol {
	if a == nil || pkgName == "" {
		return nil
	}
	if pkgScope, ok := a.AllPackages[pkgName]; ok {
		return pkgScope.Order
	}
	return nil
}

// MemberCompletions returns member symbols accessible via `baseName.` at pos:
//   - modports when baseName is an interface type (`axi_if.`)
//   - interface signals/functions/tasks/modports when baseName is an interface port or instance (`bus.`, `u_axi.`)
//   - module ports/params/signals when baseName is a module instance (`u_sub.`)
//   - struct/union fields when baseName is a struct/union variable (`pkt.`)
func (a *Analysis) MemberCompletions(pos ast.Position, baseName string, extModuleLookup func(string) *ModuleScope) []*Symbol {
	if a == nil || baseName == "" {
		return nil
	}

	// 1. Direct interface type name (`axi_if.` in a module port list) -> suggest modports.
	if ifaceScope, ok := a.AllInterfaces[baseName]; ok {
		var modports []*Symbol
		for _, sym := range ifaceScope.Order {
			if sym.Kind == KindModport {
				modports = append(modports, sym)
			}
		}
		if len(modports) > 0 {
			return modports
		}
		return ifaceScope.Order
	}

	// 2. Resolve baseName in the enclosing scope at pos.
	var localSyms, modSyms map[string]*Symbol
	if clsScope := a.ClassAt(pos); clsScope != nil {
		modSyms = clsScope.Symbols
		for _, fnScope := range clsScope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				localSyms = fnScope.Symbols
				break
			}
		}
		if localSyms == nil {
			for _, taskScope := range clsScope.Tasks {
				if taskScope.Decl.Range.Contains(pos) {
					localSyms = taskScope.Symbols
					break
				}
			}
		}
	} else if scope := a.ModuleAt(pos); scope != nil {
		modSyms = scope.Symbols
		for _, fnScope := range scope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				localSyms = fnScope.Symbols
				break
			}
		}
		if localSyms == nil {
			for _, taskScope := range scope.Tasks {
				if taskScope.Decl.Range.Contains(pos) {
					localSyms = taskScope.Symbols
					break
				}
			}
		}
		if localSyms == nil {
			for _, blkScope := range scope.Blocks {
				if blkScope.Block.Range.Contains(pos) {
					localSyms = blkScope.Symbols
					break
				}
			}
		}
	} else if ifaceScope := a.InterfaceAt(pos); ifaceScope != nil {
		modSyms = ifaceScope.Symbols
	}

	sym, ok := resolveRefInScopes(baseName, localSyms, modSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false)
	if !ok || sym == nil {
		return nil
	}

	// 2a. Inline struct/union variable.
	if len(sym.Fields) > 0 {
		return sym.Fields
	}

	// 2b. Interface port or module/interface instance.
	if sym.TargetModule != "" {
		if ifaceScope, found := a.AllInterfaces[sym.TargetModule]; found {
			return ifaceScope.Order
		}
		if modScope, found := a.Modules[sym.TargetModule]; found {
			return modScope.Order
		}
		if extModuleLookup != nil {
			if modScope := extModuleLookup(sym.TargetModule); modScope != nil {
				return modScope.Order
			}
		}
	}

	// 2c. Typed variable or port (`pkt_t pkt;`, `pkg::pkt_t pkt;`, or `axi_if u_if;`).
	if sym.TypeName != "" {
		if ifaceScope, found := a.AllInterfaces[sym.TypeName]; found {
			return ifaceScope.Order
		}
		if typeSym, found := resolveRefInScopes(sym.TypeName, localSyms, modSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); found && typeSym != nil && len(typeSym.Fields) > 0 {
			return typeSym.Fields
		}
	}

	return nil
}

func (a *Analysis) resolveBindAt(b *ast.BindStmt, pos ast.Position) *Symbol {
	if b == nil || !b.Range.Contains(pos) {
		return nil
	}
	if b.TargetNameRange.Contains(pos) && !b.IsHierarchical {
		if modScope, ok := a.AllModules[b.TargetName]; ok {
			return modScope.Symbol
		}
		if ifaceScope, ok := a.AllInterfaces[b.TargetName]; ok {
			return ifaceScope.Symbol
		}
		return &Symbol{
			Name:           b.TargetName,
			Kind:           KindModule,
			TypeDetail:     "module",
			Signature:      "module " + b.TargetName,
			Range:          b.TargetNameRange,
			SelectionRange: b.TargetNameRange,
		}
	}
	if b.Inst != nil {
		if b.Inst.ModuleNameRange.Contains(pos) {
			if modScope, ok := a.AllModules[b.Inst.ModuleName]; ok {
				return modScope.Symbol
			}
			if ifaceScope, ok := a.AllInterfaces[b.Inst.ModuleName]; ok {
				return ifaceScope.Symbol
			}
			return &Symbol{
				Name:           b.Inst.ModuleName,
				Kind:           KindModule,
				TypeDetail:     "module",
				Signature:      "module " + b.Inst.ModuleName,
				Range:          b.Inst.ModuleNameRange,
				SelectionRange: b.Inst.ModuleNameRange,
			}
		}
		if b.Inst.InstanceNameRange.Contains(pos) {
			return &Symbol{
				Name:           b.Inst.InstanceName,
				Kind:           KindInstance,
				TypeDetail:     "bind instance of " + b.Inst.ModuleName,
				Signature:      fmt.Sprintf("%s %s", b.Inst.ModuleName, b.Inst.InstanceName),
				Range:          b.Inst.Range,
				SelectionRange: b.Inst.InstanceNameRange,
				TargetModule:   b.Inst.ModuleName,
			}
		}
		if sym, _ := a.ResolveInstPortAt(pos, nil); sym != nil {
			return sym
		}
		// Resolve signal expressions inside the bound port connections against target module/interface scope.
		if !b.IsHierarchical {
			var targetSyms map[string]*Symbol
			if modScope, ok := a.AllModules[b.TargetName]; ok {
				targetSyms = modScope.Symbols
			} else if ifaceScope, ok := a.AllInterfaces[b.TargetName]; ok {
				targetSyms = ifaceScope.Symbols
			}
			if targetSyms != nil {
				for _, ref := range b.Inst.Refs {
					if ref.Range.Contains(pos) {
						if sym, ok := resolveRefInScopes(ref.Name, nil, targetSyms, a.GlobalSymbols, a.AllPackages, a.AllInterfaces, false); ok && sym != nil {
							return sym
						}
					}
				}
			}
		}
	}
	return nil
}

func (a *Analysis) findBindSymbolAt(items []ast.ModuleItem, pos ast.Position) *Symbol {
	for _, item := range items {
		switch d := item.(type) {
		case *ast.BindStmt:
			if sym := a.resolveBindAt(d, pos); sym != nil {
				return sym
			}
		case *ast.GenerateBlock:
			if sym := a.findBindSymbolAt(d.Items, pos); sym != nil {
				return sym
			}
		}
	}
	return nil
}

// InstantiationAt returns the ModuleInst enclosing pos, if any.
func (a *Analysis) InstantiationAt(pos ast.Position) *ast.ModuleInst {
	if a == nil {
		return nil
	}
	if a.File != nil {
		for _, b := range a.File.Binds {
			if b.Inst != nil && b.Inst.Range.Contains(pos) {
				return b.Inst
			}
		}
	}
	var findInItems func(items []ast.ModuleItem) *ast.ModuleInst
	findInItems = func(items []ast.ModuleItem) *ast.ModuleInst {
		for _, item := range items {
			switch d := item.(type) {
			case *ast.ModuleInst:
				if d.Range.Contains(pos) {
					return d
				}
			case *ast.BindStmt:
				if d.Inst != nil && d.Inst.Range.Contains(pos) {
					return d.Inst
				}
			case *ast.GenerateBlock:
				if found := findInItems(d.Items); found != nil {
					return found
				}
			}
		}
		return nil
	}
	if scope := a.ModuleAt(pos); scope != nil && scope.Module != nil {
		return findInItems(scope.Module.Items)
	}
	if ifaceScope := a.InterfaceAt(pos); ifaceScope != nil && ifaceScope.Interface != nil {
		return findInItems(ifaceScope.Interface.Items)
	}
	return nil
}

func buildStructFieldSymbols(fields []ast.StructField) []*Symbol {
	if len(fields) == 0 {
		return nil
	}
	out := make([]*Symbol, 0, len(fields))
	for _, f := range fields {
		if f.Name == "" {
			continue
		}
		sig := f.Name
		if f.TypeDetail != "" {
			sig = f.TypeDetail + " " + f.Name
		}
		out = append(out, &Symbol{
			Name:           f.Name,
			Kind:           KindField,
			TypeDetail:     f.TypeDetail,
			Signature:      sig,
			Range:          f.Range,
			SelectionRange: f.NameRange,
		})
	}
	return out
}

// CompletionsAt returns symbols available in scope at pos.
func (a *Analysis) CompletionsAt(pos ast.Position) []*Symbol {
	var out []*Symbol
	if clsScope := a.ClassAt(pos); clsScope != nil {
		for _, fnScope := range clsScope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				out = append(out, fnScope.Order...)
				break
			}
		}
		for _, taskScope := range clsScope.Tasks {
			if taskScope.Decl.Range.Contains(pos) {
				out = append(out, taskScope.Order...)
				break
			}
		}
		out = append(out, clsScope.Order...)
	} else if scope := a.ModuleAt(pos); scope != nil {
		for _, fnScope := range scope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				out = append(out, fnScope.Order...)
				break
			}
		}
		for _, taskScope := range scope.Tasks {
			if taskScope.Decl.Range.Contains(pos) {
				out = append(out, taskScope.Order...)
				break
			}
		}
		for _, blkScope := range scope.Blocks {
			if blkScope.Block.Range.Contains(pos) {
				out = append(out, blkScope.Order...)
				break
			}
		}
		out = append(out, scope.Order...)
	} else if ifaceScope := a.InterfaceAt(pos); ifaceScope != nil {
		for _, fnScope := range ifaceScope.Functions {
			if fnScope.Decl.Range.Contains(pos) {
				out = append(out, fnScope.Order...)
				break
			}
		}
		for _, taskScope := range ifaceScope.Tasks {
			if taskScope.Decl.Range.Contains(pos) {
				out = append(out, taskScope.Order...)
				break
			}
		}
		for _, blkScope := range ifaceScope.Blocks {
			if blkScope.Block.Range.Contains(pos) {
				out = append(out, blkScope.Order...)
				break
			}
		}
		out = append(out, ifaceScope.Order...)
	}
	out = append(out, a.GlobalOrder...)
	for _, ifaceScope := range a.InterfaceOrder {
		if ifaceScope.Symbol.Name != "" {
			out = append(out, ifaceScope.Symbol)
		}
	}
	for _, modScope := range a.ModuleOrder {
		if modScope.Symbol.Name != "" {
			out = append(out, modScope.Symbol)
		}
	}
	return out
}

func formatInterfaceSignature(iface *ast.InterfaceDecl) string {
	var b strings.Builder
	b.WriteString("interface ")
	b.WriteString(iface.Name)
	if len(iface.Ports) > 0 {
		b.WriteString(" (")
		for i, p := range iface.Ports {
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

func formatModportSignature(mp *ast.ModportDecl) string {
	var b strings.Builder
	b.WriteString("modport ")
	b.WriteString(mp.Name)
	if len(mp.Ports) > 0 {
		b.WriteString(" (")
		lastDir := ""
		for i, p := range mp.Ports {
			if i > 0 {
				b.WriteString(", ")
			}
			if p.Direction != "" && p.Direction != lastDir {
				b.WriteString(p.Direction)
				b.WriteString(" ")
				lastDir = p.Direction
			}
			b.WriteString(p.Name)
		}
		b.WriteString(")")
	}
	return b.String()
}

func formatModuleSignature(mod *ast.ModuleDecl) string {
	var b strings.Builder
	if mod.IsProgram {
		b.WriteString("program ")
	} else if mod.IsChecker {
		b.WriteString("checker ")
	} else {
		b.WriteString("module ")
	}
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
	if d.InterfaceName != "" {
		if d.ModportName != "" {
			return d.InterfaceName + "." + d.ModportName
		}
		return d.InterfaceName
	}
	if d.Direction == "interface" {
		if d.ModportName != "" {
			return "interface." + d.ModportName
		}
		return "interface"
	}
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
		case *ast.TypedefDecl:
			tdDetail := "typedef"
			tdSig := "typedef " + d.Name
			if d.BaseType != "" {
				tdDetail = "typedef " + d.BaseType
				tdSig = "typedef " + d.BaseType + " " + d.Name
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindType,
				TypeDetail:     tdDetail,
				Signature:      tdSig,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
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
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
		}
	}

	return fs
}

func formatFunctionReturnType(fn *ast.FunctionDecl) string {
	if fn.IsLet {
		return "let"
	}
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
	if fn.IsLet {
		var b strings.Builder
		b.WriteString("let ")
		b.WriteString(fn.Name)
		if len(fn.Params) > 0 {
			b.WriteString("(")
			for i, p := range fn.Params {
				if i > 0 {
					b.WriteString(", ")
				}
				if p.NetType != "" {
					b.WriteString(p.NetType)
					if p.Width != nil {
						b.WriteString(" ")
						b.WriteString(p.Width.Raw)
					}
					b.WriteString(" ")
				} else if p.Width != nil {
					b.WriteString(p.Width.Raw)
					b.WriteString(" ")
				}
				b.WriteString(p.Name)
			}
			b.WriteString(")")
		}
		return b.String()
	}
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

func buildTaskScope(task *ast.TaskDecl, diags *[]SemanticDiagnostic) *TaskScope {
	detail := "task"
	if task.Automatic {
		detail = "task automatic"
	}
	sig := formatTaskSignature(task)
	taskSym := &Symbol{
		Name:           task.Name,
		Kind:           KindTask,
		TypeDetail:     detail,
		Signature:      sig,
		Range:          task.Range,
		SelectionRange: task.NameRange,
	}

	ts := &TaskScope{
		Decl:    task,
		Symbol:  taskSym,
		Symbols: make(map[string]*Symbol),
	}

	addLocal := func(sym *Symbol) {
		if sym.Name == "" {
			return
		}
		if _, exists := ts.Symbols[sym.Name]; exists {
			if sym.SelectionRange.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		ts.Symbols[sym.Name] = sym
		ts.Order = append(ts.Order, sym)
		if sym.Range.File == "" {
			taskSym.Children = append(taskSym.Children, sym)
		}
	}

	for _, p := range task.Params {
		pDetail := formatPortDeclDetail(p)
		addLocal(&Symbol{
			Name:           p.Name,
			Kind:           KindPort,
			TypeDetail:     pDetail,
			Signature:      pDetail + " " + p.Name,
			Range:          p.Range,
			SelectionRange: p.NameRange,
		})
	}

	for _, item := range task.Locals {
		switch d := item.(type) {
		case *ast.ParamDecl:
			pDetail := formatParamDetail(d)
			s := pDetail + " " + d.Name
			if d.ValueText != "" {
				s += " = " + d.ValueText
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindParam,
				TypeDetail:     pDetail,
				Signature:      s,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
		case *ast.TypedefDecl:
			tdDetail := "typedef"
			tdSig := "typedef " + d.Name
			if d.BaseType != "" {
				tdDetail = "typedef " + d.BaseType
				tdSig = "typedef " + d.BaseType + " " + d.Name
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindType,
				TypeDetail:     tdDetail,
				Signature:      tdSig,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
		case *ast.NetDecl:
			nDetail := formatNetDeclDetail(d)
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindNet,
				TypeDetail:     nDetail,
				Signature:      nDetail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
		}
	}

	return ts
}

func formatTaskSignature(task *ast.TaskDecl) string {
	var b strings.Builder
	b.WriteString("task")
	if task.Automatic {
		b.WriteString(" automatic")
	}
	b.WriteString(" ")
	b.WriteString(task.Name)
	if len(task.Params) > 0 {
		b.WriteString(" (")
		for i, p := range task.Params {
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

func buildPropertyScope(prop *ast.PropertyDecl, diags *[]SemanticDiagnostic) *PropertyScope {
	sig := formatPropertySignature(prop)
	propSym := &Symbol{
		Name:           prop.Name,
		Kind:           KindProperty,
		TypeDetail:     prop.Kind,
		Signature:      sig,
		Range:          prop.Range,
		SelectionRange: prop.NameRange,
	}

	ps := &PropertyScope{
		Decl:    prop,
		Symbol:  propSym,
		Symbols: make(map[string]*Symbol),
	}

	addLocal := func(sym *Symbol) {
		if sym.Name == "" {
			return
		}
		if _, exists := ps.Symbols[sym.Name]; exists {
			if sym.SelectionRange.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		ps.Symbols[sym.Name] = sym
		ps.Order = append(ps.Order, sym)
		if sym.Range.File == "" {
			propSym.Children = append(propSym.Children, sym)
		}
	}

	for _, p := range prop.Params {
		pDetail := formatPortDeclDetail(p)
		addLocal(&Symbol{
			Name:           p.Name,
			Kind:           KindPort,
			TypeDetail:     pDetail,
			Signature:      pDetail + " " + p.Name,
			Range:          p.Range,
			SelectionRange: p.NameRange,
		})
	}

	for _, item := range prop.Locals {
		if d, ok := item.(*ast.NetDecl); ok {
			nDetail := formatNetDeclDetail(d)
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindNet,
				TypeDetail:     nDetail,
				Signature:      nDetail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
		}
	}

	return ps
}

func formatPropertySignature(prop *ast.PropertyDecl) string {
	var b strings.Builder
	b.WriteString(prop.Kind)
	b.WriteString(" ")
	b.WriteString(prop.Name)
	if len(prop.Params) > 0 {
		b.WriteString(" (")
		for i, p := range prop.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			if p.NetType != "" {
				b.WriteString(p.NetType)
				b.WriteString(" ")
			}
			b.WriteString(p.Name)
		}
		b.WriteString(")")
	}
	return b.String()
}

func buildCovergroupScope(cg *ast.CovergroupDecl, diags *[]SemanticDiagnostic) *CovergroupScope {
	cgSym := &Symbol{
		Name:           cg.Name,
		Kind:           KindCovergroup,
		TypeDetail:     "covergroup",
		Signature:      "covergroup " + cg.Name,
		Range:          cg.Range,
		SelectionRange: cg.NameRange,
	}

	cs := &CovergroupScope{
		Decl:    cg,
		Symbol:  cgSym,
		Symbols: make(map[string]*Symbol),
	}

	addLocal := func(sym *Symbol, isField bool) {
		if sym.Name == "" {
			return
		}
		if _, exists := cs.Symbols[sym.Name]; exists {
			if sym.SelectionRange.File == "" && isField {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		cs.Symbols[sym.Name] = sym
		cs.Order = append(cs.Order, sym)
		if isField {
			cgSym.Fields = append(cgSym.Fields, sym)
			if sym.Range.File == "" {
				cgSym.Children = append(cgSym.Children, sym)
			}
		}
	}

	for _, p := range cg.Params {
		pDetail := formatPortDeclDetail(p)
		addLocal(&Symbol{
			Name:           p.Name,
			Kind:           KindPort,
			TypeDetail:     pDetail,
			Signature:      pDetail + " " + p.Name,
			Range:          p.Range,
			SelectionRange: p.NameRange,
		}, false)
	}

	for _, pt := range cg.Points {
		addLocal(&Symbol{
			Name:           pt.Name,
			Kind:           KindField,
			TypeDetail:     pt.Kind,
			Signature:      pt.Kind + " " + pt.Name,
			Range:          pt.Range,
			SelectionRange: pt.NameRange,
		}, true)
	}

	// Built-in covergroup methods and option structures for member resolution (`cg_inst.sample()`, `cg_inst.get_coverage()`, etc.).
	for _, bm := range []struct {
		name string
		kind SymbolKind
		sig  string
	}{
		{"sample", KindFunction, "function void sample()"},
		{"get_coverage", KindFunction, "function real get_coverage()"},
		{"get_inst_coverage", KindFunction, "function real get_inst_coverage()"},
		{"set_inst_name", KindFunction, "function void set_inst_name(string s)"},
		{"start", KindFunction, "function void start()"},
		{"stop", KindFunction, "function void stop()"},
		{"option", KindField, "option"},
		{"type_option", KindField, "type_option"},
	} {
		cgSym.Fields = append(cgSym.Fields, &Symbol{
			Name:           bm.name,
			Kind:           bm.kind,
			TypeDetail:     "covergroup " + string(bm.kind),
			Signature:      bm.sig,
			Range:          cg.NameRange,
			SelectionRange: cg.NameRange,
		})
	}

	return cs
}

func buildBlockScope(block *ast.ProceduralBlock, diags *[]SemanticDiagnostic) *BlockScope {
	bs := &BlockScope{
		Block:   block,
		Symbols: make(map[string]*Symbol),
	}

	addLocal := func(sym *Symbol) {
		if sym.Name == "" {
			return
		}
		if _, exists := bs.Symbols[sym.Name]; exists {
			if sym.SelectionRange.File == "" {
				*diags = append(*diags, SemanticDiagnostic{
					Range:   sym.SelectionRange,
					Message: fmt.Sprintf("duplicate declaration of %q", sym.Name),
					IsError: false,
				})
			}
			return
		}
		bs.Symbols[sym.Name] = sym
		bs.Order = append(bs.Order, sym)
	}

	for _, item := range block.Locals {
		switch d := item.(type) {
		case *ast.ParamDecl:
			pDetail := formatParamDetail(d)
			s := pDetail + " " + d.Name
			if d.ValueText != "" {
				s += " = " + d.ValueText
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindParam,
				TypeDetail:     pDetail,
				Signature:      s,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
		case *ast.TypedefDecl:
			tdDetail := "typedef"
			tdSig := "typedef " + d.Name
			if d.BaseType != "" {
				tdDetail = "typedef " + d.BaseType
				tdSig = "typedef " + d.BaseType + " " + d.Name
			}
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindType,
				TypeDetail:     tdDetail,
				Signature:      tdSig,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
		case *ast.NetDecl:
			nDetail := formatNetDeclDetail(d)
			addLocal(&Symbol{
				Name:           d.Name,
				Kind:           KindNet,
				TypeDetail:     nDetail,
				Signature:      nDetail + " " + d.Name,
				Range:          d.Range,
				SelectionRange: d.NameRange,
			})
			for _, em := range d.EnumMembers {
				emSig := "enum " + em.Name
				if em.ValueText != "" {
					emSig += " = " + em.ValueText
				}
				addLocal(&Symbol{
					Name:           em.Name,
					Kind:           KindParam,
					TypeDetail:     "enum",
					Signature:      emSig,
					Range:          em.Range,
					SelectionRange: em.NameRange,
				})
			}
		}
	}

	return bs
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

// FindModuleInstantiations returns the ModuleNameRange of every instantiation (or interface port / bind usage) of moduleName in this file.
func (a *Analysis) FindModuleInstantiations(moduleName string) []ast.Range {
	if a == nil || moduleName == "" {
		return nil
	}
	var ranges []ast.Range
	if a.File != nil {
		for _, b := range a.File.Binds {
			if b.Inst != nil && b.Inst.ModuleName == moduleName {
				ranges = append(ranges, b.Inst.ModuleNameRange)
			}
		}
	}
	for _, scope := range a.ModuleOrder {
		if scope.Module == nil {
			continue
		}
		for _, port := range scope.Module.Ports {
			if port.InterfaceName == moduleName {
				ranges = append(ranges, port.InterfaceNameRange)
			}
		}
		collectModuleInstantiations(scope.Module.Items, moduleName, &ranges)
	}
	for _, scope := range a.InterfaceOrder {
		if scope.Interface == nil {
			continue
		}
		collectModuleInstantiations(scope.Interface.Items, moduleName, &ranges)
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
		case *ast.BindStmt:
			if d.Inst != nil && d.Inst.ModuleName == moduleName {
				*out = append(*out, d.Inst.ModuleNameRange)
			}
		case *ast.GenerateBlock:
			collectModuleInstantiations(d.Items, moduleName, out)
		}
	}
}
