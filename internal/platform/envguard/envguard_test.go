package envguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentNamesAreGamajOwned(t *testing.T) {
	root := moduleRoot(t)
	findings, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		return
	}
	var report strings.Builder
	report.WriteString("environment naming guard failed:\n")
	for _, finding := range findings {
		report.WriteString("  " + finding.String() + "\n")
	}
	report.WriteString("Every name Gamaj reads must start with GAMAJ_. Rename the variable instead of adding an exception.")
	t.Fatal(report.String())
}

func TestGuardFlagsMissingPrefixAndLegacyNames(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"config.go":      "package config\n\nimport \"os\"\n\nfunc Load() string {\n\treturn os.Getenv(\"SERVICE_PORT\")\n}\n",
		"config_test.go": "package config\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {\n\tt.Setenv(\"GAMAJ_OK\", \"1\")\n}\n",
		"install.sh":     "#!/usr/bin/env bash\nexport LEGACY_MODE=1\nprintenv GAMAJ_OK\n",
		"notes.md":       "The old panel exposed SQLALCHEMY_DATABASE_URL and rebecca dashboards.\n",
		"web.ts":         "const base = import.meta.env.VITE_BASE_API;\nconst token = process.env.LEGACY_TOKEN;\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	findings, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]Kind{
		"SERVICE_PORT":            MissingPrefix,
		"LEGACY_MODE":             MissingPrefix,
		"LEGACY_TOKEN":            MissingPrefix,
		"SQLALCHEMY_DATABASE_URL": ForbiddenName,
		"rebecca":                 ForbiddenName,
	}
	got := map[string]Kind{}
	for _, finding := range findings {
		got[finding.Name] = finding.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Fatalf("finding for %q = %q, want %q (all findings: %#v)", name, got[name], kind, findings)
		}
	}
	for name := range got {
		if !Allowed(name) && want[name] == "" {
			t.Fatalf("unexpected finding for %q", name)
		}
	}
}

func TestAllowedNames(t *testing.T) {
	allowed := []string{"GAMAJ_PORT", "PATH", "GITHUB_TOKEN", "VITE_BASE_API", "MYSQL_ROOT_PASSWORD", "CGO_ENABLED"}
	for _, name := range allowed {
		if !Allowed(name) {
			t.Fatalf("Allowed(%q) = false, want true", name)
		}
	}
	rejected := []string{"SERVICE_PORT", "DATABASE_URL", "DEBUG", "UVICORN_PORT", "XRAY_CORE_VERSION"}
	for _, name := range rejected {
		if Allowed(name) {
			t.Fatalf("Allowed(%q) = true, want false", name)
		}
	}
}

// moduleRoot walks up from the test working directory to the Go module root so
// the guard always scans the whole repository, not just this package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate the Go module root")
		}
		dir = parent
	}
}
