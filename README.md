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
- **Semantic Tokens (`textDocument/semanticTokens/full`)**
  - Verilog キーワード、モジュール名、パラメータ、入出力ポート、信号変数、数値・文字列リテラル、システムタスク、マクロ、演算子のセマンティックハイライト

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

## CLI での構文チェック・デバッグ実行

引数なし、または `-`（ハイフン1文字）・`--stdio`・`lsp` を指定して `kvx` を起動すると LSP サーバーモード（`stdio`）になります。ファイルパスを指定するとコマンドラインから Verilog ファイルの構文チェックや AST・トークンの確認ができます。

```bash
# 単一または複数ファイルの構文エラーチェック
./kvx path/to/design.v
./kvx check rtl/*.v

# オプション付き実行（デバッグ・機能追加の検討用）
./kvx -tokens path/to/design.v    # トークン列を表示
./kvx -ast path/to/design.v       # パースされた AST 構造のサマリを表示
./kvx -semantic path/to/design.v  # 未宣言信号・重複宣言などのセマンティック警告も表示
```

出力例（構文エラーがある場合、該当行とカラム位置に `^` が表示され、終了コード `1` を返します）：

```text
broken.v:2:8: syntax error: expected ';' after wire declaration
  wire x
       ^
broken.v: FAILED (1 syntax error(s))
```

構文エラーがない場合（終了コード `0`）：

```text
valid.v: OK (0 syntax errors, 1 module(s))
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

リポジトリ同梱の VS Code 拡張機能（`vscode/`）をビルドして利用できます（Node.js / npm が必要です）。

#### 1. 依存関係のインストールとコンパイル

```bash
cd vscode
npm install
npm run compile
```

- 開発中にファイル変更を監視して自動再コンパイルする場合は `npm run watch`、Lint チェックは `npm run lint` を実行します。
- パッケージ化せず動作確認したい場合は、`vscode/` フォルダを VS Code で開いて `F5` を押すと Extension Development Host で実行できます。

#### 2. `.vsix` パッケージの作成とインストール

```bash
cd vscode
npx @vscode/vsce package
code --install-extension kvx-0.0.1.vsix
```

（VS Code の拡張機能ビューのメニュー「VSIX からインストール...」からも導入できます）

#### 3. VS Code 側の設定

拡張機能をインストールしたら、VS Code の `settings.json` で `kvx.serverPath`（`${workspaceFolder}` プレースホルダに対応）または環境変数 `KVX_SERVER_PATH` にビルドした `kvx` バイナリのパスを指定してください。

```json
{
  "kvx.serverPath": "/path/to/kvx",
  "kvx.serverArgs": []
}
```

---

## プロジェクト設定 (`kvx.json`)

`kvx.json` がない場合でも、LSP 起動時にワークスペース配下の `.v` / `.vh` ディレクトリおよびルート直下の `.f` ファイルを**自動検出（ゼロコンフィグ）**してインデックス化します。

細かい設定やマクロ定義を固定したい場合は、`kvx init` コマンドで `kvx.json` を自動生成できます：

```bash
# プロジェクトを自動スキャンして kvx.json を生成
kvx init

# シミュレータ（iverilog / verilator 等）の引数から kvx.json を生成
kvx init -- -I rtl/include -D SIM -f sim/filelist.f rtl/top.v
```

生成される `kvx.json` の例：

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

- **`files`**: プロジェクト内の Verilog ソースファイル（glob パターン指定可）。LSP 起動時にインデックス化され、開いていない別ファイルのモジュールへの定義ジャンプや Hover・補完が有効になります（`kvx check` を引数なしで実行した際の一括チェック対象にもなります）。
- **`includeDirs`**: `` `include `` で検索するディレクトリのリスト（`kvx.json` からの相対パス、または絶対パス）。
- **`filelists`**: シミュレーション用 filelist（`.f`）のパスを指定すると、ファイル内の `-I <dir>`・`-I<dir>`・`+incdir+<dir1>+<dir2>`・`-D`・`+define+` 指定および記載されている `.v` ファイルパスも自動で抽出します。
- **`defines`**: 事前定義マクロ（`` `define `` 相当）のマップ。値なしのフラグマクロは `""` を指定します（例: `"SIM": ""`）。`` `ifdef `` の条件判定や `` `DATA_WIDTH `` のマクロ展開に反映されます。

### 環境変数による設定ファイルの指定（`KVX_CONFIG`）

環境変数 `KVX_CONFIG` に設定ファイルのパスを指定すると、`kvx.json` の自動探索よりも優先してそのファイルを読み込みます。

```bash
KVX_CONFIG=/path/to/custom_kvx.json kvx check
```

---

## ログ出力（デバッグ用）

LSP モード時のログは標準エラー出力（`stderr`）に出力されます。環境変数 `KVX_LOG` にファイルパスを指定すると、`stderr` に加えてそのファイルにもログを追記します。

```bash
KVX_LOG=/tmp/kvx.log kvx
```
