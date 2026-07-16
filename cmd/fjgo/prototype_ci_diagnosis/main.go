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

var budget = Budget{MaxSourceBytes: 1400, MaxEvidence: 2, MaxEvidenceText: 180}

func main() {
	fixtures := representativeFailures()
	if len(os.Args) > 1 && os.Args[1] == "--transcript" {
		for i, fixture := range fixtures {
			fmt.Printf("=== %d. %s ===\n", i+1, fixture.Name)
			render(fixture, false)
			fmt.Println()
		}
		return
	}

	reader := bufio.NewReader(os.Stdin)
	selected := 0
	for {
		fmt.Print("\x1b[2J\x1b[H")
		fmt.Printf("%sPROTOTYPE — bounded CI diagnosis and recovery%s\n", bold, reset)
		fmt.Printf("%sQuestion: is one calibrated claim plus bounded evidence and one safe next action better than raw run data?%s\n\n", dim, reset)
		render(fixtures[selected], true)
		fmt.Printf("\n%s[1]%s code/test  %s[2]%s runner/infra  %s[3]%s permission/config  %s[q]%s quit\n> ", bold, reset, bold, reset, bold, reset, bold, reset)
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

func render(fixture Fixture, ansi bool) {
	diagnosis := Diagnose(fixture, budget)
	label := func(value string) string {
		if !ansi {
			return value
		}
		return bold + value + reset
	}
	fmt.Printf("%s %s\n", label("Scenario"), fixture.Name)
	fmt.Printf("%s #%d %s %s %s\n", label("Run"), fixture.Run.Number, fixture.Job.Name, fixture.Job.Status, fixture.Run.SHA)
	if diagnosis.Category == "runner-or-infrastructure" {
		fmt.Printf("%s %s\n", label("Failure"), diagnosis.Claim)
		fmt.Printf("%s [%s] %s\n", label("Next"), diagnosis.NextActionKind, diagnosis.NextAction)
		fmt.Printf("%s bookkeeping — the scheduler already states the cause and recovery boundary\n", label("Prototype verdict"))
		return
	}
	fmt.Printf("%s %s (%s confidence) — %s\n", label("Diagnosis"), diagnosis.Category, diagnosis.Confidence, diagnosis.Claim)
	fmt.Printf("%s\n", label("Evidence"))
	for _, evidence := range diagnosis.Evidence {
		fmt.Printf("  %-24s %s\n", evidence.Source, evidence.Excerpt)
	}
	fmt.Printf("%s %s\n", label("Limit"), diagnosis.Uncertainty)
	fmt.Printf("%s [%s] %s\n", label("Next"), diagnosis.NextActionKind, diagnosis.NextAction)
	fmt.Printf("%s inspected:%d/%d bytes  evidence:%d/%d lines  redactions:%d  truncated:%t\n",
		label("Bounds"), diagnosis.InspectedBytes, diagnosis.InputBytes, diagnosis.EvidenceLines, diagnosis.InputLines, diagnosis.Redactions, diagnosis.Truncated)
}

func representativeFailures() []Fixture {
	return []Fixture{
		{
			Name:        "Code or test failure",
			Description: "Failed unit-test job plus a JUnit artifact.",
			Run:         Run{ID: 184, Number: 61, Workflow: "verify.yml", Ref: "refs/pull/42/head", SHA: "8c91d2a", Status: "failure", Event: "pull_request"},
			Job: Job{
				ID: 501, Name: "unit-tests", Status: "failure", RunsOn: []string{"docker", "linux-x64"},
				Log: `=== RUN   TestClientRedactsToken
    client_test.go:87: expected error body not to contain token=fjgo_pat_live_123; got reflected credential
--- FAIL: TestClientRedactsToken (0.00s)
FAIL repos.astrazds.net/astrazds/fjgo/internal/forgejo
` + strings.Repeat("post-failure cleanup line\n", 80),
			},
			Artifacts: []Artifact{{Name: "test-results.xml", Content: `<testsuite failures="1"><failure message="reflected credential in HTTP error">TestClientRedactsToken</failure></testsuite>`}},
		},
		{
			Name:        "Runner or infrastructure failure",
			Description: "Job stayed queued and produced scheduler evidence but no step logs or artifacts.",
			Run:         Run{ID: 185, Number: 62, Workflow: "verify.yml", Ref: "refs/heads/main", SHA: "b771ee0", Status: "failure", Event: "push"},
			Job: Job{
				ID: 502, Name: "release-linux", Status: "failure", RunsOn: []string{"self-hosted", "linux-x64"},
				Log: `scheduler: requested labels [self-hosted, linux-x64]
scheduler: waiting for runner
scheduler: no online runner matches requested labels [self-hosted, linux-x64]
job failed before first workflow step
`,
			},
		},
		{
			Name:        "Permission or configuration failure",
			Description: "Fork pull-request metadata plus a failed release upload and token-safe request summary.",
			Run:         Run{ID: 186, Number: 63, Workflow: "release.yml", Ref: "refs/pull/77/head", SHA: "54f0a19", Status: "failure", Event: "pull_request", ForkPullRequest: true},
			Job: Job{
				ID: 503, Name: "publish-release", Status: "failure", RunsOn: []string{"docker", "linux-x64"},
				Log: `step: upload release archive
POST /api/v1/repos/astrazds/fjgo/releases/27/assets
Authorization: token fjgo_pat_live_456
response: 403 Forbidden
message: resource not accessible with token
`,
			},
			Artifacts: []Artifact{{Name: "request-summary.txt", Content: "endpoint=/releases/27/assets\nstatus=403 Forbidden\ntoken=fjgo_pat_live_456\n"}},
		},
	}
}
