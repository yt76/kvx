package ast

// Position represents a 0-based line, column, and byte offset in a source file.
type Position struct {
	Line   int
	Column int
	Offset int
}

// Range represents a contiguous span of source text between Start and End.
// File is empty for the root file being parsed, or holds the file path when
// the node originated from an `include file.
type Range struct {
	File  string
	Start Position
	End   Position
}

// Contains returns true if pos (in the root file) falls within r (inclusive of Start and End).
func (r Range) Contains(pos Position) bool {
	if r.File != "" {
		return false
	}
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Column < r.Start.Column {
		return false
	}
	if pos.Line == r.End.Line && pos.Column > r.End.Column {
		return false
	}
	return true
}

// SourceFile represents a parsed Verilog source file.
type SourceFile struct {
	Modules    []*ModuleDecl
	Interfaces []*InterfaceDecl
	Packages   []*PackageDecl
	Imports    []*ImportDecl
	Includes   []*IncludeDecl
	Macros     []*MacroDecl
	Params     []*ParamDecl
	Typedefs   []*TypedefDecl
	Refs       []IdentifierRef
}

// InterfaceDecl represents a SystemVerilog interface declaration (interface ... endinterface).
type InterfaceDecl struct {
	Name      string
	NameRange Range
	Range     Range
	Imports   []*ImportDecl
	Params    []*ParamDecl
	Ports     []*PortDecl
	Modports  []*ModportDecl
	Items     []ModuleItem
	Refs      []IdentifierRef
}

// ModportDecl represents a modport declaration inside a SystemVerilog interface.
type ModportDecl struct {
	Name      string
	NameRange Range
	Ports     []ModportPort
	Range     Range
}

func (m *ModportDecl) ItemRange() Range { return m.Range }
func (m *ModportDecl) moduleItemNode()  {}

// ModportPort represents a single signal or task/function entry inside a modport declaration.
type ModportPort struct {
	Direction string // "input", "output", "inout", "ref", "import", "export"
	Name      string
	NameRange Range
}

// PackageDecl represents a SystemVerilog package declaration (package ... endpackage).
type PackageDecl struct {
	Name      string
	NameRange Range
	Range     Range
	Params    []*ParamDecl
	Items     []ModuleItem
	Refs      []IdentifierRef
}

// ImportDecl represents a SystemVerilog package import (e.g. import pkg::*; or import pkg::SYM;).
type ImportDecl struct {
	PackageName      string
	PackageNameRange Range
	SymbolName       string // "*" for wildcard import, or specific symbol name
	SymbolNameRange  Range
	Range            Range
}

func (i *ImportDecl) ItemRange() Range { return i.Range }
func (i *ImportDecl) moduleItemNode()  {}

// IncludeDecl represents an `include "..." directive.
type IncludeDecl struct {
	Path         string
	ResolvedPath string
	Range        Range
}

// MacroDecl represents a `define macro declaration.
type MacroDecl struct {
	Name      string
	NameRange Range
	ValueText string
	Range     Range
}

// ModuleItem represents any declaration, statement, or block inside a module.
type ModuleItem interface {
	ItemRange() Range
	moduleItemNode()
}

// BitRange represents a packed or unpacked bit width specification such as [7:0] or [WIDTH-1:0].
type BitRange struct {
	MSB   string
	LSB   string
	Raw   string
	Range Range
}

// IdentifierRef records an identifier usage in an expression, assignment, sensitivity list, or port connection.
type IdentifierRef struct {
	Name  string
	Range Range
}

// ModuleDecl represents a Verilog module declaration (module ... endmodule).
type ModuleDecl struct {
	Name      string
	NameRange Range
	Range     Range
	Imports   []*ImportDecl
	Params    []*ParamDecl
	Ports     []*PortDecl
	Items     []ModuleItem
	Refs      []IdentifierRef
}

// PortDecl represents a module or interface port declaration (input, output, inout, ref, or interface port).
type PortDecl struct {
	Direction          string // "input", "output", "inout", "ref", or "interface"
	NetType            string // "wire", "reg", "logic", "axi_if.master", or ""
	InterfaceName      string // non-empty for typed interface ports (e.g. "axi_if")
	InterfaceNameRange Range
	ModportName        string // non-empty when a modport is specified (e.g. "master")
	ModportNameRange   Range
	Signed             bool
	Width              *BitRange
	Name               string
	NameRange          Range
	Range              Range
}

func (d *PortDecl) ItemRange() Range { return d.Range }
func (d *PortDecl) moduleItemNode()  {}

// EnumMember represents a named enumerator inside an enum declaration.
type EnumMember struct {
	Name      string
	NameRange Range
	ValueText string
	Range     Range
}

// TypedefDecl represents a SystemVerilog typedef declaration (including enum/struct/union).
type TypedefDecl struct {
	Name        string
	NameRange   Range
	BaseType    string
	EnumMembers []EnumMember
	Range       Range
	Refs        []IdentifierRef
}

func (t *TypedefDecl) ItemRange() Range { return t.Range }
func (t *TypedefDecl) moduleItemNode()  {}

// NetDecl represents a net or variable declaration (wire, reg, integer, logic, user-defined type, etc.).
type NetDecl struct {
	Kind        string // "wire", "reg", "integer", "logic", or user-defined type name
	Signed      bool
	Width       *BitRange
	Name        string
	NameRange   Range
	EnumMembers []EnumMember
	Range       Range
}

func (d *NetDecl) ItemRange() Range { return d.Range }
func (d *NetDecl) moduleItemNode()  {}

// ParamDecl represents a parameter or localparam declaration.
type ParamDecl struct {
	Kind      string // "parameter", "localparam"
	Signed    bool
	Width     *BitRange
	Name      string
	NameRange Range
	ValueText string
	Range     Range
}

func (d *ParamDecl) ItemRange() Range { return d.Range }
func (d *ParamDecl) moduleItemNode()  {}

// AssignStmt represents a continuous assignment (assign lhs = rhs;).
type AssignStmt struct {
	Range Range
	Refs  []IdentifierRef
}

func (s *AssignStmt) ItemRange() Range { return s.Range }
func (s *AssignStmt) moduleItemNode()  {}

// DefparamStmt represents a defparam statement (defparam u_sub.P = 16;).
type DefparamStmt struct {
	Range Range
	Refs  []IdentifierRef
}

func (s *DefparamStmt) ItemRange() Range { return s.Range }
func (s *DefparamStmt) moduleItemNode()  {}

// ProceduralBlock represents an always or initial block.
type ProceduralBlock struct {
	Kind   string // "always", "initial"
	Label  string
	Labels []string
	Locals []ModuleItem
	Range  Range
	Refs   []IdentifierRef
}

func (b *ProceduralBlock) ItemRange() Range { return b.Range }
func (b *ProceduralBlock) moduleItemNode()  {}

// ModuleInst represents a module instantiation (e.g., alu u_alu (.clk(clk));).
type ModuleInst struct {
	ModuleName        string
	ModuleNameRange   Range
	InstanceName      string
	InstanceNameRange Range
	Range             Range
	Refs              []IdentifierRef
}

func (m *ModuleInst) ItemRange() Range { return m.Range }
func (m *ModuleInst) moduleItemNode()  {}

// FunctionDecl represents a Verilog function declaration (function ... endfunction).
type FunctionDecl struct {
	Automatic  bool
	Signed     bool
	ReturnType string // "integer" or ""
	Width      *BitRange
	Name       string
	NameRange  Range
	Params     []*PortDecl
	Locals     []ModuleItem
	Range      Range
	Refs       []IdentifierRef
}

func (f *FunctionDecl) ItemRange() Range { return f.Range }
func (f *FunctionDecl) moduleItemNode()  {}

// TaskDecl represents a Verilog task declaration (task ... endtask).
type TaskDecl struct {
	Automatic bool
	Name      string
	NameRange Range
	Params    []*PortDecl
	Locals    []ModuleItem
	Range     Range
	Refs      []IdentifierRef
}

func (t *TaskDecl) ItemRange() Range { return t.Range }
func (t *TaskDecl) moduleItemNode()  {}

// GenerateBlock represents a generate construct (generate...endgenerate, for-generate, if-generate, case-generate, or generate begin...end).
type GenerateBlock struct {
	Kind  string // "generate", "for", "if", "case", "begin"
	Label string
	Items []ModuleItem
	Range Range
	Refs  []IdentifierRef
}

func (g *GenerateBlock) ItemRange() Range { return g.Range }
func (g *GenerateBlock) moduleItemNode()  {}
