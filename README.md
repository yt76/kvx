# kvx - Verilog Language Server

`kvx` is a lightweight, fast Language Server (LSP) for Verilog (IEEE 1364) implemented solely with the Go standard library. It has no external dependencies (such as CGO) and runs as a single binary.

## Features

- **Diagnostics (`textDocument/publishDiagnostics`)**
  - Real-time syntax error detection (recursive descent parser with error recovery)
  - Warnings for `undeclared identifier` and `duplicate declaration`
- **Document Symbols (`textDocument/documentSymbol`)**
  - Hierarchical outline of modules, parameters, I/O ports, signals (`wire` / `reg` / `integer`), and instances
- **Hover (`textDocument/hover`)**
  - Displays the type and bit-width of the signal, port, parameter, or module under the cursor
- **Go to Definition (`textDocument/definition`)**
  - Jump to signal/parameter declarations and instantiated module definitions
- **Completion (`textDocument/completion`)**
  - Autocompletion for in-scope signals, ports, parameters, module names, and Verilog keywords
- **Semantic Tokens (`textDocument/semanticTokens/full`)**
  - Semantic highlighting for Verilog keywords, module names, parameters, I/O ports, signal variables, numeric/string literals, system tasks, macros, and operators

---

## Build and Installation

Can be built without external libraries as long as Go 1.21 or later is installed.

### Building a local binary

```bash
go build -o kvx ./cmd/kvx
```

### Installing to `$GOPATH/bin` (or `$GOBIN`)

```bash
go install ./cmd/kvx
```

### Running tests

```bash
go test -v ./...
```

---

## CLI Syntax Checking and Debugging

Running `kvx` with no arguments, or with `-`, `--stdio`, or `lsp`, starts it in LSP server mode (`stdio`). Passing file paths lets you check Verilog syntax or inspect ASTs and tokens from the command line.

```bash
# Check syntax errors in single or multiple files
./kvx path/to/design.v
./kvx check rtl/*.v

# Run with debug options (for debugging or feature development)
./kvx -tokens path/to/design.v    # Print token stream
./kvx -ast path/to/design.v       # Print parsed AST summary
./kvx -semantic path/to/design.v  # Also report semantic warnings (undeclared/duplicate signals, etc.)
```

Example output (when syntax errors exist, `^` marks the line and column, and exit code `1` is returned):

```text
broken.v:2:8: syntax error: expected ';' after wire declaration
  wire x
       ^
broken.v: FAILED (1 syntax error(s))
```

When there are no syntax errors (exit code `0`):

```text
valid.v: OK (0 syntax errors, 1 module(s))
```

---

## Editor Configuration

`kvx` communicates over LSP via standard input/output (`stdio`).

### Neovim (Built-in LSP: `vim.lsp.start`)

Add the following to your `init.lua`, etc. (adjust the `cmd` path to match your built `kvx` binary):

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "verilog", "systemverilog" },
  callback = function(args)
    vim.lsp.start({
      name = "kvx",
      cmd = { "/path/to/kvx" }, -- or { "kvx" } if it is in your PATH
      root_dir = vim.fs.root(args.buf, { ".git" }) or vim.fn.getcwd(),
    })
  end,
})
```

### Neovim (`nvim-lspconfig`)

```lua
local lspconfig = require("lspconfig")
local configs = require("lspconfig.configs")

if not configs.kvx then
  configs.kvx = {
    default_config = {
      cmd = { "kvx" },
      filetypes = { "verilog", "systemverilog" },
      root_dir = lspconfig.util.root_pattern(".git"),
      settings = {},
    },
  }
end

lspconfig.kvx.setup({})
```

### Helix (`~/.config/helix/languages.toml`)

```toml
[language-server.kvx]
command = "kvx"

[[language]]
name = "verilog"
language-servers = ["kvx"]
```

### Emacs (`eglot`)

```elisp
(with-eval-after-load 'eglot
  (add-to-list 'eglot-server-programs
               '((verilog-mode) . ("kvx"))))
```

### VS Code

You can build and use the bundled VS Code extension in `vscode/` (requires Node.js / npm).

#### 1. Install dependencies and compile

```bash
cd vscode
npm install
npm run compile
```

- Run `npm run watch` to automatically recompile on file changes during development, or `npm run lint` to run linting.
- To test without packaging, open the `vscode/` folder in VS Code and press `F5` to launch an Extension Development Host.

#### 2. Package as `.vsix` and install

```bash
cd vscode
npx @vscode/vsce package
code --install-extension kvx-0.0.1.vsix
```

(You can also install it from the Extensions view menu via "Install from VSIX...")

#### 3. VS Code settings

After installing the extension, set `kvx.serverPath` in VS Code `settings.json` (supports the `${workspaceFolder}` placeholder) or set the `KVX_SERVER_PATH` environment variable to the path of your built `kvx` binary.

```json
{
  "kvx.serverPath": "/path/to/kvx",
  "kvx.serverArgs": []
}
```

---

## Project Configuration (`kvx.json`)

Even without `kvx.json`, on LSP startup `kvx` **automatically discovers (zero-config)** `.v` / `.vh` directories under the workspace and `.f` files at the workspace root, and indexes them.

If you want explicit settings or fixed macro definitions, you can generate `kvx.json` with `kvx init`:

```bash
# Scan the project automatically and generate kvx.json
kvx init

# Generate kvx.json from simulator arguments (iverilog / verilator, etc.)
kvx init -- -I rtl/include -D SIM -f sim/filelist.f rtl/top.v
```

Example generated `kvx.json`:

```json
{
  "files": [
    "rtl/*.v",
    "ip/common/fifo.v"
  ],
  "includeDirs": [
    "rtl/include",
    "ip/common/inc"
  ],
  "filelists": [
    "sim/filelist.f"
  ],
  "defines": {
    "SIM": "",
    "DATA_WIDTH": "16"
  }
}
```

- **`files`**: Verilog source files in the project (glob patterns supported). Indexed at LSP startup so Go to Definition, Hover, and Completion work across unopened files (also used as the batch check target when running `kvx check` without arguments).
- **`includeDirs`**: List of directories searched for `` `include `` directives (relative to `kvx.json` or absolute paths).
- **`filelists`**: Paths to simulation filelist (`.f`) files. Automatically extracts `-I <dir>`, `-I<dir>`, `+incdir+<dir1>+<dir2>`, `-D`, `+define+` options and listed `.v` file paths.
- **`defines`**: Map of predefined macros (equivalent to `` `define ``). Use `""` for flag macros without values (e.g., `"SIM": ""`). Applied to `` `ifdef `` condition evaluation and `` `DATA_WIDTH `` macro expansion.

### Specifying the config file via environment variable (`KVX_CONFIG`)

Setting the `KVX_CONFIG` environment variable to a config file path overrides the automatic `kvx.json` search and loads that file instead.

```bash
KVX_CONFIG=/path/to/custom_kvx.json kvx check
```

---

## Logging (for Debugging)

In LSP mode, logs are written to standard error (`stderr`). If the `KVX_LOG` environment variable is set to a file path, logs are also appended to that file in addition to `stderr`.

```bash
KVX_LOG=/tmp/kvx.log kvx
```
