package lsp

// TextDocumentSyncKind defines how the host (editor) should sync document changes to the language server.
type TextDocumentSyncKind int

const (
	SyncNone        TextDocumentSyncKind = 0
	SyncFull        TextDocumentSyncKind = 1
	SyncIncremental TextDocumentSyncKind = 2
)

// InitializeParams represents the parameters sent in an "initialize" request.
type InitializeParams struct {
	ProcessID  *int        `json:"processId,omitempty"`
	RootURI    string      `json:"rootUri,omitempty"`
	ClientInfo *ClientInfo `json:"clientInfo,omitempty"`
}

// ClientInfo holds information about the editor/client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeResult is returned by the server in response to an "initialize" request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerCapabilities declares the features supported by this language server.
type ServerCapabilities struct {
	TextDocumentSync       TextDocumentSyncKind   `json:"textDocumentSync,omitempty"`
	HoverProvider          bool                   `json:"hoverProvider,omitempty"`
	DefinitionProvider     bool                   `json:"definitionProvider,omitempty"`
	DocumentSymbolProvider bool                   `json:"documentSymbolProvider,omitempty"`
	CompletionProvider     *CompletionOptions     `json:"completionProvider,omitempty"`
	SemanticTokensProvider *SemanticTokensOptions `json:"semanticTokensProvider,omitempty"`
}

// CompletionOptions configures completion behavior.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// SemanticTokensOptions configures semantic token support advertised by the server.
type SemanticTokensOptions struct {
	Legend SemanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
}

// SemanticTokensLegend maps token type and modifier indices to standard LSP names.
type SemanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

// ServerInfo holds metadata about this language server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Position in a text document expressed as zero-based line and character offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document expressed as start and end positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextDocumentItem represents a text document transferred from client to server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentIdentifier identifies a text document using its URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier denotes a specific version of a text document.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentContentChangeEvent describes a change to a text document.
// When Range is nil, Text is considered the full new content of the document.
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidOpenTextDocumentParams is sent with "textDocument/didOpen".
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams is sent with "textDocument/didChange".
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidCloseTextDocumentParams is sent with "textDocument/didClose".
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DiagnosticSeverity indicates the severity of a diagnostic.
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic represents a compiler/parser error or warning at a source range.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// PublishDiagnosticsParams is sent from server to client via "textDocument/publishDiagnostics".
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// TextDocumentPositionParams is a parameter literal used in requests such as hover and definition.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// Location represents a location inside a resource, such as a line inside a text file.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// MarkupContent represents a string value with a specific markup kind ("plaintext" or "markdown").
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Hover is the result of a "textDocument/hover" request.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// DocumentSymbolParams is sent with "textDocument/documentSymbol".
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// LSPSymbolKind defines numeric symbol kinds per the LSP specification.
type LSPSymbolKind int

const (
	LSPSymbolKindModule   LSPSymbolKind = 2
	LSPSymbolKindField    LSPSymbolKind = 8
	LSPSymbolKindFunction LSPSymbolKind = 12
	LSPSymbolKindVariable LSPSymbolKind = 13
	LSPSymbolKindConstant LSPSymbolKind = 14
	LSPSymbolKindObject   LSPSymbolKind = 19
)

// DocumentSymbol represents programming constructs like modules, ports, or nets in an outline.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           LSPSymbolKind    `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// CompletionParams is sent with "textDocument/completion".
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionItemKind defines numeric completion item kinds per the LSP specification.
type CompletionItemKind int

const (
	CompletionKindFunction CompletionItemKind = 3
	CompletionKindField    CompletionItemKind = 5
	CompletionKindVariable CompletionItemKind = 6
	CompletionKindModule   CompletionItemKind = 9
	CompletionKindKeyword  CompletionItemKind = 14
	CompletionKindConstant CompletionItemKind = 21
)

// CompletionItem represents a completion suggestion returned by "textDocument/completion".
type CompletionItem struct {
	Label  string             `json:"label"`
	Kind   CompletionItemKind `json:"kind,omitempty"`
	Detail string             `json:"detail,omitempty"`
}

// SemanticTokensParams is sent with "textDocument/semanticTokens/full".
type SemanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// SemanticTokens holds the relative delta-encoded semantic token array.
type SemanticTokens struct {
	Data []uint32 `json:"data"`
}




