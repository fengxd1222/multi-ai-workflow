// Package guard implements the PreToolUse / PostToolUse safety gates over the
// scope-evaluation algorithm (rev3 §10, §12). Irreversible side-effects are
// hard-denied at PreToolUse — never downgraded to a post-hoc diff that cannot
// undo a network call or a git push (N26).
package guard

import "strings"

// CmdDecision is the verdict for a shell command.
type CmdDecision int

const (
	CmdAllow CmdDecision = iota
	CmdDeny              // irreversible side-effect: block before it runs
	CmdGate              // can't statically prove safety: needs-human
)

func (d CmdDecision) String() string {
	switch d {
	case CmdDeny:
		return "deny"
	case CmdGate:
		return "gate"
	default:
		return "allow"
	}
}

// command-word sets for the first token of each pipeline segment.
var (
	networkCmds = set("curl", "wget", "nc", "ncat", "scp", "sftp", "ssh", "telnet", "ftp", "rsync")
	installPkg  = set("npm", "yarn", "pnpm", "pip", "pip3", "cargo", "gem", "apt", "apt-get", "brew", "go")
	destructive = set("rm", "chmod", "chown", "mkfs", "dd", "shred", "truncate")
)

// ClassifyCommand inspects a shell command for irreversible side-effects
// (rev3 §12 N26). Obfuscated / unparseable commands are gated rather than
// guessed.
func ClassifyCommand(cmd string) (CmdDecision, string) {
	if hasObfuscation(cmd) {
		return CmdGate, "obfuscated-or-unparseable"
	}
	if d, r := redirectToProtected(cmd); d != CmdAllow {
		return d, r
	}
	for _, seg := range splitSegments(cmd) {
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "sudo" && len(fields) > 1 {
			fields = fields[1:]
		}
		if d, r := classifySegment(fields); d != CmdAllow {
			return d, r
		}
	}
	return CmdAllow, ""
}

func classifySegment(fields []string) (CmdDecision, string) {
	w := fields[0]
	switch {
	case networkCmds[w]:
		return CmdDeny, "network-egress"
	case w == "git":
		return classifyGit(fields)
	case installPkg[w]:
		if hasInstallVerb(fields) {
			return CmdDeny, "package-install"
		}
	case destructive[w]:
		return CmdDeny, "destructive"
	}
	return CmdAllow, ""
}

func classifyGit(fields []string) (CmdDecision, string) {
	for _, f := range fields[1:] {
		switch f {
		case "push", "remote":
			return CmdDeny, "git-push"
		case "clean":
			return CmdDeny, "destructive"
		}
		if f == "reset" && contains(fields, "--hard") {
			return CmdDeny, "destructive"
		}
	}
	// `git add .` / `git commit -a` cross-scope staging -> gate
	if contains(fields, "add") && (contains(fields, ".") || contains(fields, "-A") || contains(fields, "--all")) {
		return CmdGate, "broad-git-add"
	}
	if contains(fields, "commit") && (contains(fields, "-a") || contains(fields, "-am")) {
		return CmdGate, "git-commit-all"
	}
	return CmdAllow, ""
}

func hasInstallVerb(fields []string) bool {
	for _, f := range fields[1:] {
		switch f {
		case "install", "i", "ci", "add", "get":
			return true
		}
	}
	return false
}

func hasObfuscation(cmd string) bool {
	if strings.Contains(cmd, "$(") || strings.Contains(cmd, "`") {
		return true
	}
	lc := strings.ToLower(cmd)
	for _, s := range []string{"eval ", "base64 -d", "base64 --decode", "| sh", "|sh", "| bash", "|bash"} {
		if strings.Contains(lc, s) {
			return true
		}
	}
	return false
}

// redirectToProtected flags `> path` / `>> path` whose target is a protected
// tree (.git/.harness/.worktrees).
func redirectToProtected(cmd string) (CmdDecision, string) {
	toks := strings.Fields(cmd)
	for i, t := range toks {
		if (t == ">" || t == ">>") && i+1 < len(toks) {
			if isProtected(toks[i+1]) {
				return CmdDeny, "writes-protected"
			}
		}
		// attached form: >path
		if strings.HasPrefix(t, ">") && len(t) > 1 && isProtected(strings.TrimLeft(t, ">")) {
			return CmdDeny, "writes-protected"
		}
	}
	return CmdAllow, ""
}

func isProtected(p string) bool {
	p = strings.TrimPrefix(p, "./")
	return p == ".git" || strings.HasPrefix(p, ".git/") ||
		p == ".harness" || strings.HasPrefix(p, ".harness/") ||
		p == ".worktrees" || strings.HasPrefix(p, ".worktrees/")
}

// splitSegments breaks a command on shell separators ; | & && ||.
func splitSegments(cmd string) []string {
	repl := strings.NewReplacer("&&", "\n", "||", "\n", "|", "\n", ";", "\n", "&", "\n")
	return strings.Split(repl.Replace(cmd), "\n")
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
