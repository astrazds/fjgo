// Package model is the pure state machine behind the throwaway agent-job
// lifecycle prototype. It performs no I/O and is not production code.
package model

import "fmt"

type Kind string

const (
	Change Kind = "change loop"
	Triage Kind = "triage loop"
)

type Receipt struct {
	Session int
	Action  string
	Result  string
}

type Snapshot struct {
	Phase       int
	Status      string
	NextAction  string
	Evidence    []string
	Disposition []string
}

type State struct {
	ID            string
	Kind          Kind
	Goal          string
	Session       int
	Phase         int
	Status        string
	NextAction    string
	Evidence      []string
	Disposition   []string
	Checkpoint    *Snapshot
	CheckpointSeq int
	Receipts      []Receipt
	Notice        string
}

type Action string

const (
	StartChange Action = "start-change"
	StartTriage Action = "start-triage"
	Advance     Action = "advance"
	Checkpoint  Action = "checkpoint"
	Resume      Action = "resume"
)

type step struct {
	status      string
	next        string
	evidence    string
	disposition string
}

var changeSteps = []step{
	{status: "active", next: "Inspect issue 42, its dependencies, and repository state", evidence: "Issue 42 is actionable; main is green; no blocking dependency"},
	{status: "active", next: "Form a bounded change plan and identify approval boundaries", evidence: "Plan: update parser, add focused regression, no remote mutation yet"},
	{status: "active", next: "Publish the proposed branch and open a pull request", evidence: "Local change verified; branch proposal ready"},
	{status: "active", next: "Inspect checks and assemble review context", evidence: "Pull request 17 opened; verify workflow passed"},
	{status: "review-ready", next: "Wait for reviewer decision", evidence: "Review packet includes change, checks, risks, and provenance"},
}

var triageSteps = []step{
	{status: "active", next: "Collect unread issues, pull requests, failures, and notifications", evidence: "Collected 8 repository signals since last disposition"},
	{status: "active", next: "Classify urgency, duplication, ownership, and required action", evidence: "2 actionable, 3 informational, 2 duplicates, 1 blocked"},
	{status: "active", next: "Write Forgejo-native dispositions and dependency links", disposition: "Issue 51 prioritized; issue 49 marked duplicate; failed run routed to recovery"},
	{status: "complete", next: "Wait for new repository activity", evidence: "Prioritized queue contains 2 actionable items with evidence"},
}

func Initial() State {
	return State{Status: "idle", Notice: "Choose a job to start"}
}

func Reduce(in State, action Action) State {
	s := clone(in)
	s.Notice = ""
	switch action {
	case StartChange:
		s = start(Change)
	case StartTriage:
		s = start(Triage)
	case Advance:
		s = advance(s)
	case Checkpoint:
		s = checkpoint(s)
	case Resume:
		s = resume(s)
	default:
		s.Notice = "Unknown action"
	}
	return s
}

func start(kind Kind) State {
	steps := stepsFor(kind)
	goal := "Carry issue 42 to a review-ready pull request"
	id := "job-change-42"
	if kind == Triage {
		goal = "Turn new repository activity into an actionable queue"
		id = "job-triage-2026-07-16"
	}
	return State{
		ID:         id,
		Kind:       kind,
		Goal:       goal,
		Session:    1,
		Status:     steps[0].status,
		NextAction: steps[0].next,
		Receipts:   []Receipt{{Session: 1, Action: "job started", Result: "goal and initial next action recorded"}},
		Notice:     "Live job started; no durable checkpoint exists yet",
	}
}

func advance(s State) State {
	if s.Status == "idle" || s.ID == "" {
		s.Notice = "Start a job first"
		return s
	}
	steps := stepsFor(s.Kind)
	if s.Phase >= len(steps) {
		s.Notice = "Job has no further simulated phase"
		return s
	}
	current := steps[s.Phase]
	if current.evidence != "" {
		s.Evidence = append(s.Evidence, current.evidence)
	}
	if current.disposition != "" {
		s.Disposition = append(s.Disposition, current.disposition)
	}
	s.Phase++
	if s.Phase < len(steps) {
		s.Status = steps[s.Phase].status
		s.NextAction = steps[s.Phase].next
	} else {
		s.Status = current.status
		s.NextAction = current.next
	}
	s.Receipts = append(s.Receipts, Receipt{Session: s.Session, Action: fmt.Sprintf("phase %d completed", s.Phase), Result: current.evidence + current.disposition})
	s.Notice = "Live progress advanced; checkpoint when this should survive interruption"
	return s
}

func checkpoint(s State) State {
	if s.ID == "" {
		s.Notice = "Start a job first"
		return s
	}
	s.CheckpointSeq++
	s.Checkpoint = &Snapshot{
		Phase:       s.Phase,
		Status:      s.Status,
		NextAction:  s.NextAction,
		Evidence:    append([]string(nil), s.Evidence...),
		Disposition: append([]string(nil), s.Disposition...),
	}
	s.Receipts = append(s.Receipts, Receipt{Session: s.Session, Action: fmt.Sprintf("checkpoint %d", s.CheckpointSeq), Result: "durable job state updated"})
	s.Notice = "Checkpoint recorded as the durable Forgejo-backed projection"
	return s
}

func resume(s State) State {
	if s.ID == "" {
		s.Notice = "Start a job first"
		return s
	}
	s.Session++
	if s.Checkpoint == nil {
		kind := s.Kind
		fresh := start(kind)
		fresh.Session = s.Session
		fresh.Receipts = append(s.Receipts, Receipt{Session: fresh.Session, Action: "session resumed", Result: "no checkpoint; reconstructed from job goal only"})
		fresh.Notice = "Uncheckpointed progress was lost; resumed from the job goal"
		return fresh
	}
	s.Phase = s.Checkpoint.Phase
	s.Status = s.Checkpoint.Status
	s.NextAction = s.Checkpoint.NextAction
	s.Evidence = append([]string(nil), s.Checkpoint.Evidence...)
	s.Disposition = append([]string(nil), s.Checkpoint.Disposition...)
	s.Receipts = append(s.Receipts, Receipt{Session: s.Session, Action: "session resumed", Result: fmt.Sprintf("restored checkpoint %d", s.CheckpointSeq)})
	s.Notice = fmt.Sprintf("New agent session restored checkpoint %d and its next action", s.CheckpointSeq)
	return s
}

func stepsFor(kind Kind) []step {
	if kind == Triage {
		return triageSteps
	}
	return changeSteps
}

func clone(s State) State {
	s.Evidence = append([]string(nil), s.Evidence...)
	s.Disposition = append([]string(nil), s.Disposition...)
	s.Receipts = append([]Receipt(nil), s.Receipts...)
	if s.Checkpoint != nil {
		cp := *s.Checkpoint
		cp.Evidence = append([]string(nil), cp.Evidence...)
		cp.Disposition = append([]string(nil), cp.Disposition...)
		s.Checkpoint = &cp
	}
	return s
}
