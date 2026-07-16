package main

import (
	"fmt"
	"regexp"
	"strings"
)

type Run struct {
	ID              int64
	Number          int64
	Workflow        string
	Ref             string
	SHA             string
	Status          string
	Event           string
	ForkPullRequest bool
}

type Job struct {
	ID     int64
	Name   string
	Status string
	RunsOn []string
	Log    string
}

type Artifact struct {
	Name    string
	Content string
}

type Fixture struct {
	Name        string
	Description string
	Run         Run
	Job         Job
	Artifacts   []Artifact
}

type Budget struct {
	MaxSourceBytes  int
	MaxEvidence     int
	MaxEvidenceText int
}

type Evidence struct {
	Source  string
	Excerpt string
}

type Diagnosis struct {
	Category       string
	Confidence     string
	Claim          string
	Uncertainty    string
	Evidence       []Evidence
	NextAction     string
	NextActionKind string
	NextActionWhy  string
	InputBytes     int
	InspectedBytes int
	InputLines     int
	EvidenceLines  int
	Redactions     int
	Truncated      bool
}

type source struct {
	name string
	text string
}

var (
	authorizationPattern = regexp.MustCompile(`(?i)(authorization:\s*(?:token|bearer)\s+)\S+`)
	secretPattern        = regexp.MustCompile(`(?i)((?:token|secret|password)=)\S+`)
)

// Diagnose is a pure, deliberately narrow evaluator over bounded fixture
// inputs. It chooses one claim, a tiny evidence packet, and one read-only or
// local next action; it does not execute recovery.
func Diagnose(fixture Fixture, budget Budget) Diagnosis {
	sources := []source{
		{name: "run-metadata", text: fmt.Sprintf("event=%s fork=%t ref=%s status=%s", fixture.Run.Event, fixture.Run.ForkPullRequest, fixture.Run.Ref, fixture.Run.Status)},
		{name: "job:" + fixture.Job.Name, text: fixture.Job.Log},
	}
	for _, artifact := range fixture.Artifacts {
		sources = append(sources, source{name: "artifact:" + artifact.Name, text: artifact.Content})
	}

	diagnosis := Diagnosis{}
	bounded := make([]source, 0, len(sources))
	for _, item := range sources {
		diagnosis.InputBytes += len(item.text)
		diagnosis.InputLines += strings.Count(item.text, "\n") + 1
		text := item.text
		if len(text) > budget.MaxSourceBytes {
			text = text[:budget.MaxSourceBytes]
			diagnosis.Truncated = true
		}
		diagnosis.InspectedBytes += len(text)
		bounded = append(bounded, source{name: item.name, text: text})
	}

	joined := strings.ToLower(joinSources(bounded))
	switch {
	case fixture.Run.Event == "pull_request" && fixture.Run.ForkPullRequest && (strings.Contains(joined, "403 forbidden") || strings.Contains(joined, "resource not accessible")):
		diagnosis.Category = "workflow-configuration"
		diagnosis.Confidence = "high"
		diagnosis.Claim = "The release upload cannot succeed because a forked pull-request run receives a read-only automatic token."
		diagnosis.Uncertainty = "The cause is identified; whether publishing belongs on a trusted tag or another guarded trigger remains a design decision."
		diagnosis.NextAction = fmt.Sprintf("fjgo -R origin workflow view %s --ref %s --full", fixture.Run.Workflow, fixture.Run.SHA)
		diagnosis.NextActionKind = "read-only"
		diagnosis.NextActionWhy = "Inspect the failed trigger and publish guard before changing credentials or retrying."
		diagnosis.Evidence, diagnosis.Redactions = selectEvidence(bounded, budget, []string{"event=pull_request fork=true", "403 forbidden", "authorization:"})
	case strings.Contains(joined, "no online runner matches"):
		diagnosis.Category = "runner-or-infrastructure"
		diagnosis.Confidence = "high"
		diagnosis.Claim = fmt.Sprintf("No online runner matched the job labels %s.", strings.Join(fixture.Job.RunsOn, ", "))
		diagnosis.Uncertainty = "Runner visibility and health still need readback; no workflow step started."
		diagnosis.NextAction = "fjgo api call getRepoRunners owner=astrazds repo=fjgo visible=true"
		diagnosis.NextActionKind = "read-only"
		diagnosis.NextActionWhy = "Inspect runner availability without mutating or blindly retrying the run."
		diagnosis.Evidence, diagnosis.Redactions = selectEvidence(bounded, budget, []string{"no online runner matches", "requested labels"})
	case strings.Contains(joined, "--- fail:") && strings.Contains(joined, "expected"):
		diagnosis.Category = "code-or-test"
		diagnosis.Confidence = "high"
		diagnosis.Claim = "The token-redaction test failed because a reflected credential remained in the HTTP error."
		diagnosis.Uncertainty = "The excerpt identifies the failing assertion, not the production code path that introduced it."
		diagnosis.NextAction = "go test ./internal/forgejo -run TestClientRedactsToken"
		diagnosis.NextActionKind = "local"
		diagnosis.NextActionWhy = "Reproduce the single failing test locally before editing code or rerunning all CI."
		diagnosis.Evidence, diagnosis.Redactions = selectEvidence(bounded, budget, []string{"expected error body", "<failure message"})
	default:
		diagnosis.Category = "unknown"
		diagnosis.Confidence = "low"
		diagnosis.Claim = "The bounded evidence does not support a specific failure cause."
		diagnosis.Uncertainty = "More targeted evidence is required; guessing would be unsafe."
		diagnosis.NextAction = fmt.Sprintf("fjgo -R origin run view %d --log-failed --full", fixture.Run.ID)
		diagnosis.NextActionKind = "read-only"
		diagnosis.NextActionWhy = "Inspect the failed task surface without triggering a retry."
	}
	diagnosis.EvidenceLines = len(diagnosis.Evidence)
	return diagnosis
}

func joinSources(sources []source) string {
	var joined strings.Builder
	for _, item := range sources {
		joined.WriteString(item.text)
		joined.WriteByte('\n')
	}
	return joined.String()
}

func selectEvidence(sources []source, budget Budget, needles []string) ([]Evidence, int) {
	var evidence []Evidence
	redactions := 0
	seen := map[string]bool{}
	for _, needle := range needles {
		for _, item := range sources {
			for _, line := range strings.Split(item.text, "\n") {
				if len(evidence) >= budget.MaxEvidence || !strings.Contains(strings.ToLower(line), needle) {
					continue
				}
				line, count := redact(strings.TrimSpace(line))
				if len(line) > budget.MaxEvidenceText {
					line = line[:budget.MaxEvidenceText] + "…"
				}
				key := item.name + "\x00" + line
				if line != "" && !seen[key] {
					evidence = append(evidence, Evidence{Source: item.name, Excerpt: line})
					redactions += count
					seen[key] = true
				}
			}
		}
	}
	return evidence, redactions
}

func redact(line string) (string, int) {
	count := len(authorizationPattern.FindAllStringIndex(line, -1)) + len(secretPattern.FindAllStringIndex(line, -1))
	line = authorizationPattern.ReplaceAllString(line, `${1}[redacted]`)
	line = secretPattern.ReplaceAllString(line, `${1}[redacted]`)
	return line, count
}
