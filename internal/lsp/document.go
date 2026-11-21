package lsp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
	"kvx/internal/config"
	"kvx/internal/parser"
)

// Document represents an open text document tracked in memory by the server.
type Document struct {
	URI        string
	LanguageID string
	Version    int
	Text       string
	AST        *ast.SourceFile
	ParseErrs  []parser.ParseError
	Analysis   *analyzer.Analysis
}

// DocumentStore manages open documents in memory and indexed workspace files.
type DocumentStore struct {
	mu            sync.RWMutex
	docs          map[string]*Document
	workspaceDocs map[string]*Document
	rootDir       string
	includeDirs   []string
	autoConfig    *config.Config
}

// NewDocumentStore creates an empty DocumentStore.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		docs:          make(map[string]*Document),
		workspaceDocs: make(map[string]*Document),
	}
}

// SetRootURI configures the workspace root directory, loads kvx.json if present,
// and scans all Verilog files in the workspace to index module definitions.
func (s *DocumentStore) SetRootURI(rootURI string) (string, error) {
	dir := uriToFilePath(rootURI)
	if dir == "" {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanWorkspaceLocked(dir)
}

func (s *DocumentStore) scanWorkspaceLocked(dir string) (string, error) {
	s.rootDir = dir
	s.addIncludeDirLocked(dir)

	cfg, cfgPath, err := config.FindAndLoad(dir)
	scanDir := dir
	if cfgPath != "" {
		scanDir = filepath.Dir(cfgPath)
		s.addIncludeDirLocked(scanDir)
	}

	merged := config.MergeDiscovered(cfg, scanDir)
	s.autoConfig = merged
	if merged != nil {
		s.indexWorkspaceFilesLocked(merged)
	}
	return cfgPath, err
}

// IndexedStats returns the number of indexed workspace files and total module definitions.
func (s *DocumentStore) IndexedStats() (int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	modCount := 0
	for _, doc := range s.workspaceDocs {
		if doc.Analysis != nil {
			modCount += len(doc.Analysis.ModuleOrder)
		}
	}
	return len(s.workspaceDocs), modCount
}

func (s *DocumentStore) indexWorkspaceFilesLocked(cfg *config.Config) {
	s.workspaceDocs = make(map[string]*Document, len(cfg.Files))
	for _, path := range cfg.Files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		uri := filePathToURI(path)
		text := string(data)
		fileAST, parseErrs := parser.ParseWithOptions(text, s.parseOptionsLocked(uri))
		s.workspaceDocs[uri] = &Document{
			URI:        uri,
			LanguageID: "verilog",
			Text:       text,
			AST:        fileAST,
			ParseErrs:  parseErrs,
			Analysis:   analyzer.Analyze(fileAST),
		}
	}
	// Second pass: re-analyze files with all discovered workspace modules, packages, and interfaces for cross-file resolution.
	ctx := s.workspaceContextLocked()
	if len(ctx.Modules) > 0 || len(ctx.Packages) > 0 || len(ctx.Interfaces) > 0 {
		for _, doc := range s.workspaceDocs {
			doc.Analysis = analyzer.AnalyzeWithWorkspace(doc.AST, ctx)
		}
	}
}

func (s *DocumentStore) workspaceContextLocked() analyzer.WorkspaceContext {
	return analyzer.WorkspaceContext{
		Modules:    s.workspaceModulesLocked(),
		Packages:   s.workspacePackagesLocked(),
		Interfaces: s.workspaceInterfacesLocked(),
	}
}

func (s *DocumentStore) workspaceModulesLocked() map[string]*analyzer.ModuleScope {
	mods := make(map[string]*analyzer.ModuleScope)
	collect := func(m map[string]*Document) {
		for _, doc := range m {
			if doc.Analysis == nil {
				continue
			}
			for name, modScope := range doc.Analysis.Modules {
				if _, exists := mods[name]; !exists {
					mods[name] = modScope
				}
			}
		}
	}
	collect(s.docs)
	collect(s.workspaceDocs)
	return mods
}

func (s *DocumentStore) workspaceInterfacesLocked() map[string]*analyzer.InterfaceScope {
	ifaces := make(map[string]*analyzer.InterfaceScope)
	collect := func(m map[string]*Document) {
		for _, doc := range m {
			if doc.Analysis == nil {
				continue
			}
			for name, ifaceScope := range doc.Analysis.Interfaces {
				if _, exists := ifaces[name]; !exists {
					ifaces[name] = ifaceScope
				}
			}
		}
	}
	collect(s.docs)
	collect(s.workspaceDocs)
	return ifaces
}

// FindInterface searches open documents and indexed workspace files for an interface declaration by name.
func (s *DocumentStore) FindInterface(ifaceName string) (string, *analyzer.Symbol, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for uri, doc := range s.docs {
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Interfaces[ifaceName]; ok {
			return uri, scope.Symbol, true
		}
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Interfaces[ifaceName]; ok {
			return uri, scope.Symbol, true
		}
	}
	return "", nil, false
}

// FindInterfaceURI returns the document URI declaring ifaceName across open and indexed workspace files.
func (s *DocumentStore) FindInterfaceURI(ifaceName string) (string, bool) {
	uri, _, ok := s.FindInterface(ifaceName)
	return uri, ok
}

func (s *DocumentStore) workspacePackagesLocked() map[string]*analyzer.PackageScope {
	pkgs := make(map[string]*analyzer.PackageScope)
	collect := func(m map[string]*Document) {
		for _, doc := range m {
			if doc.Analysis == nil {
				continue
			}
			for name, pkgScope := range doc.Analysis.Packages {
				if _, exists := pkgs[name]; !exists {
					pkgs[name] = pkgScope
				}
			}
		}
	}
	collect(s.docs)
	collect(s.workspaceDocs)
	return pkgs
}

// FindPackageURI returns the document URI declaring pkgName across open and indexed workspace files.
func (s *DocumentStore) FindPackageURI(pkgName string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for uri, doc := range s.docs {
		if doc.Analysis != nil {
			if _, ok := doc.Analysis.Packages[pkgName]; ok {
				return uri, true
			}
		}
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		if doc.Analysis != nil {
			if _, ok := doc.Analysis.Packages[pkgName]; ok {
				return uri, true
			}
		}
	}
	return "", false
}

func (s *DocumentStore) addIncludeDirLocked(dir string) {
	if dir == "" {
		return
	}
	dir = filepath.Clean(dir)
	for _, existing := range s.includeDirs {
		if existing == dir {
			return
		}
	}
	s.includeDirs = append(s.includeDirs, dir)
}

// Open registers a newly opened document in the store and parses it.
func (s *DocumentStore) Open(item TextDocumentItem) *Document {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rootDir == "" {
		if fp := uriToFilePath(item.URI); fp != "" {
			dir := filepath.Clean(filepath.Dir(fp))
			if dir != "/" && dir != "." && filepath.Dir(dir) != dir {
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					_, _ = s.scanWorkspaceLocked(inferWorkspaceRoot(dir))
				}
			}
		}
	}

	fileAST, parseErrs := parser.ParseWithOptions(item.Text, s.parseOptionsLocked(item.URI))
	doc := &Document{
		URI:        item.URI,
		LanguageID: item.LanguageID,
		Version:    item.Version,
		Text:       item.Text,
		AST:        fileAST,
		ParseErrs:  parseErrs,
		Analysis:   analyzer.AnalyzeWithWorkspace(fileAST, s.workspaceContextLocked()),
	}
	s.docs[item.URI] = doc
	return doc
}

func inferWorkspaceRoot(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return startDir
	}
	cur := dir
	for {
		if _, err := os.Stat(filepath.Join(cur, config.ConfigFileName)); err == nil {
			return cur
		}
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return dir
}

// Change updates an existing document's version and content, re-parsing the updated text.
func (s *DocumentStore) Change(id VersionedTextDocumentIdentifier, changes []TextDocumentContentChangeEvent) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.docs[id.URI]
	if !ok {
		return nil, fmt.Errorf("document not open: %s", id.URI)
	}

	text := doc.Text
	for _, change := range changes {
		if change.Range == nil {
			text = change.Text
		} else {
			updated, err := applyRangeChange(text, *change.Range, change.Text)
			if err != nil {
				return nil, err
			}
			text = updated
		}
	}

	fileAST, parseErrs := parser.ParseWithOptions(text, s.parseOptionsLocked(id.URI))
	doc.Version = id.Version
	doc.Text = text
	doc.AST = fileAST
	doc.ParseErrs = parseErrs
	doc.Analysis = analyzer.AnalyzeWithWorkspace(fileAST, s.workspaceContextLocked())
	return doc, nil
}

func (s *DocumentStore) parseOptionsLocked(uri string) parser.Options {
	filePath := uriToFilePath(uri)

	var incDirs []string
	seen := make(map[string]bool)
	appendUnique := func(dirs []string) {
		for _, d := range dirs {
			if d == "" {
				continue
			}
			d = filepath.Clean(d)
			if !seen[d] {
				seen[d] = true
				incDirs = append(incDirs, d)
			}
		}
	}

	// Load project config (kvx.json) from the file's directory or workspace root.
	var cfg *config.Config
	if filePath != "" {
		cfg, _, _ = config.FindAndLoad(filepath.Dir(filePath))
	}
	if cfg == nil && s.rootDir != "" {
		cfg, _, _ = config.FindAndLoad(s.rootDir)
	}
	if cfg == nil {
		cfg = s.autoConfig
	}
	var defines map[string]string
	if cfg != nil {
		appendUnique(cfg.IncludeDirs)
		defines = cfg.Defines
	}
	appendUnique(s.includeDirs)

	return parser.Options{
		FilePath:    filePath,
		IncludeDirs: incDirs,
		Defines:     defines,
		ReadFile: func(path string) ([]byte, error) {
			cleanTarget := filepath.Clean(path)
			for docURI, doc := range s.docs {
				if docPath := uriToFilePath(docURI); docPath != "" && filepath.Clean(docPath) == cleanTarget {
					return []byte(doc.Text), nil
				}
			}
			return os.ReadFile(path)
		},
	}
}

// FindModuleScope searches open documents and indexed workspace files for a ModuleScope by name.
func (s *DocumentStore) FindModuleScope(moduleName string) (*analyzer.ModuleScope, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, doc := range s.docs {
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Modules[moduleName]; ok {
			return scope, true
		}
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Modules[moduleName]; ok {
			return scope, true
		}
	}
	return nil, false
}

// FindInterfaceScope searches open documents and indexed workspace files for an InterfaceScope by name.
func (s *DocumentStore) FindInterfaceScope(ifaceName string) (*analyzer.InterfaceScope, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, doc := range s.docs {
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Interfaces[ifaceName]; ok {
			return scope, true
		}
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Interfaces[ifaceName]; ok {
			return scope, true
		}
	}
	return nil, false
}

// FindModule searches open documents and indexed workspace files for a module declaration by name.
func (s *DocumentStore) FindModule(moduleName string) (string, *analyzer.Symbol, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for uri, doc := range s.docs {
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Modules[moduleName]; ok {
			return uri, scope.Symbol, true
		}
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		if doc.Analysis == nil {
			continue
		}
		if scope, ok := doc.Analysis.Modules[moduleName]; ok {
			return uri, scope.Symbol, true
		}
	}
	return "", nil, false
}

// FindModuleReferences searches all open documents and indexed workspace files for instantiations
// of moduleName. If includeDeclaration is true, the module declaration itself is also included.
func (s *DocumentStore) FindModuleReferences(moduleName string, includeDeclaration bool) []Location {
	if moduleName == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	type locKey struct {
		uri      string
		startLn  int
		startCol int
		endLn    int
		endCol   int
	}
	seen := make(map[locKey]bool)
	var out []Location

	addLoc := func(docURI string, rng ast.Range) {
		targetURI := docURI
		if rng.File != "" {
			targetURI = filePathToURI(rng.File)
		}
		lspRng := toLSPRange(rng)
		key := locKey{
			uri:      targetURI,
			startLn:  lspRng.Start.Line,
			startCol: lspRng.Start.Character,
			endLn:    lspRng.End.Line,
			endCol:   lspRng.End.Character,
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Location{
			URI:   targetURI,
			Range: lspRng,
		})
	}

	collectDoc := func(uri string, doc *Document) {
		if doc == nil || doc.Analysis == nil {
			return
		}
		if includeDeclaration {
			if scope, ok := doc.Analysis.Modules[moduleName]; ok && scope.Symbol != nil {
				addLoc(uri, scope.Symbol.SelectionRange)
			}
			if scope, ok := doc.Analysis.Interfaces[moduleName]; ok && scope.Symbol != nil {
				addLoc(uri, scope.Symbol.SelectionRange)
			}
		}
		for _, rng := range doc.Analysis.FindModuleInstantiations(moduleName) {
			addLoc(uri, rng)
		}
	}

	for uri, doc := range s.docs {
		collectDoc(uri, doc)
	}
	for uri, doc := range s.workspaceDocs {
		if _, isOpen := s.docs[uri]; isOpen {
			continue
		}
		collectDoc(uri, doc)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].Range.Start.Character < out[j].Range.Start.Character
	})
	return out
}

// WorkspaceModules returns all module symbols from indexed workspace files and open documents.
func (s *DocumentStore) WorkspaceModules() []*analyzer.Symbol {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*analyzer.Symbol
	seen := make(map[string]bool)
	collect := func(m map[string]*Document) {
		for _, doc := range m {
			if doc.Analysis == nil {
				continue
			}
			for _, modScope := range doc.Analysis.ModuleOrder {
				if modScope.Symbol.Name != "" && !seen[modScope.Symbol.Name] {
					seen[modScope.Symbol.Name] = true
					out = append(out, modScope.Symbol)
				}
			}
		}
	}
	collect(s.docs)
	collect(s.workspaceDocs)
	return out
}

// WorkspaceInterfaces returns all interface symbols from indexed workspace files and open documents.
func (s *DocumentStore) WorkspaceInterfaces() []*analyzer.Symbol {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*analyzer.Symbol
	seen := make(map[string]bool)
	collect := func(m map[string]*Document) {
		for _, doc := range m {
			if doc.Analysis == nil {
				continue
			}
			for _, ifaceScope := range doc.Analysis.InterfaceOrder {
				if ifaceScope.Symbol.Name != "" && !seen[ifaceScope.Symbol.Name] {
					seen[ifaceScope.Symbol.Name] = true
					out = append(out, ifaceScope.Symbol)
				}
			}
		}
	}
	collect(s.docs)
	collect(s.workspaceDocs)
	return out
}

// Close removes a document from the active open store while keeping indexed workspace files.
func (s *DocumentStore) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if doc, ok := s.docs[uri]; ok {
		if _, wasIndexed := s.workspaceDocs[uri]; wasIndexed {
			s.workspaceDocs[uri] = doc
		} else if fp := uriToFilePath(uri); fp != "" {
			if _, err := os.Stat(fp); err == nil {
				s.workspaceDocs[uri] = doc
			}
		}
	}
	delete(s.docs, uri)
}

// Get retrieves a copy of a document by its URI.
func (s *DocumentStore) Get(uri string) (Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.docs[uri]
	if !ok {
		return Document{}, false
	}
	return *doc, true
}

// applyRangeChange replaces the text within rng (0-based line/character offsets) with replacement.
func applyRangeChange(text string, rng Range, replacement string) (string, error) {
	startOffset, err := positionToByteOffset(text, rng.Start)
	if err != nil {
		return "", fmt.Errorf("start position: %w", err)
	}
	endOffset, err := positionToByteOffset(text, rng.End)
	if err != nil {
		return "", fmt.Errorf("end position: %w", err)
	}
	if startOffset > endOffset {
		return "", fmt.Errorf("invalid range: start offset %d > end offset %d", startOffset, endOffset)
	}
	return text[:startOffset] + replacement + text[endOffset:], nil
}

// positionToByteOffset converts a 0-based LSP Position into a byte offset within text.
func positionToByteOffset(text string, pos Position) (int, error) {
	if pos.Line < 0 || pos.Character < 0 {
		return 0, fmt.Errorf("negative position (%d:%d)", pos.Line, pos.Character)
	}

	line := 0
	lineStart := 0
	for i := 0; i < len(text); i++ {
		if line == pos.Line {
			break
		}
		if text[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	if line < pos.Line {
		return 0, fmt.Errorf("line %d out of range (total lines %d)", pos.Line, line+1)
	}

	// Find end of target line.
	lineEnd := len(text)
	for i := lineStart; i < len(text); i++ {
		if text[i] == '\n' {
			lineEnd = i
			if lineEnd > lineStart && text[lineEnd-1] == '\r' {
				lineEnd--
			}
			break
		}
	}

	offset := lineStart + pos.Character
	if offset > lineEnd {
		offset = lineEnd
	}
	return offset, nil
}

// toLSPRange converts an AST Range to an LSP Range.
func toLSPRange(r ast.Range) Range {
	return Range{
		Start: Position{
			Line:      r.Start.Line,
			Character: r.Start.Column,
		},
		End: Position{
			Line:      r.End.Line,
			Character: r.End.Column,
		},
	}
}

func uriToFilePath(uri string) string {
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "file://") {
		if u, err := url.Parse(uri); err == nil && u.Path != "" {
			return filepath.FromSlash(u.Path)
		}
		return filepath.FromSlash(strings.TrimPrefix(uri, "file://"))
	}
	return uri
}

func filePathToURI(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "file://") {
		return path
	}
	abs := path
	if a, err := filepath.Abs(path); err == nil {
		abs = a
	}
	u := url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(abs),
	}
	return u.String()
}


