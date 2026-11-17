# kvx - Verilog Language Server

`kvx` は Go 標準ライブラリのみで実装された軽量・高速な Verilog (IEEE 1364) 向けの Language Server (LSP) です。外部依存（CGO 等）がなく、単一バイナリとして動作します。

## 主な機能

- **Diagnostics (`textDocument/publishDiagnostics`)**
  - 構文エラーのリアルタイム検出（エラー回復付き再帰下降パーサー）
  - 未宣言の信号（`undeclared identifier`）や重複宣言（`duplicate declaration`）の警告表示
- **Document Symbols (`textDocument/documentSymbol`)**
  - モジュール、パラメータ、入出力ポート、信号（`wire` / `reg` / `integer`）、インスタンスの階層アウトライン表示
- **Hover (`textDocument/hover`)**
  - カーソル下の信号・ポート・パラメータ・モジュールの型やビット幅の表示
- **Go to Definition (`textDocument/definition`)**
  - 信号・パラメータの宣言箇所へのジャンプ、およびインスタンス化されたモジュール定義へのジャンプ
- **Completion (`textDocument/completion`)**
  - スコープ内の信号名・ポート名・パラメータ名・モジュール名、および Verilog キーワードの入力補完

---

## ビルドとインストール

Go 1.21 以降がインストールされていれば、外部ライブラリなしでビルドできます。

### ローカルにバイナリをビルドする場合

```bash
go build -o kvx ./cmd/kvx
```

### `$GOPATH/bin` (または `$GOBIN`) にインストールする場合

```bash
go install ./cmd/kvx
```

### テストの実行

```bash
go test -v ./...
```

---

## エディタの設定例

`kvx` は標準入出力（`stdio`）経由で LSP 通信を行います。

### Neovim (組み込み LSP: `vim.lsp.start`)

`init.lua` 等に以下を追加します（`cmd` のパスはビルドした `kvx` バイナリのパスに合わせて変更してください）。

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "verilog", "systemverilog" },
  callback = function(args)
    vim.lsp.start({
      name = "kvx",
      cmd = { "/path/to/kvx" }, -- PATH が通っている場合は { "kvx" }
      root_dir = vim.fs.root(args.buf, { ".git" }) or vim.fn.getcwd(),
    })
  end,
})
```

### Neovim (`nvim-lspconfig` を使用する場合)

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

汎用の LSP クライアント拡張機能（[vscode-generic-lsp-proxy](https://marketplace.visualstudio.com/items?itemName=ivia.generic-lsp-proxy) など）を使用するか、最小構成のクライアント拡張で以下のように `Executable` に `kvx` を指定します。

```typescript
const serverOptions: ServerOptions = {
  command: "kvx",
  args: [],
  transport: TransportKind.stdio,
};

const clientOptions: LanguageClientOptions = {
  documentSelector: [{ scheme: "file", language: "verilog" }],
};
```
