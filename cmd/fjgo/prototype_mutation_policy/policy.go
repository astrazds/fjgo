package main

import (
	"fmt"
	"strings"
)

type Classification string

const (
	Allowed          Classification = "allowed"
	ApprovalRequired Classification = "approval-required"
	Denied           Classification = "denied"
)

type Rule struct {
	Action         string
	Classification Classification
	Reason         string
}

type Policy struct {
	Name  string
	Repo  string
	Rules []Rule
}

type Approval struct {
	ID              string
	By              string
	Action          string
	ResourceVersion string
}

type Mutation struct {
	Action          string
	Repo            string
	Resource        string
	Command         string
	ExpectedVersion string
	ObservedVersion string
	Approval        *Approval
	Result          string
	ReceiptFields   []Field
}

type Field struct {
	Name  string
	Value string
}

type Evaluation struct {
	Classification Classification
	Rule           string
	PolicyReason   string
	Precondition   string
	Outcome        string
	OutcomeReason  string
	Executable     bool
}

// Evaluate is deliberately pure: policy decides authority, while current
// Forgejo state and approval evidence decide whether this invocation may run.
func Evaluate(policy Policy, mutation Mutation) Evaluation {
	classification := Denied
	ruleName := "default deny"
	reason := "no policy rule grants this action"

	if mutation.Repo != policy.Repo {
		reason = fmt.Sprintf("repository %s is outside scope %s", mutation.Repo, policy.Repo)
	} else {
		for _, rule := range policy.Rules {
			if rule.Action == mutation.Action {
				classification = rule.Classification
				ruleName = rule.Action
				reason = rule.Reason
				break
			}
		}
	}

	evaluation := Evaluation{
		Classification: classification,
		Rule:           ruleName,
		PolicyReason:   reason,
		Precondition:   "current",
	}

	if mutation.ExpectedVersion != mutation.ObservedVersion {
		evaluation.Precondition = "stale"
		evaluation.Outcome = "rejected"
		evaluation.OutcomeReason = fmt.Sprintf("expected %s; observed %s", mutation.ExpectedVersion, mutation.ObservedVersion)
		return evaluation
	}

	switch classification {
	case Denied:
		evaluation.Outcome = "rejected"
		evaluation.OutcomeReason = "mutation policy denies the action"
	case ApprovalRequired:
		if approvalMatches(mutation.Approval, mutation) {
			evaluation.Outcome = "execute"
			evaluation.OutcomeReason = "approval is bound to this action and resource version"
			evaluation.Executable = true
		} else {
			evaluation.Outcome = "awaiting-approval"
			evaluation.OutcomeReason = "request approval bound to the current resource version"
		}
	case Allowed:
		evaluation.Outcome = "execute"
		evaluation.OutcomeReason = "policy grants the scoped action"
		evaluation.Executable = true
	}

	return evaluation
}

func approvalMatches(approval *Approval, mutation Mutation) bool {
	return approval != nil &&
		approval.Action == mutation.Action &&
		approval.ResourceVersion == mutation.ObservedVersion
}

func Receipt(policy Policy, mutation Mutation, evaluation Evaluation) []Field {
	if !evaluation.Executable {
		return nil
	}
	fields := []Field{
		{Name: "receipt", Value: "rcpt_01JZ8M4P"},
		{Name: "policy", Value: policy.Name},
		{Name: "repository", Value: mutation.Repo},
		{Name: "action", Value: mutation.Action},
		{Name: "resource", Value: mutation.Resource},
		{Name: "classification", Value: string(evaluation.Classification)},
		{Name: "precondition", Value: mutation.ObservedVersion},
	}
	if mutation.Approval != nil {
		fields = append(fields,
			Field{Name: "approval", Value: mutation.Approval.ID},
			Field{Name: "approved_by", Value: mutation.Approval.By},
		)
	}
	fields = append(fields, mutation.ReceiptFields...)
	fields = append(fields, Field{Name: "result", Value: mutation.Result})
	return redact(fields)
}

func redact(fields []Field) []Field {
	redacted := make([]Field, 0, len(fields))
	for _, field := range fields {
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "token") || name == "secret_value" || strings.Contains(name, "authorization") {
			continue
		}
		redacted = append(redacted, field)
	}
	return redacted
}
