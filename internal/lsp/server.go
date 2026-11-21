package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
)

const (
	ServerName    = "kvx"
	ServerVersion = "0.1.0"
)

// ErrExitWithoutShutdown is returned by Run when the client sends "exit" before "shutdown".
var ErrExitWithoutShutdown = errors.New("received exit notification without prior shutdown request")

// Server manages the LSP lifecycle and dispatches JSON-RPC messages.
type Server struct {
	reader           *bufio.Reader
	writer           io.Writer
	logger           *log.Logger
	docs             *DocumentStore
	initialized      bool
	shutdownReceived bool
}

// NewServer creates a new LSP Server reading from in, writing JSON-RPC responses to out,
// and writing logs to logger (if nil, logs are discarded).
func NewServer(in io.Reader, out io.Writer, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Server{
		reader: bufio.NewReader(in),
		writer: out,
		logger: logger,
		docs:   NewDocumentStore(),
	}
}

// GetDocument returns a copy of an open document from the server's store.
func (s *Server) GetDocument(uri string) (Document, bool) {
	return s.docs.Get(uri)
}

// Run processes incoming LSP messages until an "exit" notification or EOF occurs.
func (s *Server) Run() error {
	for {
		payload, err := ReadMessage(s.reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("reading message: %w", err)
		}

		shouldExit, err := s.handlePayload(payload)
		if err != nil {
			return err
		}
		if shouldExit {
			if !s.shutdownReceived {
				return ErrExitWithoutShutdown
			}
			return nil
		}
	}
}

func (s *Server) handlePayload(payload []byte) (bool, error) {
	var msg IncomingMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		s.logger.Printf("failed to unmarshal incoming message: %v", err)
		if writeErr := s.sendError(json.RawMessage("null"), CodeParseError, "parse error"); writeErr != nil {
			return false, writeErr
		}
		return false, nil
	}

	s.logger.Printf("received method=%q isRequest=%v", msg.Method, msg.IsRequest())

	if msg.Method == "exit" {
		return true, nil
	}

	if !s.initialized && msg.Method != "initialize" {
		if msg.IsRequest() {
			return false, s.sendError(msg.ID, CodeServerNotInitialized, "server not initialized")
		}
		// Ignore notifications before initialize (except exit, handled above).
		return false, nil
	}

	if s.shutdownReceived {
		if msg.IsRequest() {
			return false, s.sendError(msg.ID, CodeInvalidRequest, "server is shutting down")
		}
		return false, nil
	}

	switch msg.Method {
	case "initialize":
		return false, s.handleInitialize(msg)
	case "initialized":
		s.logger.Println("client initialized handshake completed")
		return false, nil
	case "shutdown":
		s.shutdownReceived = true
		s.logger.Println("shutdown request received")
		return false, s.sendResult(msg.ID, nil)
	case "textDocument/didOpen":
		return false, s.handleDidOpen(msg)
	case "textDocument/didChange":
		return false, s.handleDidChange(msg)
	case "textDocument/didClose":
		return false, s.handleDidClose(msg)
	case "textDocument/documentSymbol":
		return false, s.handleDocumentSymbol(msg)
	case "textDocument/hover":
		return false, s.handleHover(msg)
	case "textDocument/definition":
		return false, s.handleDefinition(msg)
	case "textDocument/references":
		return false, s.handleReferences(msg)
	case "textDocument/completion":
		return false, s.handleCompletion(msg)
	case "textDocument/semanticTokens/full":
		return false, s.handleSemanticTokensFull(msg)
	default:
		if msg.IsRequest() {
			return false, s.sendError(msg.ID, CodeMethodNotFound, fmt.Sprintf("method not found: %s", msg.Method))
		}
		s.logger.Printf("ignoring unhandled notification: %s", msg.Method)
		return false, nil
	}
}

func (s *Server) handleDidOpen(msg IncomingMessage) error {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("invalid didOpen params: %v", err)
		return nil
	}

	doc := s.docs.Open(params.TextDocument)
	s.logger.Printf("opened document uri=%s version=%d len=%d", doc.URI, doc.Version, len(doc.Text))
	return s.publishDiagnostics(doc)
}

func (s *Server) handleDidChange(msg IncomingMessage) error {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("invalid didChange params: %v", err)
		return nil
	}

	doc, err := s.docs.Change(params.TextDocument, params.ContentChanges)
	if err != nil {
		s.logger.Printf("failed to update document %s: %v", params.TextDocument.URI, err)
		return nil
	}
	s.logger.Printf("updated document uri=%s version=%d len=%d", doc.URI, doc.Version, len(doc.Text))
	return s.publishDiagnostics(doc)
}

func (s *Server) handleDidClose(msg IncomingMessage) error {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("invalid didClose params: %v", err)
		return nil
	}

	s.docs.Close(params.TextDocument.URI)
	s.logger.Printf("closed document uri=%s", params.TextDocument.URI)
	return s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

func (s *Server) publishDiagnostics(doc *Document) error {
	diags := make([]Diagnostic, 0, len(doc.ParseErrs))
	for _, pe := range doc.ParseErrs {
		diags = append(diags, Diagnostic{
			Range:    toLSPRange(pe.Range),
			Severity: SeverityError,
			Source:   ServerName,
			Message:  pe.Message,
		})
	}
	if doc.Analysis != nil {
		for _, sd := range doc.Analysis.Diagnostics {
			sev := SeverityWarning
			if sd.IsError {
				sev = SeverityError
			}
			diags = append(diags, Diagnostic{
				Range:    toLSPRange(sd.Range),
				Severity: sev,
				Source:   ServerName,
				Message:  sd.Message,
			})
		}
	}
	return s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diags,
	})
}

func (s *Server) handleDocumentSymbol(msg IncomingMessage) error {
	var params DocumentSymbolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid documentSymbol params: %v", err))
	}

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil {
		return s.sendResult(msg.ID, []DocumentSymbol{})
	}

	out := make([]DocumentSymbol, 0, len(doc.Analysis.PackageOrder)+len(doc.Analysis.InterfaceOrder)+len(doc.Analysis.ModuleOrder))
	for _, pkgScope := range doc.Analysis.PackageOrder {
		if ds, ok := toDocumentSymbol(pkgScope.Symbol); ok {
			out = append(out, ds)
		}
	}
	for _, ifaceScope := range doc.Analysis.InterfaceOrder {
		if ds, ok := toDocumentSymbol(ifaceScope.Symbol); ok {
			out = append(out, ds)
		}
	}
	for _, modScope := range doc.Analysis.ModuleOrder {
		if ds, ok := toDocumentSymbol(modScope.Symbol); ok {
			out = append(out, ds)
		}
	}
	return s.sendResult(msg.ID, out)
}

func (s *Server) handleHover(msg IncomingMessage) error {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid hover params: %v", err))
	}

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil {
		return s.sendResult(msg.ID, nil)
	}

	pos := ast.Position{Line: params.Position.Line, Column: params.Position.Character}
	sym := doc.Analysis.SymbolAt(pos)
	if sym == nil {
		extLookup := func(modName string) *analyzer.ModuleScope {
			if scope, found := s.docs.FindModuleScope(modName); found {
				return scope
			}
			return nil
		}
		if mSym, _ := doc.Analysis.ResolveMemberAt(pos, extLookup); mSym != nil {
			sym = mSym
		} else if pSym, _ := doc.Analysis.ResolveInstPortAt(pos, extLookup); pSym != nil {
			sym = pSym
		}
	}
	if sym == nil {
		return s.sendResult(msg.ID, nil)
	}

	// If hovering over an external module or interface instantiation, try resolving across workspace documents.
	if sym.Kind == analyzer.KindModule && sym.Signature == "module "+sym.Name {
		if _, targetSym, found := s.docs.FindModule(sym.Name); found {
			sym = targetSym
		} else if _, targetSym, found := s.docs.FindInterface(sym.Name); found {
			sym = targetSym
		}
	} else if sym.Kind == analyzer.KindInterface && sym.Signature == "interface "+sym.Name {
		if _, targetSym, found := s.docs.FindInterface(sym.Name); found {
			sym = targetSym
		}
	}

	hover := Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: fmt.Sprintf("```verilog\n%s\n```", sym.Signature),
		},
	}
	return s.sendResult(msg.ID, hover)
}

func (s *Server) handleDefinition(msg IncomingMessage) error {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid definition params: %v", err))
	}

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil {
		return s.sendResult(msg.ID, nil)
	}

	pos := ast.Position{Line: params.Position.Line, Column: params.Position.Character}
	sym := doc.Analysis.SymbolAt(pos)
	extOwnerMod := ""
	if sym == nil {
		extLookup := func(modName string) *analyzer.ModuleScope {
			if scope, found := s.docs.FindModuleScope(modName); found {
				return scope
			}
			return nil
		}
		if mSym, owner := doc.Analysis.ResolveMemberAt(pos, extLookup); mSym != nil {
			sym = mSym
			extOwnerMod = owner
		} else if pSym, owner := doc.Analysis.ResolveInstPortAt(pos, extLookup); pSym != nil {
			sym = pSym
			extOwnerMod = owner
		}
	}
	if sym == nil {
		return s.sendResult(msg.ID, nil)
	}

	targetURI := doc.URI
	targetRange := sym.SelectionRange

	if extOwnerMod != "" && targetRange.File == "" {
		if extURI, _, found := s.docs.FindModule(extOwnerMod); found {
			targetURI = extURI
		} else if extURI, _, found := s.docs.FindInterface(extOwnerMod); found {
			targetURI = extURI
		}
	}

	if sym.PackageName != "" && targetRange.File == "" {
		if _, existsInSameDoc := doc.Analysis.Packages[sym.PackageName]; !existsInSameDoc {
			if pkgURI, found := s.docs.FindPackageURI(sym.PackageName); found {
				targetURI = pkgURI
			}
		}
	}

	if sym.InterfaceName != "" && targetRange.File == "" {
		if _, existsInSameDoc := doc.Analysis.Interfaces[sym.InterfaceName]; !existsInSameDoc {
			if ifaceURI, found := s.docs.FindInterfaceURI(sym.InterfaceName); found {
				targetURI = ifaceURI
			}
		}
	}

	if sym.Kind == analyzer.KindInstance && sym.TargetModule != "" {
		if targetScope, existsInSameDoc := doc.Analysis.Modules[sym.TargetModule]; existsInSameDoc {
			targetRange = targetScope.Symbol.SelectionRange
		} else if ifaceScope, existsInSameDoc := doc.Analysis.Interfaces[sym.TargetModule]; existsInSameDoc {
			targetRange = ifaceScope.Symbol.SelectionRange
		} else if extURI, extSym, found := s.docs.FindModule(sym.TargetModule); found {
			targetURI = extURI
			targetRange = extSym.SelectionRange
		} else if extURI, extSym, found := s.docs.FindInterface(sym.TargetModule); found {
			targetURI = extURI
			targetRange = extSym.SelectionRange
		}
	} else if sym.Kind == analyzer.KindModule && targetRange.File == "" {
		if _, existsInSameDoc := doc.Analysis.Modules[sym.Name]; !existsInSameDoc {
			if extURI, extSym, found := s.docs.FindModule(sym.Name); found {
				targetURI = extURI
				targetRange = extSym.SelectionRange
			} else if extURI, extSym, found := s.docs.FindInterface(sym.Name); found {
				targetURI = extURI
				targetRange = extSym.SelectionRange
			} else {
				return s.sendResult(msg.ID, nil)
			}
		}
	} else if sym.Kind == analyzer.KindInterface && targetRange.File == "" {
		if _, existsInSameDoc := doc.Analysis.Interfaces[sym.Name]; !existsInSameDoc {
			if extURI, extSym, found := s.docs.FindInterface(sym.Name); found {
				targetURI = extURI
				targetRange = extSym.SelectionRange
			} else {
				return s.sendResult(msg.ID, nil)
			}
		}
	}

	if targetRange.File != "" {
		targetURI = filePathToURI(targetRange.File)
	}

	loc := Location{
		URI:   targetURI,
		Range: toLSPRange(targetRange),
	}
	return s.sendResult(msg.ID, loc)
}

func (s *Server) handleReferences(msg IncomingMessage) error {
	var params ReferenceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid references params: %v", err))
	}

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil {
		return s.sendResult(msg.ID, []Location{})
	}

	pos := ast.Position{Line: params.Position.Line, Column: params.Position.Character}
	sym := doc.Analysis.SymbolAt(pos)
	if sym == nil {
		return s.sendResult(msg.ID, []Location{})
	}

	targetModule := ""
	if (sym.Kind == analyzer.KindModule || sym.Kind == analyzer.KindInterface) && sym.TypeDetail != "`include" {
		targetModule = sym.Name
	} else if (sym.Kind == analyzer.KindInstance || sym.Kind == analyzer.KindPort) && sym.TargetModule != "" {
		targetModule = sym.TargetModule
	}

	if targetModule == "" {
		return s.sendResult(msg.ID, []Location{})
	}

	locs := s.docs.FindModuleReferences(targetModule, params.Context.IncludeDeclaration)
	if locs == nil {
		locs = []Location{}
	}
	return s.sendResult(msg.ID, locs)
}

func (s *Server) handleCompletion(msg IncomingMessage) error {
	var params CompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid completion params: %v", err))
	}

	var docPtr *Document
	if doc, ok := s.docs.Get(params.TextDocument.URI); ok {
		docPtr = &doc
	}
	items := computeCompletions(docPtr, s.docs, params.Position)
	if items == nil {
		items = []CompletionItem{}
	}
	return s.sendResult(msg.ID, items)
}

func (s *Server) handleSemanticTokensFull(msg IncomingMessage) error {
	var params SemanticTokensParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid semanticTokens params: %v", err))
	}

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return s.sendResult(msg.ID, SemanticTokens{Data: []uint32{}})
	}

	return s.sendResult(msg.ID, computeSemanticTokens(&doc))
}

func toDocumentSymbol(sym *analyzer.Symbol) (DocumentSymbol, bool) {
	if sym == nil || sym.Name == "" {
		return DocumentSymbol{}, false
	}
	ds := DocumentSymbol{
		Name:           sym.Name,
		Detail:         sym.TypeDetail,
		Kind:           toLSPSymbolKind(sym.Kind),
		Range:          toLSPRange(sym.Range),
		SelectionRange: toLSPRange(sym.SelectionRange),
	}
	for _, child := range sym.Children {
		if childDS, ok := toDocumentSymbol(child); ok {
			ds.Children = append(ds.Children, childDS)
		}
	}
	return ds, true
}

func toLSPSymbolKind(k analyzer.SymbolKind) LSPSymbolKind {
	switch k {
	case analyzer.KindModule, analyzer.KindPackage:
		return LSPSymbolKindModule
	case analyzer.KindInterface:
		return LSPSymbolKindInterface
	case analyzer.KindPort, analyzer.KindModport, analyzer.KindField:
		return LSPSymbolKindField
	case analyzer.KindNet:
		return LSPSymbolKindVariable
	case analyzer.KindParam:
		return LSPSymbolKindConstant
	case analyzer.KindInstance:
		return LSPSymbolKindObject
	case analyzer.KindFunction, analyzer.KindTask:
		return LSPSymbolKindFunction
	case analyzer.KindType:
		return LSPSymbolKindStruct
	default:
		return LSPSymbolKindVariable
	}
}

func toCompletionKind(k analyzer.SymbolKind) CompletionItemKind {
	switch k {
	case analyzer.KindModule, analyzer.KindPackage:
		return CompletionKindModule
	case analyzer.KindInterface:
		return CompletionKindInterface
	case analyzer.KindPort, analyzer.KindModport, analyzer.KindField:
		return CompletionKindField
	case analyzer.KindParam:
		return CompletionKindConstant
	case analyzer.KindFunction, analyzer.KindTask:
		return CompletionKindFunction
	case analyzer.KindType:
		return CompletionKindStruct
	default:
		return CompletionKindVariable
	}
}

func (s *Server) sendNotification(method string, params any) error {
	notif := NotificationMessage{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	}
	return WriteMessage(s.writer, notif)
}

func (s *Server) handleInitialize(msg IncomingMessage) error {
	var params InitializeParams
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.sendError(msg.ID, CodeInvalidParams, fmt.Sprintf("invalid initialize params: %v", err))
		}
	}

	rootURI := params.RootURI
	if rootURI == "" && len(params.WorkspaceFolders) > 0 {
		rootURI = params.WorkspaceFolders[0].URI
	}
	if rootURI == "" && params.RootPath != "" {
		rootURI = filePathToURI(params.RootPath)
	}

	if rootURI != "" {
		cfgPath, cfgErr := s.docs.SetRootURI(rootURI)
		if cfgErr != nil {
			s.logger.Printf("warning: failed to load %s: %v", cfgPath, cfgErr)
		} else if cfgPath != "" {
			s.logger.Printf("loaded project config: %s", cfgPath)
		}
		filesCount, modCount := s.docs.IndexedStats()
		s.logger.Printf("scanned workspace %s: indexed %d Verilog file(s), %d module(s)", rootURI, filesCount, modCount)
	}
	if params.ClientInfo != nil {
		s.logger.Printf("initializing for client: %s %s (rootUri=%s)",
			params.ClientInfo.Name, params.ClientInfo.Version, rootURI)
	}

	s.initialized = true

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:       SyncFull,
			HoverProvider:          true,
			DefinitionProvider:     true,
			ReferencesProvider:     true,
			DocumentSymbolProvider: true,
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{".", ":"},
			},
			SemanticTokensProvider: &SemanticTokensOptions{
				Legend: defaultSemanticTokensLegend,
				Full:   true,
			},
		},
		ServerInfo: &ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}

	return s.sendResult(msg.ID, result)
}

func (s *Server) sendResult(id json.RawMessage, result any) error {
	resp := ResponseMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
	return WriteMessage(s.writer, resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string) error {
	resp := ResponseMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error: &ResponseError{
			Code:    code,
			Message: message,
		},
	}
	return WriteMessage(s.writer, resp)
}
