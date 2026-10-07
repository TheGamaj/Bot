// Package routeguard fails the build when an HTTP handler is defined and never
// wired to a router.
//
// The Go compiler will not catch this. It rejects an unused local variable and
// an unused import, but a method that nobody calls is perfectly legal Go: it
// compiles, it passes vet, and it never runs. In Gamaj that is not a cosmetic
// problem. The whole bot sales API once shipped that way - every handler
// written, reviewed and tested in isolation, and none of them registered, so
// /api/bot/plans answered 404 for every caller while every suite in the
// workflow stayed green.
//
// The check is deliberately narrow. It does not try to prove that a route is
// reachable, because that needs a running server. It proves the narrower,
// sufficient thing: that every handler in a package is referenced somewhere
// in non-test code. A handler with no reference is dead, and a dead handler
// is the failure mode that actually happened.
//
// References from _test.go files are not counted. A handler exercised only by
// a test that never reaches the router is exactly the bug this exists for, so
// counting it would hide the defect rather than report it.
package routeguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// handlerPrefix is the naming convention every HTTP handler in Gamaj follows.
// Matching on the prefix rather than on the http.HandlerFunc signature keeps
// the rule readable at the call site: a function called handle* is a handler,
// and if nothing calls it, that is a mistake worth failing over.
const handlerPrefix = "handle"

// guardedDirs are the packages this guard reads, relative to this test's own
// directory. GAMAJ_ROUTE_DIRS overrides them so the same file can be vendored
// into another repository and pointed at that repository's own packages.
//
// Only non-test files are parsed, so the walk stops at the package directory
// and ignores subdirectories entirely.
var guardedDirs = []string{
	"../../web",
	"../../botpanel",
}

// declaration is one handler found on disk.
type declaration struct {
	name string
	file string
	line int
}

// declared is every handler defined in the package, keyed by name.
func declared(t *testing.T, dir string) map[string]declaration {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	found := map[string]declaration{}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, handlerPrefix) {
					continue
				}
				pos := fset.Position(fn.Pos())
				found[fn.Name.Name] = declaration{
					name: fn.Name.Name,
					file: pos.Filename,
					line: pos.Line,
				}
			}
		}
	}
	return found
}

// referenced is every identifier that appears in non-test code.
//
// Inspecting each function body rather than the whole file is what keeps a
// declaration from counting as its own reference: a FuncDecl's name lives
// outside its body. It also means a parameter that happens to be named
// handleSomething cannot mask a real dead handler.
func referenced(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	names := map[string]bool{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.SelectorExpr:
						// s.handleFoo - the ordinary registration.
						names[node.Sel.Name] = true
					case *ast.Ident:
						names[node.Name] = true
					}
					return true
				})
			}
		}
	}
	return names
}

// TestNoUnreferencedHandlers is the build failure.
func TestNoUnreferencedHandlers(t *testing.T) {
	dirs := guardedDirs
	if raw := strings.TrimSpace(os.Getenv("GAMAJ_ROUTE_DIRS")); raw != "" {
		dirs = nil
		for _, entry := range strings.Split(raw, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				dirs = append(dirs, entry)
			}
		}
	}

	orphans := []string{}
	checked := 0
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("resolve %s: %v", dir, err)
		}
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("guarded directory %s does not exist: %v", dir, err)
		}
		decls := declared(t, abs)
		refs := referenced(t, abs)
		names := make([]string, 0, len(decls))
		for name := range decls {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			checked++
			if refs[name] {
				continue
			}
			d := decls[name]
			rel, relErr := filepath.Rel(mustwd(t), d.file)
			if relErr != nil {
				rel = d.file
			}
			orphans = append(orphans, rel+":"+itoa(d.line)+" -> "+name)
		}
	}

	if len(orphans) > 0 {
		sort.Strings(orphans)
		for _, orphan := range orphans {
			t.Errorf("handler defined but never referenced: %s", orphan)
		}
		t.Errorf(
			"\nEvery handler must be reachable from a router. A handler nothing\n" +
				"references is dead code that still compiles, still passes vet,\n" +
				"and still answers 404 for every caller - which is how the whole\n" +
				"bot sales API once shipped. Register it, or delete it. If a\n" +
				"handler is genuinely reached only through an interface or\n" +
				"reflection, name it in routeguardExempt with the reason.",
		)
		return
	}
	t.Logf("routeguard: %d handlers across %d package(s), all referenced", checked, len(dirs))
}

// routeguardExempt names handlers the check must not report.
//
// It is empty on purpose. An entry is a claim that a handler is unreachable
// from the router and that this is intended, which is a thing to argue for in
// review rather than to accumulate quietly. Format: handler name, then why.
var routeguardExempt = map[string]string{}

func mustwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	return wd
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
