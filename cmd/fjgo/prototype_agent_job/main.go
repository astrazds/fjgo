// PROTOTYPE: throwaway terminal driver for the agent-job lifecycle state model.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"repos.astrazds.net/astrazds/fjgo/cmd/fjgo/prototype_agent_job/model"
)

const (
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

func main() {
	state := model.Initial()
	in := bufio.NewScanner(os.Stdin)
	for {
		render(state)
		if !in.Scan() {
			return
		}
		switch strings.TrimSpace(in.Text()) {
		case "1":
			state = model.Reduce(state, model.StartChange)
		case "2":
			state = model.Reduce(state, model.StartTriage)
		case "a":
			state = model.Reduce(state, model.Advance)
		case "c":
			state = model.Reduce(state, model.Checkpoint)
		case "r":
			state = model.Reduce(state, model.Resume)
		case "q":
			return
		default:
			state.Notice = "Use one of the listed keys"
		}
	}
}

func render(s model.State) {
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Println(bold + "PROTOTYPE — Forgejo-backed agent job lifecycle" + reset)
	fmt.Println(dim + "Question: does explicit durable state reduce manual steering across sessions?" + reset)
	fmt.Println()
	field("job", empty(s.ID))
	field("kind", string(s.Kind))
	field("goal", empty(s.Goal))
	field("session", fmt.Sprintf("%d", s.Session))
	field("status", s.Status)
	field("phase", fmt.Sprintf("%d", s.Phase))
	field("next action", empty(s.NextAction))
	checkpoint := "none"
	if s.Checkpoint != nil {
		checkpoint = fmt.Sprintf("%d (phase %d, %d evidence items)", s.CheckpointSeq, s.Checkpoint.Phase, len(s.Checkpoint.Evidence))
	}
	field("durable checkpoint", checkpoint)

	fmt.Println()
	section("Evidence")
	list(s.Evidence)
	section("Forgejo dispositions")
	list(s.Disposition)
	section("Recent receipt entries")
	start := len(s.Receipts) - 4
	if start < 0 {
		start = 0
	}
	if len(s.Receipts) == 0 {
		fmt.Println("  " + dim + "(none)" + reset)
	}
	for _, entry := range s.Receipts[start:] {
		fmt.Printf("  %sS%d%s  %s — %s\n", dim, entry.Session, reset, entry.Action, empty(entry.Result))
	}

	fmt.Println()
	fmt.Println(bold+"Notice:"+reset, s.Notice)
	fmt.Println()
	fmt.Println(bold+"[1]"+reset, dim+"start change"+reset, " ", bold+"[2]"+reset, dim+"start triage"+reset)
	fmt.Println(bold+"[a]"+reset, dim+"advance live work"+reset, " ", bold+"[c]"+reset, dim+"checkpoint"+reset, " ", bold+"[r]"+reset, dim+"resume new session"+reset, " ", bold+"[q]"+reset, dim+"quit"+reset)
	fmt.Print("\n> ")
}

func field(name, value string) {
	fmt.Printf("%s%-20s%s %s\n", bold, name+":", reset, value)
}

func section(name string) {
	fmt.Println(bold + name + ":" + reset)
}

func list(items []string) {
	if len(items) == 0 {
		fmt.Println("  " + dim + "(none)" + reset)
		return
	}
	for _, item := range items {
		fmt.Println("  -", item)
	}
}

func empty(value string) string {
	if value == "" {
		return dim + "(none)" + reset
	}
	return value
}
