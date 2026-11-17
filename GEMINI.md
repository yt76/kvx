# kvx - Verilog Language Server in Go

## Project Overview
`kvx` is a Language Server Protocol (LSP) implementation for Verilog written in Go.

## Architecture & Design Decisions
- **Zero External Dependencies (Standard Library Only)**:
  - Both the LSP transport/protocol layer (JSON-RPC 2.0 over stdio) and the Verilog parser are implemented using only the Go standard library (no CGO, no third-party LSP or parser generator libraries).
- **Hand-Written Lexer & Parser**:
  - Custom lexer and recursive descent parser designed with error recovery in mind so incomplete/broken code during editing still produces a partial AST and useful diagnostics.
- **Target Language Scope (Initial Core Subset)**:
  - Focus on core RTL Verilog (IEEE 1364 Verilog-2001/2005 subset):
    - Module declarations (`module`, `endmodule`, parameters, port lists)
    - Port declarations (`input`, `output`, `inout`)
    - Net and variable types (`wire`, `reg`, `integer`, `localparam`, `parameter`, bit-width ranges `[msb:lsb]`)
    - Continuous assignments (`assign`)
    - Procedural blocks (`always @(...)`, `initial`, `begin`/`end`, `if`/`else`, `case`)
    - Module instantiations

## Directory Structure
```text
kvx/
├── go.mod
├── cmd/
│   └── kvx/             # Entry point (stdio connection & server startup)
└── internal/
    ├── lsp/             # JSON-RPC 2.0 transport, LSP protocol types, server & document state
    ├── ast/             # Verilog AST node definitions
    ├── lexer/           # Lexical analysis (tokenizer)
    ├── parser/          # Syntax analysis (recursive descent parser & error recovery)
    └── analyzer/        # Symbol table construction, scope resolution, and semantic checks
```

## Implementation Roadmap
1. **Step 1: Minimal LSP Server (Handshake & Transport)**
   - JSON-RPC 2.0 framing (`Content-Length` header over stdio)
   - Handle `initialize`, `initialized`, `shutdown`, and `exit`
2. **Step 2: Document Synchronization**
   - In-memory document store handling `textDocument/didOpen`, `textDocument/didChange`, and `textDocument/didClose`
3. **Step 3: Lexer, Parser & Diagnostics**
   - Tokenize and parse core Verilog syntax
   - Publish syntax errors via `textDocument/publishDiagnostics`
4. **Step 4: Symbol Table & Core LSP Features**
   - `textDocument/documentSymbol` (outline of modules, ports, signals)
   - `textDocument/hover` (display signal type and bit-width)
   - `textDocument/definition` (jump to declaration of signals/modules)
   - `textDocument/completion` (autocomplete signals and Verilog keywords)

## Coding Guidelines
- Keep packages decoupled: `lexer`, `ast`, and `parser` must not depend on `lsp`.
- Write table-driven unit tests (`*_test.go`) for each package (`lexer`, `parser`, `lsp`, `analyzer`).
- Never write logs or debug output to `stdout` (since `stdout` is used for LSP JSON-RPC communication); always use `stderr` or a log file for logging.
