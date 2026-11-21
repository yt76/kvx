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
	Modules  []*ModuleDecl
	Includes []*IncludeDecl
	Macros   []*MacroDecl
	Params   []*ParamDecl
	Refs     []IdentifierRef
}

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
	Params    []*ParamDecl
	Ports     []*PortDecl
	Items     []ModuleItem
	Refs      []IdentifierRef
}

// PortDecl represents a module port declaration (input, output, inout).
type PortDecl struct {
	Direction string // "input", "output", "inout"
	NetType   string // "wire", "reg", or ""
	Signed    bool
	Width     *BitRange
	Name      string
	NameRange Range
	Range     Range
}

func (d *PortDecl) ItemRange() Range { return d.Range }
func (d *PortDecl) moduleItemNode()  {}

// NetDecl represents a net or variable declaration (wire, reg, integer).
type NetDecl struct {
	Kind      string // "wire", "reg", "integer"
	Signed    bool
	Width     *BitRange
	Name      string
	NameRange Range
	Range     Range
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
