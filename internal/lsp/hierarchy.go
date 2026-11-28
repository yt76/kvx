package lsp

import (
	"sort"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
)

func prepareCallHierarchy(doc *Document, store *DocumentStore, pos Position) []CallHierarchyItem {
	if doc == nil || doc.Analysis == nil {
		return nil
	}
	astPos := ast.Position{Line: pos.Line, Column: pos.Character}
	sym := doc.Analysis.SymbolAt(astPos)
	if sym == nil {
		return nil
	}

	targetName := ""
	switch sym.Kind {
	case analyzer.KindModule, analyzer.KindInterface:
		targetName = sym.Name
	case analyzer.KindInstance:
		targetName = sym.TargetModule
	default:
		return nil
	}
	if targetName == "" {
		return nil
	}

	item, ok := resolveHierarchyItem(doc, store, targetName)
	if !ok {
		return nil
	}
	return []CallHierarchyItem{item}
}

func resolveHierarchyItem(doc *Document, store *DocumentStore, name string) (CallHierarchyItem, bool) {
	if doc != nil && doc.Analysis != nil {
		if scope, ok := doc.Analysis.Modules[name]; ok && scope != nil && scope.Symbol != nil {
			return symbolToHierarchyItem(doc.URI, scope.Symbol), true
		}
		if scope, ok := doc.Analysis.Interfaces[name]; ok && scope != nil && scope.Symbol != nil {
			return symbolToHierarchyItem(doc.URI, scope.Symbol), true
		}
	}
	if store != nil {
		if uri, sym, ok := store.FindModule(name); ok && sym != nil {
			return symbolToHierarchyItem(uri, sym), true
		}
		if uri, sym, ok := store.FindInterface(name); ok && sym != nil {
			return symbolToHierarchyItem(uri, sym), true
		}
	}
	return CallHierarchyItem{}, false
}

func symbolToHierarchyItem(uri string, sym *analyzer.Symbol) CallHierarchyItem {
	kind := LSPSymbolKindModule
	if sym.Kind == analyzer.KindInterface {
		kind = LSPSymbolKindInterface
	}
	if sym.SelectionRange.File != "" {
		uri = filePathToURI(sym.SelectionRange.File)
	}
	return CallHierarchyItem{
		Name:           sym.Name,
		Kind:           kind,
		Detail:         sym.TypeDetail,
		URI:            uri,
		Range:          toLSPRange(sym.Range),
		SelectionRange: toLSPRange(sym.SelectionRange),
	}
}

// allEffectiveDocsLocked returns all open documents plus non-open indexed workspace documents.
func (s *DocumentStore) allEffectiveDocsLocked() []*Document {
	docs := make([]*Document, 0, len(s.docs)+len(s.workspaceDocs))
	for _, doc := range s.docs {
		docs = append(docs, doc)
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		docs = append(docs, doc)
	}
	return docs
}

func collectInstsFromItems(items []ast.ModuleItem, out *[]*ast.ModuleInst) {
	for _, item := range items {
		switch d := item.(type) {
		case *ast.ModuleInst:
			*out = append(*out, d)
		case *ast.GenerateBlock:
			collectInstsFromItems(d.Items, out)
		case *ast.BindStmt:
			if d.Inst != nil {
				*out = append(*out, d.Inst)
			}
		}
	}
}

func incomingHierarchyCalls(store *DocumentStore, item CallHierarchyItem) []CallHierarchyIncomingCall {
	if store == nil || item.Name == "" {
		return []CallHierarchyIncomingCall{}
	}

	store.mu.RLock()
	docs := store.allEffectiveDocsLocked()
	store.mu.RUnlock()

	type parentKey struct {
		uri  string
		name string
	}
	grouped := make(map[parentKey]*CallHierarchyIncomingCall)
	addCall := func(parent CallHierarchyItem, callRange Range) {
		key := parentKey{uri: parent.URI, name: parent.Name}
		entry, exists := grouped[key]
		if !exists {
			entry = &CallHierarchyIncomingCall{From: parent}
			grouped[key] = entry
		}
		entry.FromRanges = append(entry.FromRanges, callRange)
	}

	for _, doc := range docs {
		if doc.Analysis == nil {
			continue
		}
		for _, modScope := range doc.Analysis.ModuleOrder {
			if modScope.Module == nil || modScope.Symbol == nil {
				continue
			}
			var insts []*ast.ModuleInst
			collectInstsFromItems(modScope.Module.Items, &insts)
			for _, inst := range insts {
				if inst.ModuleName == item.Name {
					addCall(symbolToHierarchyItem(doc.URI, modScope.Symbol), toLSPRange(inst.ModuleNameRange))
				}
			}
		}
		// Top-level `bind target_mod child_mod u_chk (...);`
		if doc.Analysis.File != nil {
			for _, b := range doc.Analysis.File.Binds {
				if b.Inst != nil && b.Inst.ModuleName == item.Name && !b.IsHierarchical && b.TargetName != "" {
					if parentItem, ok := resolveHierarchyItem(doc, store, b.TargetName); ok {
						addCall(parentItem, toLSPRange(b.Inst.ModuleNameRange))
					}
				}
			}
		}
	}

	out := make([]CallHierarchyIncomingCall, 0, len(grouped))
	for _, entry := range grouped {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From.Name != out[j].From.Name {
			return out[i].From.Name < out[j].From.Name
		}
		return out[i].From.URI < out[j].From.URI
	})
	return out
}

func outgoingHierarchyCalls(store *DocumentStore, item CallHierarchyItem) []CallHierarchyOutgoingCall {
	if store == nil || item.Name == "" {
		return []CallHierarchyOutgoingCall{}
	}

	store.mu.RLock()
	docs := store.allEffectiveDocsLocked()
	store.mu.RUnlock()

	grouped := make(map[string]*CallHierarchyOutgoingCall)
	addChild := func(childName string, fallbackURI string, callRange ast.Range) {
		entry, exists := grouped[childName]
		if !exists {
			childItem, ok := resolveHierarchyItem(nil, store, childName)
			if !ok {
				r := toLSPRange(callRange)
				childItem = CallHierarchyItem{
					Name:           childName,
					Kind:           LSPSymbolKindModule,
					Detail:         "module (external)",
					URI:            fallbackURI,
					Range:          r,
					SelectionRange: r,
				}
			}
			entry = &CallHierarchyOutgoingCall{To: childItem}
			grouped[childName] = entry
		}
		entry.FromRanges = append(entry.FromRanges, toLSPRange(callRange))
	}

	for _, doc := range docs {
		if doc.Analysis == nil {
			continue
		}
		if modScope, ok := doc.Analysis.Modules[item.Name]; ok && modScope.Module != nil {
			var insts []*ast.ModuleInst
			collectInstsFromItems(modScope.Module.Items, &insts)
			for _, inst := range insts {
				if inst.ModuleName != "" {
					addChild(inst.ModuleName, doc.URI, inst.ModuleNameRange)
				}
			}
		}
		// Top-level `bind item.Name child_mod u_chk (...);`
		if doc.Analysis.File != nil {
			for _, b := range doc.Analysis.File.Binds {
				if b.Inst != nil && !b.IsHierarchical && b.TargetName == item.Name && b.Inst.ModuleName != "" {
					addChild(b.Inst.ModuleName, doc.URI, b.Inst.ModuleNameRange)
				}
			}
		}
	}

	out := make([]CallHierarchyOutgoingCall, 0, len(grouped))
	for _, entry := range grouped {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To.Name != out[j].To.Name {
			return out[i].To.Name < out[j].To.Name
		}
		return out[i].To.URI < out[j].To.URI
	})
	return out
}
