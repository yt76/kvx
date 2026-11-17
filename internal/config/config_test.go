package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileAndFilelist(t *testing.T) {
	root := t.TempDir()
	incA := filepath.Join(root, "rtl", "inc")
	incB := filepath.Join(root, "ip", "common")
	incC := filepath.Join(root, "sim", "headers")
	for _, d := range []string{incA, incB, incC} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	flPath := filepath.Join(root, "sim", "files.f")
	flContent := "# Simulator filelist\n" +
		"-I rtl/inc\n" +
		"-Iip/common // inline comment\n" +
		"+incdir+headers+../rtl/inc\n" +
		"../rtl/top.v\n"
	if err := os.WriteFile(flPath, []byte(flContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, ConfigFileName)
	cfgContent := `{
  "includeDirs": ["rtl/inc", "custom/inc"],
  "filelists": ["sim/files.f"]
}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	want := []string{
		incA,
		filepath.Join(root, "custom", "inc"),
		incB,
		incC,
	}
	if len(cfg.IncludeDirs) != len(want) {
		t.Fatalf("got %d includeDirs %v, want %d %v", len(cfg.IncludeDirs), cfg.IncludeDirs, len(want), want)
	}
	for i, w := range want {
		if cfg.IncludeDirs[i] != w {
			t.Errorf("includeDirs[%d] = %q, want %q", i, cfg.IncludeDirs[i], w)
		}
	}
}

func TestFindAndLoad(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "rtl", "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, ConfigFileName)
	if err := os.WriteFile(cfgPath, []byte(`{"includeDirs": ["inc"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, foundPath, err := FindAndLoad(subDir)
	if err != nil {
		t.Fatalf("FindAndLoad error: %v", err)
	}
	if foundPath != cfgPath {
		t.Errorf("foundPath = %q, want %q", foundPath, cfgPath)
	}
	if cfg == nil || len(cfg.IncludeDirs) != 1 || cfg.IncludeDirs[0] != filepath.Join(root, "inc") {
		t.Errorf("unexpected cfg: %+v", cfg)
	}

	// Missing config should return (nil, "", nil).
	emptyDir := t.TempDir()
	cfg2, found2, err2 := FindAndLoad(emptyDir)
	if err2 != nil || cfg2 != nil || found2 != "" {
		t.Errorf("expected (nil, \"\", nil) for missing config, got (%+v, %q, %v)", cfg2, found2, err2)
	}
}
