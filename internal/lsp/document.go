package lsp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
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

// DocumentStore manages open documents in memory.
type DocumentStore struct {
	mu          sync.RWMutex
	docs        map[string]*Document
	includeDirs []string
}

// NewDocumentStore creates an empty DocumentStore.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		docs: make(map[string]*Document),
	}
}

// SetRootURI configures the workspace root directory as an `include search path.
func (s *DocumentStore) SetRootURI(rootURI string) {
	dir := uriToFilePath(rootURI)
	if dir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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

	fileAST, parseErrs := parser.ParseWithOptions(item.Text, s.parseOptionsLocked(item.URI))
	doc := &Document{
		URI:        item.URI,
		LanguageID: item.LanguageID,
		Version:    item.Version,
		Text:       item.Text,
		AST:        fileAST,
		ParseErrs:  parseErrs,
		Analysis:   analyzer.Analyze(fileAST),
	}
	s.docs[item.URI] = doc
	return doc
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
	doc.Analysis = analyzer.Analyze(fileAST)
	return doc, nil
}

func (s *DocumentStore) parseOptionsLocked(uri string) parser.Options {
	incDirs := append([]string(nil), s.includeDirs...)
	return parser.Options{
		FilePath:    uriToFilePath(uri),
		IncludeDirs: incDirs,
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

// FindModule searches all open documents for a module declaration by name.
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
	return "", nil, false
}

// Close removes a document from the store.
func (s *DocumentStore) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()

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


