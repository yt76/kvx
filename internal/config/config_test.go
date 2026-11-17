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
		"-D SIM_FLAG\n" +
		"+define+WIDTH=8+EXTRA=1\n" +
		"../rtl/top.v\n"
	if err := os.WriteFile(flPath, []byte(flContent), 0644); err != nil {
		t.Fatal(err)
	}

	subV := filepath.Join(root, "rtl", "sub.v")
	topV := filepath.Join(root, "rtl", "top.v")
	for _, f := range []string{subV, topV} {
		if err := os.WriteFile(f, []byte("module m; endmodule\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfgPath := filepath.Join(root, ConfigFileName)
	cfgContent := `{
  "files": ["rtl/sub.v", "rtl/*.v"],
  "includeDirs": ["rtl/inc", "custom/inc"],
  "filelists": ["sim/files.f"],
  "defines": {
    "WIDTH": "16",
    "CUSTOM": "8'hAA"
  }
}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	wantDirs := []string{
		incA,
		filepath.Join(root, "custom", "inc"),
		incB,
		incC,
	}
	if len(cfg.IncludeDirs) != len(wantDirs) {
		t.Fatalf("got %d includeDirs %v, want %d %v", len(cfg.IncludeDirs), cfg.IncludeDirs, len(wantDirs), wantDirs)
	}
	for i, w := range wantDirs {
		if cfg.IncludeDirs[i] != w {
			t.Errorf("includeDirs[%d] = %q, want %q", i, cfg.IncludeDirs[i], w)
		}
	}

	wantFiles := []string{subV, topV}
	if len(cfg.Files) != len(wantFiles) {
		t.Fatalf("got %d files %v, want %d %v", len(cfg.Files), cfg.Files, len(wantFiles), wantFiles)
	}
	for i, w := range wantFiles {
		if cfg.Files[i] != w {
			t.Errorf("files[%d] = %q, want %q", i, cfg.Files[i], w)
		}
	}

	wantDefines := map[string]string{
		"SIM_FLAG": "",
		"WIDTH":    "16", // kvx.json overrides filelist's WIDTH=8
		"EXTRA":    "1",
		"CUSTOM":   "8'hAA",
	}
	if len(cfg.Defines) != len(wantDefines) {
		t.Fatalf("got defines %v, want %v", cfg.Defines, wantDefines)
	}
	for k, w := range wantDefines {
		if got, ok := cfg.Defines[k]; !ok || got != w {
			t.Errorf("defines[%q] = (%q, %v), want (%q, true)", k, got, ok, w)
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

func TestEnvOverrides(t *testing.T) {
	t.Run("KVX_CONFIG overrides kvx.json discovery", func(t *testing.T) {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, ConfigFileName), []byte(`{"includeDirs": ["default_inc"]}`), 0644)

		altDir := t.TempDir()
		altCfg := filepath.Join(altDir, "custom.json")
		if err := os.WriteFile(altCfg, []byte(`{"includeDirs": ["alt_inc"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		t.Setenv(EnvConfig, altCfg)

		cfg, found, err := FindAndLoad(root)
		if err != nil {
			t.Fatalf("FindAndLoad error: %v", err)
		}
		if found != altCfg {
			t.Errorf("found = %q, want %q", found, altCfg)
		}
		wantInc := filepath.Join(altDir, "alt_inc")
		if len(cfg.IncludeDirs) != 1 || cfg.IncludeDirs[0] != wantInc {
			t.Errorf("IncludeDirs = %v, want [%q]", cfg.IncludeDirs, wantInc)
		}
	})
}
