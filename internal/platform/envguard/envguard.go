// Package envguard protects Gamaj's configuration surface.
//
// Gamaj owns every environment variable it reads, so every name must carry the
// GAMAJ_ prefix. The guard also blocks the configuration names and project
// names that leaked in from the pre-Gamaj (Python/uvicorn) implementation, so
// they can never come back unnoticed.
//
// The guard inspects the runtime configuration surface: Go sources, shell
// scripts, dashboard TypeScript, example env files and documentation. Names
// that belong to a third party (the operating system, the Go toolchain, GitHub
// Actions, the Vite bundler, the MySQL service image) live in AllowedExternal
// because Gamaj does not own them.
package envguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Prefix is the mandatory prefix for every environment variable Gamaj reads.
const Prefix = "GAMAJ_"

// AllowedExternal lists environment names that Gamaj does not own, together
// with the owner that does. They are accepted verbatim.
var AllowedExternal = map[string]bool{
	// Operating system and POSIX shells.
	"PATH": true, "HOME": true, "USER": true, "SHELL": true, "PWD": true,
	"OLDPWD": true, "TMPDIR": true, "TEMP": true, "TMP": true, "TERM": true,
	"LANG": true, "TZ": true, "HOSTNAME": true, "EDITOR": true, "PAGER": true,
	"SUDO_USER": true, "DEBIAN_FRONTEND": true, "COLUMNS": true, "LINES": true,
	"NO_COLOR": true, "IFS": true, "REPLY": true,
	// Language and build toolchains (Go, Node, Vite, MSYS).
	"CGO_ENABLED": true, "GOOS": true, "GOARCH": true, "GOARM": true,
	"GOFLAGS": true, "GOPATH": true, "GOROOT": true, "GOTOOLCHAIN": true,
	"NODE_ENV": true, "BASE_URL": true, "DEV": true, "PROD": true, "MODE": true,
	"SSR": true, "MSYS_NO_PATHCONV": true,
	// CI providers and their tokens.
	"CI": true, "GITHUB_TOKEN": true, "GH_TOKEN": true,
	// Third-party service image contract (the MySQL/MariaDB container names).
	"MYSQL_ROOT_PASSWORD": true, "MYSQL_DATABASE": true, "MYSQL_USER": true,
	"MYSQL_PASSWORD": true,
}

// allowedPrefixes are namespaces owned by other tools.
var allowedPrefixes = []string{"VITE_", "GITHUB_", "NPM_", "npm_", "XDG_", "LC_", "GO"}

// ForbiddenTokens must never appear anywhere in the scanned tree. Each token is
// matched as a whole word, so GAMAJ_XRAY_JSON does not match XRAY_JSON.
var ForbiddenTokens = []string{
	// Pre-Gamaj (Python/uvicorn) configuration names.
	"SQLALCHEMY_DATABASE_URL", "UVICORN_PORT", "UVICORN_HOST", "UVICORN_SSL",
	"XRAY_JSON", "XRAY_EXECUTABLE_PATH", "XRAY_ASSETS_PATH", "XRAY_SUBSCRIPTION",
	"USERS_AUTODELETE_DAYS", "USER_AUTODELETE_INCLUDE_LIMITED",
	"JWT_ACCESS_TOKEN_EXPIRE_MINUTES", "JOB_REVIEW_USERS", "JOB_SEND_NOTIFICATIONS",
	"NUMBER_OF_RECURRENT_NOTIFICATIONS", "RECURRENT_NOTIFICATIONS_TIMEOUT",
	"USERS_LIST_TIMEOUT", "WEBHOOK_ADDRESS", "WEBHOOK_SECRET",
	// Pre-Gamaj tooling.
	"pymysql", "PYTHONPATH", "requirements.txt", "alembic.ini",
	// Projects Gamaj must never be derived from or point at. Third-party
	// installers hosted by the external app catalog, and pinned upstream build
	// dependencies, are intentionally out of scope.
	"rebecca", "3x-ui", "3xui", "vpn-ui", "vpnui",
}

// Kind classifies a guard finding.
type Kind string

const (
	// MissingPrefix means an environment name Gamaj reads lacks the GAMAJ_ prefix.
	MissingPrefix Kind = "missing GAMAJ_ prefix"
	// ForbiddenName means a pre-Gamaj or foreign project name reappeared.
	ForbiddenName Kind = "forbidden legacy name"
)

// Finding is one proven violation.
type Finding struct {
	Path   string
	Line   int
	Name   string
	Kind   Kind
	Source string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s (%s)", f.Path, f.Line, f.Kind, f.Name, f.Source)
}

var (
	goAccessor = regexp.MustCompile(`\b(?:os\.(?:Getenv|LookupEnv|Setenv|Unsetenv)|t\.(?:Setenv|LookupEnv)|getString|getInt|getBool|getCSV|getCSVDefault|firstEnv|envString|envInt|envBool|resolveEnv|getenv|lookup)\(`)
	tsAccessor = regexp.MustCompile(`(?:process|import\.meta)\.env\.([A-Za-z_][A-Za-z0-9_]*)`)

	shellExport = regexp.MustCompile(`(?m)^[ \t]*export[ \t]+([A-Za-z_][A-Za-z0-9_]*)=`)
	// An inline assignment only defines an environment variable when its value
	// is a complete token, so unbalanced quotes (a shell local such as
	// ROOT_DIR="$(cd ...) ) are rejected by the quote-balance check in code.
	shellInline = regexp.MustCompile(`(?m)^[ \t]*([A-Z][A-Z0-9_]{2,})=([^ \t;\r\n]*)[ \t]+[A-Za-z_./$]`)
	// A variable that defaults to itself (`NAME="${NAME:-...}"`) is read from
	// the process environment, so its name must be Gamaj-owned. RE2 cannot
	// compare the two capture groups, so that check happens in code.
	shellSelfDefault = regexp.MustCompile(`([A-Z][A-Z0-9_]{2,})="\$\{([A-Z][A-Z0-9_]{2,}):-`)
	shellAssignment  = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?([A-Z][A-Z0-9_]{2,})=(.*)$`)
	shellUnit        = regexp.MustCompile(`Environment=([A-Za-z_][A-Za-z0-9_]*)=`)
	shellDotenv      = regexp.MustCompile(`(?:set_env_value|upsert_env_assignment|get_env_value|remove_env_assignment)[ \t]+"([A-Za-z_][A-Za-z0-9_]*)"`)
	envFileKey       = regexp.MustCompile(`(?m)^[ \t]*([A-Z][A-Z0-9_]{2,})[ \t]*=`)

	quoted       = regexp.MustCompile(`"([^"]*)"`)
	envNameShape = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,}$`)
)

// skipDirs are generated, vendored or tool-owned directories.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "dist": true,
	"build": true, "coverage": true, ".cache": true, "tmp": true,
	".gradle": true, "target": true, "__pycache__": true,
}

// guardPackagePath is the guard's own directory. Its token list and its fixture
// test quote legacy names on purpose, so the guard does not scan itself.
const guardPackagePath = "internal/platform/envguard/"

// skipFiles are lock files and generated bundles.
var skipFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "package.json": false,
}

// scannedExtensions are the text extensions the guard reads.
var scannedExtensions = map[string]bool{
	".go": true, ".sh": true, ".bash": true, ".env": true, ".example": true,
	".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true,
	".yml": true, ".yaml": true, ".json": true, ".toml": true, ".conf": true,
	".service": true, ".md": true, ".mdx": true, ".html": true, ".ini": true,
	".txt": true,
}

const maxFileSize = 2 << 20 // 2 MiB

// Scan walks root and returns every naming violation it can prove.
func Scan(root string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if skipFiles[entry.Name()] {
			return nil
		}
		if !scannedExtensions[strings.ToLower(filepath.Ext(path))] && !strings.HasSuffix(entry.Name(), ".env.example") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, guardPackagePath) {
			return nil
		}
		findings = append(findings, scanContent(rel, string(content))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

func scanContent(path, content string) []Finding {
	var findings []Finding
	report := func(offset int, name string, kind Kind, source string) {
		findings = append(findings, Finding{
			Path:   path,
			Line:   lineNumber(content, offset),
			Name:   name,
			Kind:   kind,
			Source: source,
		})
	}

	for _, token := range ForbiddenTokens {
		pattern := regexp.MustCompile(`(?:^|[^A-Za-z0-9_])` + regexp.QuoteMeta(token) + `(?:[^A-Za-z0-9_]|$)`)
		for _, match := range pattern.FindAllStringIndex(content, -1) {
			report(match[0], token, ForbiddenName, excerpt(content, match[0]))
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == ".go":
		for _, match := range goAccessor.FindAllStringIndex(content, -1) {
			for _, name := range stringLiteralsInCall(content, match[1]-1) {
				if !envNameShape.MatchString(name) || Allowed(name) {
					continue
				}
				report(match[0], name, MissingPrefix, excerpt(content, match[0]))
			}
		}
	case ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" || ext == ".mjs":
		for _, match := range tsAccessor.FindAllStringSubmatchIndex(content, -1) {
			name := content[match[2]:match[3]]
			if Allowed(name) {
				continue
			}
			report(match[0], name, MissingPrefix, excerpt(content, match[0]))
		}
	case ext == ".sh" || ext == ".bash":
		for _, pattern := range []*regexp.Regexp{shellExport, shellUnit, shellDotenv} {
			for _, match := range pattern.FindAllStringSubmatchIndex(content, -1) {
				name := content[match[2]:match[3]]
				if Allowed(name) {
					continue
				}
				report(match[0], name, MissingPrefix, excerpt(content, match[0]))
			}
		}
		for _, match := range shellInline.FindAllStringSubmatchIndex(content, -1) {
			name := content[match[2]:match[3]]
			value := content[match[4]:match[5]]
			if !balancedQuotes(value) || Allowed(name) {
				continue
			}
			report(match[0], name, MissingPrefix, excerpt(content, match[0]))
		}
		for _, match := range shellSelfDefault.FindAllStringSubmatchIndex(content, -1) {
			name := content[match[2]:match[3]]
			read := content[match[4]:match[5]]
			if name != read || Allowed(read) || !readsProcessEnvironment(content, name) {
				continue
			}
			report(match[0], read, MissingPrefix, excerpt(content, match[0]))
		}
	case ext == ".env" || ext == ".example":
		for _, match := range envFileKey.FindAllStringSubmatchIndex(content, -1) {
			name := content[match[2]:match[3]]
			if Allowed(name) {
				continue
			}
			report(match[0], name, MissingPrefix, excerpt(content, match[0]))
		}
	}
	return findings
}

// readsProcessEnvironment reports whether name reaches the script from the
// process environment. A name only assigned to itself (NAME="${NAME:-...}") is
// an environment input; a name that the script also assigns on its own is a
// shell local.
func readsProcessEnvironment(content, name string) bool {
	self := `"${` + name + `:-`
	for _, match := range shellAssignment.FindAllStringSubmatchIndex(content, -1) {
		if content[match[2]:match[3]] != name {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(content[match[4]:match[5]]), self) {
			return false
		}
	}
	return true
}

// balancedQuotes reports whether every quote opened in value is closed, which
// separates a real NAME=value environment prefix from a shell local assignment.
func balancedQuotes(value string) bool {
	for _, quote := range []byte{'"', '\''} {
		if strings.Count(value, string(quote))%2 != 0 {
			return false
		}
	}
	return true
}

// Allowed reports whether name is either Gamaj-owned or owned by another tool
// that Gamaj cannot rename.
func Allowed(name string) bool {
	if strings.HasPrefix(name, Prefix) {
		return true
	}
	if AllowedExternal[name] {
		return true
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// stringLiteralsInCall returns every non-empty string literal inside the call
// whose opening parenthesis is at openIdx.
func stringLiteralsInCall(content string, openIdx int) []string {
	end := matchingParen(content, openIdx)
	if end < 0 {
		end = len(content)
	}
	call := content[openIdx:end]
	matches := quoted.FindAllStringSubmatch(call, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if value := strings.TrimSpace(match[1]); value != "" {
			names = append(names, value)
		}
	}
	return names
}

// matchingParen returns the index just after the parenthesis that closes the
// call opened at openIdx, skipping string and rune literals.
func matchingParen(content string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(content); i++ {
		switch content[i] {
		case '"', '\'', '`':
			quote := content[i]
			for i++; i < len(content); i++ {
				if content[i] == '\\' {
					i++
					continue
				}
				if content[i] == quote {
					break
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func lineNumber(content string, offset int) int {
	if offset > len(content) {
		offset = len(content)
	}
	return strings.Count(content[:offset], "\n") + 1
}

func excerpt(content string, offset int) string {
	start := strings.LastIndex(content[:offset], "\n") + 1
	end := strings.Index(content[offset:], "\n")
	if end < 0 {
		end = len(content)
	} else {
		end += offset
	}
	line := strings.TrimSpace(content[start:end])
	if len(line) > 120 {
		line = line[:120] + "..."
	}
	return line
}
