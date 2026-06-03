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

	case "recover":
		return report(cli.Recover(cwd, sessionFlag(args[1:])))

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

func sessionFlag(args []string) string {
	for i, a := range args {
		if a == "--session" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--session="); ok {
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
  harness recover [--session]  rebuild views from the event log
  harness version`)
}
