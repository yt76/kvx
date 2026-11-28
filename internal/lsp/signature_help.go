package lsp

import (
	"strings"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
	"kvx/internal/lexer"
)

type callContext struct {
	openParen   int
	activeParam int
	namedArg    string
	isParamList bool   // true for `#(...)`
	instMod     string // non-empty for module/interface/checker instantiation ports or `#(...)`
	pkgName     string // non-empty for `pkg::fn(...)`
	baseChain   string // non-empty for `obj.method(...)` or `a.b.method(...)`
	calleeName  string // function/task/let/method/system-task name
}

var builtinSystemSignatures = map[string]string{
	"$display":       "$display(format, ...)",
	"$write":         "$write(format, ...)",
	"$strobe":        "$strobe(format, ...)",
	"$monitor":       "$monitor(format, ...)",
	"$sformatf":      "function string $sformatf(string format, ...)",
	"$sformat":       "task $sformat(output string str, string format, ...)",
	"$fdisplay":      "task $fdisplay(int fd, format, ...)",
	"$fwrite":        "task $fwrite(int fd, format, ...)",
	"$fopen":         "function int $fopen(string filename, [string type])",
	"$fclose":        "task $fclose(int fd)",
	"$readmemh":      "task $readmemh(string filename, memory, [int start_addr], [int finish_addr])",
	"$readmemb":      "task $readmemb(string filename, memory, [int start_addr], [int finish_addr])",
	"$clog2":         "function int $clog2(arg)",
	"$bits":          "function int $bits(expression_or_type)",
	"$size":          "function int $size(array, [int dimension])",
	"$left":          "function int $left(array, [int dimension])",
	"$right":         "function int $right(array, [int dimension])",
	"$low":           "function int $low(array, [int dimension])",
	"$high":          "function int $high(array, [int dimension])",
	"$signed":        "function signed $signed(expr)",
	"$unsigned":      "function unsigned $unsigned(expr)",
	"$cast":          "function int $cast(dest_var, source_exp)",
	"$urandom":       "function int unsigned $urandom([int seed])",
	"$urandom_range": "function int unsigned $urandom_range(int unsigned maxval, [int unsigned minval])",
	"$finish":        "task $finish([int n])",
	"$stop":          "task $stop([int n])",
	"$fatal":         "task $fatal([int finish_number], [format], ...)",
	"$error":         "task $error([format], ...)",
	"$warning":       "task $warning([format], ...)",
	"$info":          "task $info([format], ...)",
}

func computeSignatureHelp(doc *Document, store *DocumentStore, pos Position) *SignatureHelp {
	if doc == nil || doc.Analysis == nil {
		return nil
	}
	offset, err := positionToByteOffset(doc.Text, pos)
	if err != nil {
		return nil
	}

	ctx, ok := detectCallContext(doc.Text, offset)
	if !ok {
		return nil
	}

	astPos := ast.Position{Line: pos.Line, Column: pos.Character}
	extLookup := func(modName string) *analyzer.ModuleScope {
		if store == nil {
			return nil
		}
		if scope, found := store.FindModuleScope(modName); found {
			return scope
		}
		return nil
	}

	// 1. Module / interface / checker / class parameter list `#(...)` or port list `u_inst(...)`
	if ctx.instMod != "" {
		if ctx.isParamList {
			if sig := buildInstParamSignature(doc.Analysis, store, ctx.instMod); sig != "" {
				return buildSignatureFromLabel(sig, ctx.activeParam, ctx.namedArg)
			}
		} else {
			if sig := buildInstPortSignature(doc.Analysis, store, ctx.instMod); sig != "" {
				return buildSignatureFromLabel(sig, ctx.activeParam, ctx.namedArg)
			}
		}
	}

	// 2. Built-in system task/function (`$display(...)`, `$clog2(...)`, etc.)
	if strings.HasPrefix(ctx.calleeName, "$") {
		if sig, exists := builtinSystemSignatures[ctx.calleeName]; exists {
			return buildSignatureFromLabel(sig, ctx.activeParam, ctx.namedArg)
		}
		return nil
	}

	// 3. Member method call (`obj.method(...)`, `q.insert(...)`, `b1.item.print(...)`)
	if ctx.baseChain != "" && ctx.calleeName != "" {
		for _, sym := range doc.Analysis.MemberCompletions(astPos, ctx.baseChain, extLookup) {
			if sym.Name == ctx.calleeName && (sym.Kind == analyzer.KindFunction || sym.Kind == analyzer.KindTask) {
				return buildSignatureFromLabel(sym.Signature, ctx.activeParam, ctx.namedArg)
			}
		}
		return nil
	}

	// 4. Package-scoped call (`pkg::fn(...)`)
	if ctx.pkgName != "" && ctx.calleeName != "" {
		if pkgScope, found := doc.Analysis.AllPackages[ctx.pkgName]; found {
			if sym, exists := pkgScope.Symbols[ctx.calleeName]; exists &&
				(sym.Kind == analyzer.KindFunction || sym.Kind == analyzer.KindTask) {
				return buildSignatureFromLabel(sym.Signature, ctx.activeParam, ctx.namedArg)
			}
		}
		return nil
	}

	// 5. Plain function / task / `let` call in enclosing scope or global symbols
	if ctx.calleeName != "" {
		if sym := lookupCallableInScope(doc.Analysis, astPos, ctx.calleeName); sym != nil {
			return buildSignatureFromLabel(sym.Signature, ctx.activeParam, ctx.namedArg)
		}
	}

	return nil
}

func lookupCallableInScope(an *analyzer.Analysis, pos ast.Position, name string) *analyzer.Symbol {
	checkMap := func(syms map[string]*analyzer.Symbol) *analyzer.Symbol {
		if sym, ok := syms[name]; ok && sym != nil &&
			(sym.Kind == analyzer.KindFunction || sym.Kind == analyzer.KindTask) {
			return sym
		}
		return nil
	}

	if clsScope := an.ClassAt(pos); clsScope != nil {
		if sym := checkMap(clsScope.Symbols); sym != nil {
			return sym
		}
	}
	if modScope := an.ModuleAt(pos); modScope != nil {
		if sym := checkMap(modScope.Symbols); sym != nil {
			return sym
		}
	}
	if ifaceScope := an.InterfaceAt(pos); ifaceScope != nil {
		if sym := checkMap(ifaceScope.Symbols); sym != nil {
			return sym
		}
	}
	if sym := checkMap(an.GlobalSymbols); sym != nil {
		return sym
	}
	return nil
}

func buildInstPortSignature(an *analyzer.Analysis, store *DocumentStore, modName string) string {
	if modScope, ok := an.AllModules[modName]; ok && modScope != nil {
		return modScope.Symbol.Signature
	}
	if ifaceScope, ok := an.AllInterfaces[modName]; ok && ifaceScope != nil {
		return formatPortsSignature("interface "+modName, ifaceScope.Interface.Ports)
	}
	if store != nil {
		if modScope, ok := store.FindModuleScope(modName); ok && modScope != nil {
			return modScope.Symbol.Signature
		}
		if ifaceScope, ok := store.FindInterfaceScope(modName); ok && ifaceScope != nil {
			return formatPortsSignature("interface "+modName, ifaceScope.Interface.Ports)
		}
	}
	return ""
}

func buildInstParamSignature(an *analyzer.Analysis, store *DocumentStore, name string) string {
	if modScope, ok := an.AllModules[name]; ok && modScope != nil && len(modScope.Module.Params) > 0 {
		return formatParamsSignature(name, modScope.Module.Params)
	}
	if ifaceScope, ok := an.AllInterfaces[name]; ok && ifaceScope != nil && len(ifaceScope.Interface.Params) > 0 {
		return formatParamsSignature(name, ifaceScope.Interface.Params)
	}
	if clsScope, ok := an.AllClasses[name]; ok && clsScope != nil && len(clsScope.Class.Params) > 0 {
		return formatParamsSignature(name, clsScope.Class.Params)
	}
	if store != nil {
		if modScope, ok := store.FindModuleScope(name); ok && modScope != nil && len(modScope.Module.Params) > 0 {
			return formatParamsSignature(name, modScope.Module.Params)
		}
		if ifaceScope, ok := store.FindInterfaceScope(name); ok && ifaceScope != nil && len(ifaceScope.Interface.Params) > 0 {
			return formatParamsSignature(name, ifaceScope.Interface.Params)
		}
	}
	return ""
}

func formatParamsSignature(owner string, params []*ast.ParamDecl) string {
	var b strings.Builder
	b.WriteString(owner)
	b.WriteString(" #(")
	for i, p := range params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Kind != "" {
			b.WriteString(p.Kind)
			b.WriteString(" ")
		}
		if p.Width != nil {
			b.WriteString(p.Width.Raw)
			b.WriteString(" ")
		}
		b.WriteString(p.Name)
		if p.ValueText != "" {
			b.WriteString(" = ")
			b.WriteString(p.ValueText)
		}
	}
	b.WriteString(")")
	return b.String()
}

func formatPortsSignature(prefix string, ports []*ast.PortDecl) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(" (")
	for i, p := range ports {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Direction != "" {
			b.WriteString(p.Direction)
			b.WriteString(" ")
		}
		if p.NetType != "" {
			b.WriteString(p.NetType)
			b.WriteString(" ")
		}
		if p.Width != nil {
			b.WriteString(p.Width.Raw)
			b.WriteString(" ")
		}
		b.WriteString(p.Name)
	}
	b.WriteString(")")
	return b.String()
}

// detectCallContext scans backwards from offset to find the active call or instantiation `(`
// and determines the callee, active parameter index, and optional named port/parameter `.name(...)`.
func detectCallContext(text string, offset int) (callContext, bool) {
	if offset < 0 || offset > len(text) {
		return callContext{}, false
	}

	openParen, activeParam, namedArg, ok := findEnclosingParen(text, offset)
	if !ok {
		return callContext{}, false
	}

	// If the cursor is inside a named port/param value like `.din(expr|)`,
	// step out to the enclosing instantiation `(` while preserving namedArg = "din".
	if isNamedConnectionParen(text, openParen) {
		connName := extractNamedConnectionName(text, openParen)
		outerParen, outerParam, _, outerOK := findEnclosingParen(text, openParen)
		if outerOK {
			openParen = outerParen
			activeParam = outerParam
			namedArg = connName
		}
	}

	ctx := callContext{
		openParen:   openParen,
		activeParam: activeParam,
		namedArg:    namedArg,
	}

	// Check if this is a module/interface/checker instantiation port list or `#(...)` parameter override.
	if modName, inParams := inferEnclosingInstantiation(text, openParen+1); modName != "" {
		ctx.instMod = modName
		ctx.isParamList = inParams
		return ctx, true
	}

	// Otherwise extract the callee right before `(`.
	j := openParen
	for j > 0 && isSpaceByte(text[j-1]) {
		j--
	}
	endName := j
	for j > 0 && (isIdentByte(text[j-1]) || text[j-1] == '$') {
		j--
	}
	callee := text[j:endName]
	if callee == "" {
		return callContext{}, false
	}
	// Ignore control-flow keywords like `if (`, `case (`, `for (`, `while (`, `repeat (`.
	if !strings.HasPrefix(callee, "$") && lexer.LookupIdent(callee) != lexer.TokenIdent {
		return callContext{}, false
	}
	ctx.calleeName = callee

	// Check for `pkg::fn(` or `base.method(`
	k := j
	for k > 0 && isSpaceByte(text[k-1]) {
		k--
	}
	if k >= 2 && text[k-2] == ':' && text[k-1] == ':' {
		k -= 2
		for k > 0 && isSpaceByte(text[k-1]) {
			k--
		}
		endPkg := k
		for k > 0 && isIdentByte(text[k-1]) {
			k--
		}
		if k < endPkg {
			ctx.pkgName = text[k:endPkg]
		}
	} else if k >= 1 && text[k-1] == '.' {
		k--
		for k > 0 && isSpaceByte(text[k-1]) {
			k--
		}
		// Extract dotted base chain (stripping `[...]` index slices)
		endBase := k
		for k > 0 {
			if isIdentByte(text[k-1]) {
				k--
				continue
			}
			if text[k-1] == ']' {
				depth := 1
				k--
				for k > 0 && depth > 0 {
					k--
					if text[k] == ']' {
						depth++
					} else if text[k] == '[' {
						depth--
					}
				}
				continue
			}
			if text[k-1] == '.' && k-1 > 0 && (isIdentByte(text[k-2]) || text[k-2] == ']') {
				k--
				continue
			}
			break
		}
		if k < endBase {
			ctx.baseChain = stripBrackets(text[k:endBase])
		}
	}

	return ctx, true
}

func findEnclosingParen(text string, offset int) (openParen int, activeParam int, namedArg string, ok bool) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inString := false
	lastArgStart := offset

	for i := offset - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '"' && (i == 0 || text[i-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case ')':
			parenDepth++
		case '(':
			if parenDepth == 0 {
				argSeg := strings.TrimSpace(text[i+1 : lastArgStart])
				if activeParam == 0 {
					argSeg = strings.TrimSpace(text[i+1 : offset])
				}
				return i, activeParam, extractLeadingNamedArg(argSeg), true
			}
			parenDepth--
		case ']':
			bracketDepth++
		case '[':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '}':
			braceDepth++
		case '{':
			if braceDepth > 0 {
				braceDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				if activeParam == 0 {
					namedArg = extractLeadingNamedArg(strings.TrimSpace(text[i+1 : offset]))
				}
				activeParam++
				lastArgStart = i
			}
		case ';':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return -1, 0, "", false
			}
		}
	}
	return -1, 0, "", false
}

func isNamedConnectionParen(text string, openParen int) bool {
	return extractNamedConnectionName(text, openParen) != ""
}

func extractNamedConnectionName(text string, openParen int) string {
	j := openParen
	for j > 0 && isSpaceByte(text[j-1]) {
		j--
	}
	end := j
	for j > 0 && isIdentByte(text[j-1]) {
		j--
	}
	if j == end {
		return ""
	}
	name := text[j:end]
	for j > 0 && isSpaceByte(text[j-1]) {
		j--
	}
	if j > 0 && text[j-1] == '.' {
		// Ensure it's a standalone `.name(` (preceded by `(`, `,`, or whitespace), not `obj.method(`.
		k := j - 1
		for k > 0 && isSpaceByte(text[k-1]) {
			k--
		}
		if k == 0 || text[k-1] == '(' || text[k-1] == ',' {
			return name
		}
	}
	return ""
}

func extractLeadingNamedArg(seg string) string {
	if !strings.HasPrefix(seg, ".") {
		return ""
	}
	i := 1
	for i < len(seg) && isIdentByte(seg[i]) {
		i++
	}
	if i > 1 {
		return seg[1:i]
	}
	return ""
}

func stripBrackets(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}

func isSpaceByte(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func buildSignatureFromLabel(label string, activeParam int, namedArg string) *SignatureHelp {
	if label == "" {
		return nil
	}

	var params []ParameterInformation
	if lp := strings.IndexByte(label, '('); lp >= 0 {
		if rp := strings.LastIndexByte(label, ')'); rp > lp {
			inner := strings.TrimSpace(label[lp+1 : rp])
			if inner != "" {
				for _, part := range splitTopLevelCommas(inner) {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						params = append(params, ParameterInformation{Label: trimmed})
					}
				}
			}
		}
	}

	if namedArg != "" {
		for idx, p := range params {
			if paramLabelMatchesName(p.Label, namedArg) {
				activeParam = idx
				break
			}
		}
	}

	if len(params) > 0 && activeParam >= len(params) {
		if strings.Contains(params[len(params)-1].Label, "...") {
			activeParam = len(params) - 1
		} else {
			activeParam = len(params) - 1
		}
	}

	return &SignatureHelp{
		Signatures: []SignatureInformation{{
			Label:      label,
			Parameters: params,
		}},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}
}

func splitTopLevelCommas(s string) []string {
	var out []string
	paren, bracket, brace := 0, 0, 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			paren++
		case ')':
			if paren > 0 {
				paren--
			}
		case '[':
			bracket++
		case ']':
			if bracket > 0 {
				bracket--
			}
		case '{':
			brace++
		case '}':
			if brace > 0 {
				brace--
			}
		case ',':
			if paren == 0 && bracket == 0 && brace == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

func paramLabelMatchesName(paramLabel, targetName string) bool {
	// Strip optional default value `= ...` and optional `[...]` brackets, then check the last identifier token.
	s := paramLabel
	if eq := strings.IndexByte(s, '='); eq >= 0 {
		s = strings.TrimSpace(s[:eq])
	}
	s = strings.TrimRight(s, "[]")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	last := strings.Trim(fields[len(fields)-1], "[]")
	return last == targetName
}
