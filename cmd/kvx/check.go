package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kvx/internal/analyzer"
	"kvx/internal/ast"
	"kvx/internal/config"
	"kvx/internal/lexer"
	"kvx/internal/parser"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func runCheck(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("kvx check", flag.ContinueOnError)
	fs.SetOutput(out)

	showTokens := fs.Bool("tokens", false, "print lexical tokens")
	showAST := fs.Bool("ast", false, "print parsed AST summary")
	showSemantic := fs.Bool("semantic", false, "run semantic analysis and report warnings")
	var includeDirs stringSliceFlag
	fs.Var(&includeDirs, "I", "add directory to `include search path (may be specified multiple times)")

	fs.Usage = func() {
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  kvx [-]                      Start LSP server over stdio\n")
		fmt.Fprintf(out, "  kvx [check] [flags] <file.v> Check Verilog file(s) for syntax errors\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
	}

	// Allow flags and file arguments in any order (e.g., `kvx a.v -ast` or `kvx -I inc a.v`).
	var flagArgs, fileArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a == "-I" || a == "--I") && i+1 < len(args) {
			flagArgs = append(flagArgs, a, args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(a, "-I") && len(a) > 2 {
			flagArgs = append(flagArgs, "-I", a[2:])
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flagArgs = append(flagArgs, a)
		} else {
			fileArgs = append(fileArgs, a)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	fileArgs = append(fileArgs, fs.Args()...)

	if len(fileArgs) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			if cfg, _, cfgErr := config.FindAndLoad(cwd); cfgErr == nil && cfg != nil && len(cfg.Files) > 0 {
				fileArgs = append(fileArgs, cfg.Files...)
			}
		}
	}

	if len(fileArgs) == 0 {
		fmt.Fprintln(out, "error: no Verilog files specified")
		fs.Usage()
		return 2
	}

	exitCode := 0
	for _, path := range fileArgs {
		if !checkFile(path, includeDirs, *showTokens, *showAST, *showSemantic, out) {
			exitCode = 1
		}
	}
	return exitCode
}

func checkFile(path string, includeDirs []string, showTokens, showAST, showSemantic bool, out io.Writer) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(out, "%s: error reading file: %v\n", path, err)
		return false
	}

	src := string(data)
	lines := strings.Split(src, "\n")

	if showTokens {
		fmt.Fprintf(out, "=== Tokens: %s ===\n", path)
		for _, tok := range lexer.TokenizeWithDirectives(src) {
			fmt.Fprintf(out, "  %d:%d\t%-12s\t%q\n",
				tok.Range.Start.Line+1,
				tok.Range.Start.Column+1,
				tok.Type,
				tok.Literal,
			)
		}
	}

	allIncludeDirs := append([]string(nil), includeDirs...)
	if cfg, cfgPath, cfgErr := config.FindAndLoad(filepath.Dir(path)); cfgErr != nil {
		fmt.Fprintf(out, "warning: failed to load %s: %v\n", cfgPath, cfgErr)
	} else if cfg != nil {
		allIncludeDirs = append(allIncludeDirs, cfg.IncludeDirs...)
	}

	fileAST, parseErrs := parser.ParseWithOptions(src, parser.Options{
		FilePath:    path,
		IncludeDirs: allIncludeDirs,
	})

	if showAST && fileAST != nil {
		printASTSummary(path, fileAST, out)
	}

	if len(parseErrs) > 0 {
		for _, pe := range parseErrs {
			lineNum := pe.Range.Start.Line + 1
			colNum := pe.Range.Start.Column + 1
			fmt.Fprintf(out, "%s:%d:%d: syntax error: %s\n", path, lineNum, colNum, pe.Message)
			if pe.Range.Start.Line >= 0 && pe.Range.Start.Line < len(lines) {
				srcLine := strings.TrimRight(lines[pe.Range.Start.Line], "\r")
				fmt.Fprintf(out, "  %s\n", srcLine)
				fmt.Fprintf(out, "  %s^\n", buildCaretPadding(srcLine, pe.Range.Start.Column))
			}
		}
		fmt.Fprintf(out, "%s: FAILED (%d syntax error(s))\n", path, len(parseErrs))
	} else {
		modCount := 0
		if fileAST != nil {
			modCount = len(fileAST.Modules)
		}
		fmt.Fprintf(out, "%s: OK (0 syntax errors, %d module(s))\n", path, modCount)
	}

	if showSemantic && fileAST != nil {
		an := analyzer.Analyze(fileAST)
		for _, sd := range an.Diagnostics {
			kind := "warning"
			if sd.IsError {
				kind = "error"
			}
			fmt.Fprintf(out, "%s:%d:%d: %s: %s\n",
				path,
				sd.Range.Start.Line+1,
				sd.Range.Start.Column+1,
				kind,
				sd.Message,
			)
		}
	}

	return len(parseErrs) == 0
}

func buildCaretPadding(line string, col int) string {
	var b strings.Builder
	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func printASTSummary(path string, file *ast.SourceFile, out io.Writer) {
	fmt.Fprintf(out, "=== AST: %s ===\n", path)
	for _, mod := range file.Modules {
		fmt.Fprintf(out, "  Module %s (line %d..%d)\n",
			mod.Name, mod.Range.Start.Line+1, mod.Range.End.Line+1)
		for _, item := range mod.Items {
			switch v := item.(type) {
			case *ast.ParamDecl:
				w := ""
				if v.Width != nil {
					w = " " + v.Width.Raw
				}
				fmt.Fprintf(out, "    Param: %s%s %s = %s\n", v.Kind, w, v.Name, v.ValueText)
			case *ast.PortDecl:
				w := ""
				if v.Width != nil {
					w = " " + v.Width.Raw
				}
				nt := ""
				if v.NetType != "" {
					nt = " " + v.NetType
				}
				fmt.Fprintf(out, "    Port:  %s%s%s %s\n", v.Direction, nt, w, v.Name)
			case *ast.NetDecl:
				w := ""
				if v.Width != nil {
					w = " " + v.Width.Raw
				}
				fmt.Fprintf(out, "    Net:   %s%s %s\n", v.Kind, w, v.Name)
			case *ast.AssignStmt:
				fmt.Fprintf(out, "    Assign (line %d, %d refs)\n", v.Range.Start.Line+1, len(v.Refs))
			case *ast.ProceduralBlock:
				lbl := ""
				if v.Label != "" {
					lbl = " :" + v.Label
				}
				fmt.Fprintf(out, "    Block: %s%s (line %d..%d, %d refs)\n",
					v.Kind, lbl, v.Range.Start.Line+1, v.Range.End.Line+1, len(v.Refs))
			case *ast.ModuleInst:
				fmt.Fprintf(out, "    Inst:  %s %s (line %d..%d)\n",
					v.ModuleName, v.InstanceName, v.Range.Start.Line+1, v.Range.End.Line+1)
			case *ast.FunctionDecl:
				fmt.Fprintf(out, "    Func:  %s (line %d..%d, %d params)\n",
					v.Name, v.Range.Start.Line+1, v.Range.End.Line+1, len(v.Params))
			case *ast.GenerateBlock:
				lbl := ""
				if v.Label != "" {
					lbl = " :" + v.Label
				}
				fmt.Fprintf(out, "    Gen:   %s%s (line %d..%d, %d items)\n",
					v.Kind, lbl, v.Range.Start.Line+1, v.Range.End.Line+1, len(v.Items))
			}
		}
	}
}
