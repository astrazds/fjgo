package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

type Scenario struct {
	Name     string
	Snapshot Snapshot
	Derive   func(Snapshot) View
}

func main() {
	scenarios := representativeStates()
	if len(os.Args) > 1 && os.Args[1] == "--transcript" {
		for i, scenario := range scenarios {
			fmt.Printf("=== %d. %s ===\n", i+1, scenario.Name)
			render(scenario, false)
			fmt.Println()
		}
		return
	}

	reader := bufio.NewReader(os.Stdin)
	selected := 0
	for {
		fmt.Print("\x1b[2J\x1b[H")
		fmt.Printf("%sPROTOTYPE — Forgejo-derived next work%s\n", bold, reset)
		fmt.Printf("%sNo start, checkpoint, advance, or resume state; every line is derived from the displayed Forgejo snapshot.%s\n\n", dim, reset)
		render(scenarios[selected], true)
		fmt.Printf("\n%s[1]%s review blocked  %s[2]%s review-ready  %s[3]%s triage queue  %s[q]%s quit\n> ", bold, reset, bold, reset, bold, reset, bold, reset)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimSpace(line) {
		case "1", "2", "3":
			selected = int(strings.TrimSpace(line)[0] - '1')
		case "q":
			return
		}
	}
}

func render(scenario Scenario, ansi bool) {
	view := scenario.Derive(scenario.Snapshot)
	label := func(value string) string {
		if !ansi {
			return value
		}
		return bold + value + reset
	}
	fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
	if scenario.Name == "Change loop at review-ready boundary" {
		fmt.Printf("%s pull request already exposes passing checks and approval\n", label("Visible state"))
		fmt.Printf("%s bookkeeping — no competing signal or context reconstruction eliminated\n", label("Prototype verdict"))
		fmt.Printf("%s merge remains governed by explicit authorization, not this view\n", label("Boundary"))
		return
	}
	if scenario.Name == "Triage loop with false candidates" {
		fmt.Printf("%s native facts can exclude blocked, unassigned, and already-active issues\n", label("Known"))
		fmt.Printf("%s issue:50 and issue:52 are both assigned, open, and unblocked\n", label("Ambiguity"))
		fmt.Printf("%s arbitrary ranking — Forgejo state contains no policy saying an unread mention outranks existing assigned work\n", label("Prototype verdict"))
		fmt.Printf("%s none defensibly derived; present the eligible queue instead\n", label("Next"))
		return
	}
	fmt.Printf("%s %s\n", label("Position"), view.Position)
	if view.Focus != "" {
		fmt.Printf("%s %s\n", label("Focus"), view.Focus)
	}
	fmt.Printf("%s\n", label("Because"))
	for _, reason := range view.Because {
		fmt.Printf("  - %s\n", reason)
	}
	fmt.Printf("%s %s\n", label("Next"), view.Next)
	fmt.Printf("%s %s\n", label("Command"), view.Command)
	if len(view.Alternates) > 0 {
		fmt.Printf("%s\n", label("Not next"))
		for _, alternate := range view.Alternates {
			fmt.Printf("  - %s\n", alternate)
		}
	}
	fmt.Printf("%s %d Forgejo-native facts; 0 lifecycle records\n", label("Derived from"), view.FactCount)
}

func representativeStates() []Scenario {
	return []Scenario{
		{
			Name: "Change loop with competing blockers",
			Snapshot: Snapshot{
				Actor:    "agent",
				Issues:   []Issue{{Number: 42, Title: "Redact reflected API credentials", State: "open", Assignee: "agent"}},
				Branches: []Branch{{Name: "agent/42-redact-errors", SHA: "8c91d2a9"}},
				PullRequests: []PullRequest{{
					Number: 27, Title: "Redact credentials in API errors", State: "open", HeadBranch: "agent/42-redact-errors", HeadSHA: "8c91d2a9", Closes: 42,
					Checks:   []Check{{Name: "verify", Status: "failure"}, {Name: "lint", Status: "success"}},
					Reviews:  []Review{{ID: 901, Author: "andrejs", State: "REQUEST_CHANGES"}},
					Comments: []Comment{{ReviewID: 901, Author: "andrejs", Body: "redact the response body before wrapping the status", Resolved: false}},
				}},
				Notifications: []Notification{{PullNumber: 27, Reason: "review_requested", Unread: true, Updated: "2026-07-16T05:10:00Z"}},
			},
			Derive: DeriveChange,
		},
		{
			Name: "Change loop at review-ready boundary",
			Snapshot: Snapshot{
				Actor:    "agent",
				Issues:   []Issue{{Number: 42, Title: "Redact reflected API credentials", State: "open", Assignee: "agent"}},
				Branches: []Branch{{Name: "agent/42-redact-errors", SHA: "54f0a19c"}},
				PullRequests: []PullRequest{{
					Number: 27, Title: "Redact credentials in API errors", State: "open", HeadBranch: "agent/42-redact-errors", HeadSHA: "54f0a19c", Closes: 42,
					Checks:  []Check{{Name: "verify", Status: "success"}, {Name: "lint", Status: "success"}},
					Reviews: []Review{{ID: 902, Author: "andrejs", State: "APPROVED"}},
				}},
			},
			Derive: DeriveChange,
		},
		{
			Name: "Triage loop with false candidates",
			Snapshot: Snapshot{
				Actor: "agent",
				Issues: []Issue{
					{Number: 50, Title: "Define v16 capability model", State: "open", Assignee: "agent"},
					{Number: 51, Title: "Upgrade Actions API", State: "open", Assignee: "agent", Dependencies: []int{50}},
					{Number: 52, Title: "Repair stale hook paths", State: "open", Assignee: "agent"},
					{Number: 53, Title: "Investigate cache growth", State: "open"},
					{Number: 54, Title: "Tighten release output", State: "open", Assignee: "agent"},
				},
				Branches:     []Branch{{Name: "agent/54-release-output", SHA: "a61cdb2f"}},
				PullRequests: []PullRequest{{Number: 31, Title: "Tighten release output", State: "open", HeadBranch: "agent/54-release-output", HeadSHA: "a61cdb2f", Closes: 54}},
				Notifications: []Notification{
					{IssueNumber: 52, Reason: "mention", Unread: true, Updated: "2026-07-16T05:15:00Z"},
					{IssueNumber: 53, Reason: "subscribed", Unread: true, Updated: "2026-07-16T05:16:00Z"},
				},
			},
			Derive: DeriveTriage,
		},
	}
}
