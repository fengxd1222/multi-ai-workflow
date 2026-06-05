// Command harness is the v1 multi-agent harness CLI (rev3 §16).
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fengxudong/harness/internal/cli"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return cli.ExitUsage
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	switch args[0] {
	case "init":
		return report(cli.Init(cwd))

	case "session":
		if len(args) >= 2 && args[1] == "start" {
			_, err := cli.SessionStart(cwd)
			return report(err)
		}
		usage()
		return cli.ExitUsage

	case "run":
		jobID := flagValue(args[1:], "--job")
		if jobID == "" {
			usage()
			return cli.ExitUsage
		}
		return report(cli.Run(cwd, flagValue(args[1:], "--session"), jobID))

	case "task":
		if len(args) >= 2 && args[1] == "create" {
			_, err := cli.TaskCreate(cwd, flagValue(args[2:], "--session"),
				flagValue(args[2:], "--title"), flagValues(args[2:], "--accept"),
				atoi(flagValue(args[2:], "--budget")))
			return report(err)
		}
		if len(args) >= 2 && args[1] == "phase" {
			return report(cli.TaskPhase(cwd, flagValue(args[2:], "--session"),
				flagValue(args[2:], "--task"), flagValue(args[2:], "--to")))
		}
		usage()
		return cli.ExitUsage

	case "delegate":
		a := args[1:]
		_, err := cli.Delegate(cwd, flagValue(a, "--session"), cli.DelegateSpec{
			TaskID: flagValue(a, "--task"), Role: flagValue(a, "--role"), Runtime: flagValue(a, "--runtime"),
			Goal:        flagValue(a, "--goal"),
			Brief:       flagValue(a, "--brief"),
			Allowed:     flagValues(a, "--allow"), Denied: flagValues(a, "--deny"),
			Constraints: flagValuesRaw(a, "--constraint"),
			Context:     flagValuesRaw(a, "--context"),
			From:        flagValues(a, "--from"),
			Verify:      flagValuesRaw(a, "--verify"), Depth: atoi(flagValue(a, "--depth")),
		})
		return report(err)

	case "verify":
		a := args[1:]
		return report(cli.VerifyTask(cwd, flagValue(a, "--session"), flagValue(a, "--task"),
			flagValue(a, "--workdir"), flagValuesRaw(a, "--cmd")))

	case "integrate":
		a := args[1:]
		return report(cli.Integrate(cwd, flagValue(a, "--session"), flagValue(a, "--task")))

	case "handoff":
		a := args[1:]
		return report(cli.Handoff(cwd, flagValue(a, "--session"), flagValue(a, "--task")))

	case "gate":
		a := args[2:]
		if len(args) < 2 {
			usage()
			return cli.ExitUsage
		}
		switch args[1] {
		case "list":
			return report(cli.GateList(cwd, flagValue(a, "--session")))
		case "show":
			return report(cli.GateShow(cwd, flagValue(a, "--session"), flagValue(a, "--gate")))
		case "approve":
			return report(cli.GateApprove(cwd, flagValue(a, "--session"),
				flagValue(a, "--gate"), flagValue(a, "--option"), flagValues(a, "--files")))
		case "reject":
			return report(cli.GateReject(cwd, flagValue(a, "--session"), flagValue(a, "--gate")))
		}
		usage()
		return cli.ExitUsage

	case "guard":
		if len(args) >= 2 && args[1] == "pretool" {
			return report(cli.GuardPretool(cwd, flagValue(args[2:], "--repo"), flagValue(args[2:], "--session"),
				flagValue(args[2:], "--job"), flagValue(args[2:], "--runtime"), os.Stdin, os.Stdout))
		}
		if len(args) >= 2 && args[1] == "posttool" {
			return report(cli.GuardPosttool(cwd, flagValue(args[2:], "--repo"), flagValue(args[2:], "--session"), flagValue(args[2:], "--job")))
		}
		usage()
		return cli.ExitUsage

	case "hook":
		if len(args) >= 2 && args[1] == "task-stop" {
			return report(cli.HookTaskStop(cwd, flagValue(args[2:], "--session"),
				flagValue(args[2:], "--role"), flagValue(args[2:], "--job"), flagValue(args[2:], "--task")))
		}
		usage()
		return cli.ExitUsage

	case "recover":
		return report(cli.Recover(cwd, flagValue(args[1:], "--session")))

	case "version", "--version", "-v":
		fmt.Println("harness v1")
		return cli.ExitOK

	default:
		usage()
		return cli.ExitUsage
	}
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return cli.CodeOf(err)
	}
	return cli.ExitOK
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
}

// flagValues collects every occurrence of a repeatable flag, splitting
// comma-separated values (e.g. --allow a,b --allow c -> [a b c]).
func flagValues(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			out = append(out, splitComma(args[i+1])...)
			i++
		} else if v, ok := strings.CutPrefix(args[i], name+"="); ok {
			out = append(out, splitComma(v)...)
		}
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// flagValuesRaw collects every occurrence of a repeatable flag WITHOUT splitting
// on commas — for whole-value flags like --verify/--constraint whose values are
// shell commands or sentences that legitimately contain commas.
func flagValuesRaw(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			out = append(out, args[i+1])
			i++
		} else if v, ok := strings.CutPrefix(args[i], name+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func usage() {
	fmt.Fprintln(os.Stderr, `harness v1 — multi-agent CLI harness

usage:
  harness init                 initialize .harness/ in the current git repo
  harness session start        start a session, capture repo baseline
  harness run --job <id>       run a created job to a terminal state
  harness recover [--session]  rebuild views from the event log
  harness version`)
}
