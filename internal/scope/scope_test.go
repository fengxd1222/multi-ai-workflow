package scope

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

func TestMatch_GlobSemantics(t *testing.T) {
	cases := []struct {
		path, pattern string
		want          bool
	}{
		{"src/auth/service.ts", "src/auth/**", true}, // ** matches descendants
		{"src/auth", "src/auth/**", true},            // p/** also matches p itself
		{"src/a/b.ts", "src/**", true},               // ** crosses dirs
		{"src/a.ts", "src/*", true},                  // * within one segment
		{"src/a/b.ts", "src/*", false},               // * does NOT cross '/'
		{"package.json", "package.json", true},
		{".env", "**/.env", true},        // **/ = zero dirs
		{"config/.env", "**/.env", true}, // **/ = some dirs
		{"a.ts", "src/**", false},
	}
	for _, c := range cases {
		if got := Match(c.path, c.pattern, false); got != c.want {
			t.Errorf("Match(%q,%q)=%v want %v", c.path, c.pattern, got, c.want)
		}
	}
}

func TestClassify_Precedence(t *testing.T) {
	rsv := Reserved{Patterns: []string{"**/.env", ".harness/**"}}

	// case 7: deny beats a wide allow
	if v := Classify("package.json", model.Scope{Allowed: []string{"**"}, Denied: []string{"package.json"}}, rsv, false); v.Decision != DenyScope {
		t.Errorf("deny>allow: got %s", v.Decision)
	}
	// reserved beats a wide allow (no gate)
	if v := Classify(".env", model.Scope{Allowed: []string{"**"}}, rsv, false); v.Decision != DenyReserved {
		t.Errorf("reserved>allow: got %s", v.Decision)
	}
	// allow when matched and not denied/reserved
	if v := Classify("src/auth/x.ts", model.Scope{Allowed: []string{"src/auth/**"}}, rsv, false); v.Decision != Allow {
		t.Errorf("allow: got %s", v.Decision)
	}
	// case 8: default-deny when in neither allow nor deny
	if v := Classify("docs/new.ts", model.Scope{Allowed: []string{"src/**"}}, rsv, false); v.Decision != Gate {
		t.Errorf("default-deny: got %s", v.Decision)
	}
}

func TestClassify_CasefoldDeniedBypass(t *testing.T) {
	sc := model.Scope{Allowed: []string{"src/**"}, Denied: []string{"package.json"}}
	// case 4: on a case-insensitive FS, Package.json must still hit denied.
	if v := Classify("Package.json", sc, Reserved{}, true); v.Decision != DenyScope {
		t.Errorf("casefold deny: got %s", v.Decision)
	}
	// case-sensitive: Package.json is not the denied package.json -> default-deny
	if v := Classify("Package.json", sc, Reserved{}, false); v.Decision != Gate {
		t.Errorf("case-sensitive: got %s", v.Decision)
	}
}

func TestNormalize_RejectsEscapeAndSymlink(t *testing.T) {
	base := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(base, "src", "auth"), 0o755))

	// valid path
	if rel, err := Normalize("src/auth/x.ts", base); err != nil || rel != "src/auth/x.ts" {
		t.Fatalf("valid normalize: rel=%q err=%v", rel, err)
	}

	// in-scope '..' that stays within base resolves normally
	if rel, err := Normalize("src/auth/../service.ts", base); err != nil || rel != "src/service.ts" {
		t.Fatalf("in-base ..: rel=%q err=%v", rel, err)
	}

	// case 5: '../' that escapes above base
	_, err := Normalize("../../etc/passwd", base)
	var re *RejectError
	if !errors.As(err, &re) || re.Reason != "escapes-base-root" {
		t.Fatalf("want escapes-base-root, got %v", err)
	}

	// case 6: symlink in path
	must(t, os.Symlink("/etc", filepath.Join(base, "evil")))
	_, err = Normalize("evil/passwd", base)
	if !errors.As(err, &re) || re.Reason != "symlink-in-path" {
		t.Fatalf("want symlink-in-path, got %v", err)
	}
}

// TestNormalize_SystemSymlinkPrefix reproduces the macOS /var -> /private/var
// case: the base is canonical but a tool reports the path via a symlinked
// prefix. resolvePrefix must canonicalize it instead of falsely rejecting.
func TestNormalize_SystemSymlinkPrefix(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	rel, err := Normalize(filepath.Join(link, "src", "x.ts"), real)
	if err != nil || rel != "src/x.ts" {
		t.Fatalf("symlinked-prefix path: rel=%q err=%v", rel, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
