package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kvx/internal/parser"
)

type regressionCase struct {
	line            int
	file            string
	wantParseErrs   int
	wantAnalyzeErrs int
}

func loadRegressionCases(manifestPath string) ([]regressionCase, error) {
	f, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []regressionCase
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s:%d: expected 3 fields (<file> <parser_errors> <analyzer_errors>), got %d", manifestPath, lineNum, len(fields))
		}
		wantParse, err := strconv.Atoi(fields[1])
		if err != nil || wantParse < 0 {
			return nil, fmt.Errorf("%s:%d: invalid parser error count %q", manifestPath, lineNum, fields[1])
		}
		wantAnalyze, err := strconv.Atoi(fields[2])
		if err != nil || wantAnalyze < 0 {
			return nil, fmt.Errorf("%s:%d: invalid analyzer error count %q", manifestPath, lineNum, fields[2])
		}
		cases = append(cases, regressionCase{
			line:            lineNum,
			file:            fields[0],
			wantParseErrs:   wantParse,
			wantAnalyzeErrs: wantAnalyze,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

func TestRegressionFiles(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")
	manifestPath := filepath.Join(testdataDir, "cases.txt")

	cases, err := loadRegressionCases(manifestPath)
	if err != nil {
		t.Fatalf("failed to load %s: %v", manifestPath, err)
	}
	if len(cases) == 0 {
		t.Fatalf("no test cases found in %s", manifestPath)
	}

	listed := make(map[string]bool, len(cases))
	for _, tc := range cases {
		listed[tc.file] = true
		t.Run(tc.file, func(t *testing.T) {
			verilogPath := filepath.Join(testdataDir, tc.file)
			src, err := os.ReadFile(verilogPath)
			if err != nil {
				t.Fatalf("read %s: %v", verilogPath, err)
			}

			file, parseErrs := parser.ParseWithOptions(string(src), parser.Options{
				FilePath:    verilogPath,
				IncludeDirs: []string{testdataDir},
			})
			if len(parseErrs) != tc.wantParseErrs {
				t.Errorf("parser errors = %d, want %d: %v", len(parseErrs), tc.wantParseErrs, parseErrs)
			}

			an := Analyze(file)
			if len(an.Diagnostics) != tc.wantAnalyzeErrs {
				t.Errorf("analyzer diagnostics = %d, want %d: %+v", len(an.Diagnostics), tc.wantAnalyzeErrs, an.Diagnostics)
			}
		})
	}

	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", testdataDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if (ext == ".v" || ext == ".vh" || ext == ".sv" || ext == ".svh") && !listed[entry.Name()] {
			t.Errorf("unlisted testdata file %q (add it to %s)", entry.Name(), manifestPath)
		}
	}
}
