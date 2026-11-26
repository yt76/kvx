package lsp

import (
	"sort"
	"strings"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
	"kvx/internal/lexer"
)

type completionKind int

const (
	compDefault completionKind = iota
	compScope                  // `pkg::` or `pkg::prefix`
	compMember                 // `base.` or `base.prefix`
	compNamedPort              // `.` or `.prefix` inside module/interface instantiation
)

type completionContext struct {
	kind     completionKind
	baseName string // package name (compScope) or base ident (compMember)
	prefix   string // partial identifier already typed after `::` or `.`
	inParams bool   // true when `.` is inside `#(...)` parameter override list
	instMod  string // inferred module/interface name for compNamedPort
}

func computeCompletions(doc *Document, store *DocumentStore, pos Position) []CompletionItem {
	var items []CompletionItem
	seen := make(map[string]bool)

	addSym := func(sym *analyzer.Symbol) {
		if sym == nil || sym.Name == "" || seen[sym.Name] {
			return
		}
		seen[sym.Name] = true
		items = append(items, CompletionItem{
			Label:  sym.Name,
			Kind:   toCompletionKind(sym.Kind),
			Detail: sym.TypeDetail,
		})
	}

	if doc != nil && doc.Analysis != nil {
		astPos := ast.Position{Line: pos.Line, Column: pos.Character}
		ctx := detectCompletionContext(doc.Text, pos)

		switch ctx.kind {
		case compScope:
			for _, sym := range doc.Analysis.PackageCompletions(ctx.baseName) {
				if ctx.prefix == "" || strings.HasPrefix(sym.Name, ctx.prefix) {
					addSym(sym)
				}
			}
			return items

		case compMember:
			extLookup := func(modName string) *analyzer.ModuleScope {
				if store == nil {
					return nil
				}
				if scope, ok := store.FindModuleScope(modName); ok {
					return scope
				}
				return nil
			}
			for _, sym := range doc.Analysis.MemberCompletions(astPos, ctx.baseName, extLookup) {
				if ctx.prefix == "" || strings.HasPrefix(sym.Name, ctx.prefix) {
					addSym(sym)
				}
			}
			return items

		case compNamedPort:
			modName := ctx.instMod
			if modName == "" {
				if inst := doc.Analysis.InstantiationAt(astPos); inst != nil {
					modName = inst.ModuleName
				}
			}
			if modName != "" {
				wantKind := analyzer.KindPort
				if ctx.inParams {
					wantKind = analyzer.KindParam
				}
				var order []*analyzer.Symbol
				if scope, ok := doc.Analysis.Modules[modName]; ok {
					order = scope.Order
				} else if scope, ok := doc.Analysis.AllInterfaces[modName]; ok {
					order = scope.Order
				} else if store != nil {
					if scope, ok := store.FindModuleScope(modName); ok {
						order = scope.Order
					} else if scope, ok := store.FindInterfaceScope(modName); ok {
						order = scope.Order
					}
				}
				for _, sym := range order {
					if sym.Kind == wantKind && (ctx.prefix == "" || strings.HasPrefix(sym.Name, ctx.prefix)) {
						addSym(sym)
					}
				}
				return items
			}
		}

		// Default completion: symbols in scope at pos.
		for _, sym := range doc.Analysis.CompletionsAt(astPos) {
			addSym(sym)
		}
	}

	if store != nil {
		for _, sym := range store.WorkspaceModules() {
			addSym(sym)
		}
		for _, sym := range store.WorkspaceInterfaces() {
			addSym(sym)
		}
	}

	kwList := lexer.Keywords()
	sort.Strings(kwList)
	for _, kw := range kwList {
		if seen[kw] {
			continue
		}
		seen[kw] = true
		items = append(items, CompletionItem{
			Label:  kw,
			Kind:   CompletionKindKeyword,
			Detail: "keyword",
		})
	}

	return items
}

func detectCompletionContext(text string, pos Position) completionContext {
	offset, err := positionToByteOffset(text, pos)
	if err != nil || offset <= 0 {
		return completionContext{kind: compDefault}
	}

	// Scan backwards over partial identifier prefix right before cursor.
	i := offset
	for i > 0 && isIdentByte(text[i-1]) {
		i--
	}
	prefix := text[i:offset]

	// 1. Check for `pkg::` scope resolution completion.
	if i >= 2 && text[i-1] == ':' && text[i-2] == ':' {
		j := i - 2
		for j > 0 && (text[j-1] == ' ' || text[j-1] == '\t') {
			j--
		}
		endBase := j
		for j > 0 && isIdentByte(text[j-1]) {
			j--
		}
		if j < endBase {
			return completionContext{
				kind:     compScope,
				baseName: text[j:endBase],
				prefix:   prefix,
			}
		}
	}

	// 2. Check for `.` completion (either `base.` member access or standalone `.` named port).
	if i >= 1 && text[i-1] == '.' {
		j := i - 1
		// Skip optional `[...]` index right before `.` (e.g. `arr[0].`).
		for j > 0 && text[j-1] == ']' {
			depth := 1
			j--
			for j > 0 && depth > 0 {
				j--
				if text[j] == ']' {
					depth++
				} else if text[j] == '[' {
					depth--
				}
			}
		}
		endBase := j
		for j > 0 {
			if isIdentByte(text[j-1]) {
				j--
				continue
			}
			// Allow chained member access such as `b1.item.` or `arr[0].item.`
			if text[j-1] == '.' && j-1 > 0 {
				k := j - 1
				if k > 0 && text[k-1] == ']' {
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
				}
				if k > 0 && isIdentByte(text[k-1]) {
					j = k
					continue
				}
			}
			break
		}
		if j < endBase {
			// Strip any `[...]` index slices from intermediate segments so `arr[0].item` becomes `arr.item`.
			rawChain := text[j:endBase]
			var cleanChain strings.Builder
			bDepth := 0
			for ci := 0; ci < len(rawChain); ci++ {
				ch := rawChain[ci]
				if ch == '[' {
					bDepth++
					continue
				}
				if ch == ']' {
					if bDepth > 0 {
						bDepth--
					}
					continue
				}
				if bDepth == 0 {
					cleanChain.WriteByte(ch)
				}
			}
			return completionContext{
				kind:     compMember,
				baseName: cleanChain.String(),
				prefix:   prefix,
			}
		}

		// Standalone `.` (not preceded by an identifier): check if inside an instantiation `(...)` or `#(...)`.
		modName, inParams := inferEnclosingInstantiation(text, i-1)
		if modName != "" {
			return completionContext{
				kind:     compNamedPort,
				prefix:   prefix,
				inParams: inParams,
				instMod:  modName,
			}
		}
	}

	return completionContext{kind: compDefault}
}

// inferEnclosingInstantiation scans backwards from dotOffset to find the enclosing
// `mod_name [#(...)] inst_name (` or `mod_name #(` header, even if the instantiation is incomplete.
func inferEnclosingInstantiation(text string, dotOffset int) (string, bool) {
	depth := 0
	openParen := -1
	for i := dotOffset - 1; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				openParen = i
				goto foundParen
			}
			depth--
		case ';':
			return "", false
		}
	}
	return "", false

foundParen:
	// Collect identifiers / `#` before openParen back to the previous statement boundary.
	j := openParen
	skipSpaces := func() {
		for j > 0 && (text[j-1] == ' ' || text[j-1] == '\t' || text[j-1] == '\r' || text[j-1] == '\n') {
			j--
		}
	}
	readIdentBack := func() string {
		end := j
		for j > 0 && isIdentByte(text[j-1]) {
			j--
		}
		return text[j:end]
	}

	skipSpaces()
	if j > 0 && text[j-1] == '#' {
		// Parameter override list: `mod_name #(.`
		j--
		skipSpaces()
		modName := readIdentBack()
		if modName != "" && lexer.LookupIdent(modName) == lexer.TokenIdent {
			return modName, true
		}
		return "", false
	}

	// Port connection list: `mod_name [#(...)] inst_name [range] (.`
	// Skip optional instance array range `[...]`
	if j > 0 && text[j-1] == ']' {
		bdepth := 1
		j--
		for j > 0 && bdepth > 0 {
			j--
			if text[j] == ']' {
				bdepth++
			} else if text[j] == '[' {
				bdepth--
			}
		}
		skipSpaces()
	}

	instName := readIdentBack()
	if instName == "" || lexer.LookupIdent(instName) != lexer.TokenIdent {
		return "", false
	}

	skipSpaces()
	// Skip optional `#(...)` parameter override list before instance name.
	if j > 0 && text[j-1] == ')' {
		pdepth := 1
		j--
		for j > 0 && pdepth > 0 {
			j--
			if text[j] == ')' {
				pdepth++
			} else if text[j] == '(' {
				pdepth--
			}
		}
		skipSpaces()
		if j > 0 && text[j-1] == '#' {
			j--
			skipSpaces()
		}
	}

	modName := readIdentBack()
	if modName != "" && lexer.LookupIdent(modName) == lexer.TokenIdent {
		return modName, false
	}
	return "", false
}

func isIdentByte(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}
