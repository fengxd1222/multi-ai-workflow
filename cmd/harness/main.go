// Command harness is the v1 multi-agent harness CLI (rev3 §16).
package main

import (
	"fmt"
	"os"
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

	case "guard":
		if len(args) >= 2 && args[1] == "pretool" {
			return report(cli.GuardPretool(cwd, flagValue(args[2:], "--session"),
				flagValue(args[2:], "--job"), flagValue(args[2:], "--runtime"), os.Stdin, os.Stdout))
		}
		if len(args) >= 2 && args[1] == "posttool" {
			return report(cli.GuardPosttool(cwd, flagValue(args[2:], "--session"), flagValue(args[2:], "--job")))
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
		fmt.Println("harness v1 (M1)")
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

func usage() {
	fmt.Fprintln(os.Stderr, `harness v1 — multi-agent CLI harness

usage:
  harness init                 initialize .harness/ in the current git repo
  harness session start        start a session, capture repo baseline
  harness run --job <id>       run a created job to a terminal state
  harness recover [--session]  rebuild views from the event log
  harness version`)
}
