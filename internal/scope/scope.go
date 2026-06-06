// Package scope implements the single deterministic scope-evaluation algorithm
// shared by the PreToolUse path guard and the PostToolUse / migration-point diff
// review (scope-eval.md, rev3 §9). Zero LLM involvement; worker self-reports are
// never consulted here.
package scope

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

// Decision is the outcome of classifying one path.
type Decision int

const (
	Allow        Decision = iota // in scope.allowed
	DenyScope                    // hit scope.denied
	DenyReserved                 // hit the reserved hard-deny layer (no gate)
	Gate                         // default-deny: not in allowed -> needs-human
	Reject                       // normalization rejected (symlink/escape)
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case DenyScope:
		return "deny-scope"
	case DenyReserved:
		return "deny-reserved"
	case Gate:
		return "gate"
	case Reject:
		return "reject"
	}
	return "unknown"
}

// Reserved mirrors reserved.json: the highest-priority hard-deny layer (N15/N27).
type Reserved struct {
	Patterns             []string `json:"patterns"`
	PreExistingUntracked []string `json:"pre_existing_untracked"`
	PreExistingIgnored   []string `json:"pre_existing_ignored"`
	ReservedByHuman      []string `json:"reserved_by_human"`
}

func (r Reserved) all() []string {
	out := make([]string, 0, len(r.Patterns)+len(r.PreExistingUntracked)+len(r.PreExistingIgnored)+len(r.ReservedByHuman))
	out = append(out, r.Patterns...)
	out = append(out, r.PreExistingUntracked...)
	out = append(out, r.PreExistingIgnored...)
	out = append(out, r.ReservedByHuman...)
	return out
}

// Verdict is the classification of one path.
type Verdict struct {
	Path     string // workdir-relative, slash-separated
	Decision Decision
	Rule     string // the glob that matched (empty for Gate)
}

// RejectError is returned by Normalize for paths that escape the base root or
// traverse a symlink (rev3 N25).
type RejectError struct {
	Reason string
	Path   string
}

func (e *RejectError) Error() string { return e.Reason + ": " + e.Path }

// Normalize resolves raw against baseRoot and returns a clean workdir-relative
// slash path. It rejects '..' escapes and any path whose ancestors traverse a
// symlink, closing the predominant scope-bypass channels (rev3 N25).
func Normalize(raw, baseRoot string) (string, error) {
	realBase, err := filepath.EvalSymlinks(baseRoot)
	if err != nil {
		return "", err
	}

	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(realBase, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve symlinks in abs's existing prefix. This canonicalizes system
	// symlinks above the workdir (e.g. macOS /var -> /private/var) AND follows any
	// worker-created symlink to its real target, so a redirect outside scope shows
	// up as an escape (rev3 N25).
	resolved := resolvePrefix(abs)
	rel, err := filepath.Rel(realBase, resolved)
	if err != nil || escapesBase(rel) {
		// If the path was within the base before resolution but escapes after, a
		// symlink in the path redirected it out.
		if rawRel, e := filepath.Rel(realBase, abs); e == nil && !escapesBase(rawRel) {
			return "", &RejectError{Reason: "symlink-in-path", Path: raw}
		}
		return "", &RejectError{Reason: "escapes-base-root", Path: raw}
	}
	return filepath.ToSlash(rel), nil
}

func escapesBase(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolvePrefix resolves symlinks in the longest existing ancestor of abs and
// re-appends the non-existent tail (the file about to be written).
func resolvePrefix(abs string) string {
	p := abs
	var tail []string
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				r = filepath.Join(r, tail[i])
			}
			return r
		}
		parent := filepath.Dir(p)
		if parent == p {
			return abs
		}
		tail = append(tail, filepath.Base(p))
		p = parent
	}
}

// Classify applies the fixed precedence reserved > denied > allowed >
// default-deny (rev3 §9 N24). ignorecase casefolds both sides for
// case-insensitive filesystems (N32).
func Classify(rel string, sc model.Scope, rsv Reserved, ignorecase bool) Verdict {
	if g, ok := matchAny(rel, rsv.all(), ignorecase); ok {
		return Verdict{Path: rel, Decision: DenyReserved, Rule: g}
	}
	if g, ok := matchAny(rel, sc.Denied, ignorecase); ok {
		return Verdict{Path: rel, Decision: DenyScope, Rule: g}
	}
	if g, ok := matchAny(rel, sc.Allowed, ignorecase); ok {
		return Verdict{Path: rel, Decision: Allow, Rule: g}
	}
	return Verdict{Path: rel, Decision: Gate}
}

func matchAny(path string, patterns []string, ignorecase bool) (string, bool) {
	for _, p := range patterns {
		if Match(path, p, ignorecase) {
			return p, true
		}
	}
	return "", false
}

// Match reports whether a slash path matches a gitignore/minimatch-style glob:
// '**' crosses directories, '*' does not cross '/', and a trailing '/**' also
// matches the prefix directory itself (rev3 §9 glob semantics).
func Match(path, pattern string, ignorecase bool) bool {
	if ignorecase {
		path = strings.ToLower(path)
		pattern = strings.ToLower(pattern)
	}
	return compile(pattern).MatchString(path)
}

var reCache sync.Map // pattern -> *regexp.Regexp

func compile(pattern string) *regexp.Regexp {
	if v, ok := reCache.Load(pattern); ok {
		return v.(*regexp.Regexp)
	}
	suffix := "$"
	p := pattern
	if strings.HasSuffix(p, "/**") { // p/** also matches p itself
		p = p[:len(p)-3]
		suffix = "(?:/.*)?$"
	}

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*':
			if i+1 < len(p) && p[i+1] == '*' {
				i++
				if i+1 < len(p) && p[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?") // **/ = zero or more dirs
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*") // * does not cross '/'
			}
		case c == '?':
			b.WriteString("[^/]")
		case strings.IndexByte(`.+()|^$\{}[]`, c) >= 0:
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString(suffix)
	re := regexp.MustCompile(b.String())
	reCache.Store(pattern, re)
	return re
}
