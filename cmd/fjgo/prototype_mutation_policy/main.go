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
	Name        string
	Description string
	Mutation    Mutation
}

var policy = Policy{
	Name: "review-ready-agent",
	Repo: "astrazds/fjgo",
	Rules: []Rule{
		{Action: "issue.comment", Classification: Allowed, Reason: "comments are allowed inside the delegated repository"},
		{Action: "pr.merge", Classification: ApprovalRequired, Reason: "merge continues beyond the review-ready boundary"},
		{Action: "repo.delete", Classification: Denied, Reason: "repository deletion is outside delegated authority"},
		{Action: "secret.set", Classification: Allowed, Reason: "named CI secret rotation is allowed in this scope"},
	},
}

func main() {
	scenarios := representativeScenarios()
	if len(os.Args) > 1 && os.Args[1] == "--transcript" {
		for i := range scenarios {
			fmt.Printf("=== %d. %s ===\n", i+1, scenarios[i].Name)
			renderScenario(scenarios[i], false)
			fmt.Println()
		}
		return
	}

	reader := bufio.NewReader(os.Stdin)
	selected := 0
	for {
		fmt.Print("\x1b[2J\x1b[H")
		fmt.Printf("%sPROTOTYPE — mutation policy and receipts%s\n", bold, reset)
		fmt.Printf("%sQuestion: can a developer predict the authority and outcome before fjgo mutates Forgejo?%s\n\n", dim, reset)
		renderPolicy()
		fmt.Println()
		renderScenario(scenarios[selected], true)
		fmt.Printf("\n%s[1-5]%s scenario  %s[a]%s grant/clear approval  %s[q]%s quit\n> ", bold, reset, bold, reset, bold, reset)

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimSpace(line) {
		case "1", "2", "3", "4", "5":
			selected = int(strings.TrimSpace(line)[0] - '1')
		case "a":
			mutation := &scenarios[selected].Mutation
			if mutation.Approval == nil {
				mutation.Approval = &Approval{
					ID:              "appr_01JZ8M2H",
					By:              "astrazds",
					Action:          mutation.Action,
					ResourceVersion: mutation.ObservedVersion,
				}
			} else {
				mutation.Approval = nil
			}
		case "q":
			return
		}
	}
}

func representativeScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "Allowed write",
			Description: "Add evidence to an issue in the delegated repository.",
			Mutation: Mutation{
				Action: "issue.comment", Repo: "astrazds/fjgo", Resource: "issue:42",
				Command:         "fjgo -R origin issue comment 42 --body-file finding.md --yes",
				ExpectedVersion: "issue:42@2026-07-16T02:10:00Z", ObservedVersion: "issue:42@2026-07-16T02:10:00Z",
				Result: "created comment:113",
			},
		},
		{
			Name:        "Approval boundary",
			Description: "Merge is beyond the review-ready boundary; press [a] to bind approval to this head.",
			Mutation: Mutation{
				Action: "pr.merge", Repo: "astrazds/fjgo", Resource: "pull:27",
				Command:         "fjgo -R origin pr merge 27 --method squash --yes",
				ExpectedVersion: "pull:27@head:8c91d2a", ObservedVersion: "pull:27@head:8c91d2a",
				Result: "merged pull:27 as 54f0a19",
			},
		},
		{
			Name:        "Denied destructive action",
			Description: "Deleting the repository is outside delegated authority.",
			Mutation: Mutation{
				Action: "repo.delete", Repo: "astrazds/fjgo", Resource: "repo:astrazds/fjgo",
				Command:         "fjgo -R origin api call deleteRepository owner=astrazds repo=fjgo --yes",
				ExpectedVersion: "repo:astrazds/fjgo@13", ObservedVersion: "repo:astrazds/fjgo@13",
				Result: "not executed",
			},
		},
		{
			Name:        "Stale-state conflict",
			Description: "The approved pull request changed after inspection; policy classification does not hide the conflict.",
			Mutation: Mutation{
				Action: "pr.merge", Repo: "astrazds/fjgo", Resource: "pull:27",
				Command:         "fjgo -R origin pr merge 27 --method squash --yes",
				ExpectedVersion: "pull:27@head:8c91d2a", ObservedVersion: "pull:27@head:b771ee0",
				Approval: &Approval{ID: "appr_01JZ8JYQ", By: "astrazds", Action: "pr.merge", ResourceVersion: "pull:27@head:8c91d2a"},
				Result:   "not executed",
			},
		},
		{
			Name:        "Redacted receipt",
			Description: "The receipt proves a named secret changed without retaining its value.",
			Mutation: Mutation{
				Action: "secret.set", Repo: "astrazds/fjgo", Resource: "secret:DEPLOY_TOKEN",
				Command:         "printf '[redacted]' | fjgo -R origin secret set DEPLOY_TOKEN --yes",
				ExpectedVersion: "secret:DEPLOY_TOKEN@absent", ObservedVersion: "secret:DEPLOY_TOKEN@absent",
				ReceiptFields: []Field{{Name: "secret_name", Value: "DEPLOY_TOKEN"}, {Name: "secret_value", Value: "fjgo_pat_live_example"}, {Name: "authorization", Value: "token fjgo_pat_live_example"}},
				Result:        "created secret:DEPLOY_TOKEN",
			},
		},
	}
}

func renderPolicy() {
	fmt.Printf("%sPolicy%s %s  %sScope%s %s\n", bold, reset, policy.Name, bold, reset, policy.Repo)
	for _, rule := range policy.Rules {
		fmt.Printf("  %-16s %s\n", rule.Action, rule.Classification)
	}
	fmt.Println("  unmatched        denied")
}

func renderScenario(scenario Scenario, ansi bool) {
	evaluation := Evaluate(policy, scenario.Mutation)
	label := func(value string) string {
		if !ansi {
			return value
		}
		return bold + value + reset
	}

	if evaluation.Executable {
		fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
		fmt.Printf("%s %s\n", label("Command"), scenario.Mutation.Command)
		fmt.Printf("%s completed\n", label("Outcome"))
		renderReceipt(label, Receipt(policy, scenario.Mutation, evaluation), false)
		return
	}
	if evaluation.Classification == ApprovalRequired && evaluation.Precondition == "current" {
		fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
		fmt.Printf("%s %s\n", label("Command"), scenario.Mutation.Command)
		fmt.Printf("%s\n", label("Approval required"))
		fmt.Printf("  %-16s %s\n", "action", scenario.Mutation.Action)
		fmt.Printf("  %-16s %s\n", "resource", scenario.Mutation.Resource)
		fmt.Printf("  %-16s %s\n", "bound_to", scenario.Mutation.ObservedVersion)
		fmt.Printf("  %-16s %s\n", "reason", evaluation.PolicyReason)
		fmt.Printf("%s fjgo approval grant appr_01JZ8M2H --yes\n", label("Approve"))
		fmt.Printf("%s awaiting-approval\n", label("Outcome"))
		return
	}
	if evaluation.Classification == Denied && evaluation.Precondition == "current" {
		fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
		fmt.Printf("%s %s\n", label("Command"), scenario.Mutation.Command)
		fmt.Printf("%s\n", label("Denied"))
		fmt.Printf("  %-16s %s\n", "action", scenario.Mutation.Action)
		fmt.Printf("  %-16s %s\n", "resource", scenario.Mutation.Resource)
		fmt.Printf("  %-16s %s\n", "reason", evaluation.PolicyReason)
		fmt.Printf("%s request not sent; change policy separately to grant authority\n", label("Outcome"))
		return
	}
	if evaluation.Precondition == "stale" {
		fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
		fmt.Printf("%s %s\n", label("Command"), scenario.Mutation.Command)
		fmt.Printf("%s\n", label("State conflict"))
		fmt.Printf("  %-16s %s\n", "action", scenario.Mutation.Action)
		fmt.Printf("  %-16s %s\n", "resource", scenario.Mutation.Resource)
		fmt.Printf("  %-16s %s\n", "inspected", scenario.Mutation.ExpectedVersion)
		fmt.Printf("  %-16s %s\n", "current", scenario.Mutation.ObservedVersion)
		if scenario.Mutation.Approval != nil {
			fmt.Printf("  %-16s %s invalidated\n", "approval", scenario.Mutation.Approval.ID)
		}
		fmt.Printf("%s request not sent; re-inspect and request fresh approval\n", label("Outcome"))
		return
	}

	fmt.Printf("%s %s\n", label("Scenario"), scenario.Name)
	fmt.Printf("%s %s\n", label("Why"), scenario.Description)
	fmt.Printf("%s %s\n", label("Command"), scenario.Mutation.Command)
	fmt.Printf("%s %s\n", label("Action"), scenario.Mutation.Action)
	fmt.Printf("%s %s\n", label("Resource"), scenario.Mutation.Resource)
	fmt.Printf("%s %s\n", label("Expected"), scenario.Mutation.ExpectedVersion)
	fmt.Printf("%s %s\n", label("Observed"), scenario.Mutation.ObservedVersion)
	fmt.Printf("%s %s\n", label("Classification"), evaluation.Classification)
	fmt.Printf("%s %s (%s)\n", label("Policy reason"), evaluation.PolicyReason, evaluation.Rule)
	fmt.Printf("%s %s\n", label("Precondition"), evaluation.Precondition)
	if scenario.Mutation.Approval == nil {
		fmt.Printf("%s none\n", label("Approval"))
	} else {
		fmt.Printf("%s %s by %s for %s\n", label("Approval"), scenario.Mutation.Approval.ID, scenario.Mutation.Approval.By, scenario.Mutation.Approval.ResourceVersion)
	}
	fmt.Printf("%s %s — %s\n", label("Outcome"), evaluation.Outcome, evaluation.OutcomeReason)

	receipt := Receipt(policy, scenario.Mutation, evaluation)
	if len(receipt) == 0 {
		fmt.Printf("%s not issued\n", label("Receipt"))
		return
	}
	renderReceipt(label, receipt, true)
}

func renderReceipt(label func(string) string, receipt []Field, full bool) {
	fmt.Printf("%s\n", label("Receipt"))
	for _, field := range receipt {
		if !full && field.Name != "receipt" && field.Name != "action" && field.Name != "resource" && field.Name != "approval" && field.Name != "approved_by" && field.Name != "secret_name" && field.Name != "result" {
			continue
		}
		fmt.Printf("  %-16s %s\n", field.Name, field.Value)
	}
}
