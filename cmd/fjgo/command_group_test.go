package main

import (
	"io"
	"strings"
	"testing"
)

func TestTakeCuratedYes(t *testing.T) {
	rest, token, err := takeCuratedYes([]string{"--yes", "a"}, mutationRequired)
	if err != nil || token.command == "" || len(rest) != 1 || rest[0] != "a" {
		t.Fatalf("rest=%v token=%q err=%v", rest, token.command, err)
	}
	_, token, err = takeCuratedYes([]string{"a"}, mutationRequired)
	if err == nil || err.Error() != "requires --yes" || token.command != "" {
		t.Fatalf("token=%q err=%v", token.command, err)
	}
	rest, token, err = takeCuratedYes([]string{"--dry-run"}, mutationNone)
	if err != nil || token.command != "" || len(rest) != 1 {
		t.Fatalf("optional rest=%v token=%q err=%v", rest, token.command, err)
	}
	if err := writeMutationPreview(io.Discard, nil, "issueEditIssue", repoRef{}, nil, nil, yesToken{}); err == nil {
		t.Fatal("empty token was accepted")
	}
}

func TestLeafHelpViolations(t *testing.T) {
	groups := []commandGroup{
		rootGroup(),
		apiGroup(),
		aliasGroup(),
		issueGroup(),
		labelGroup(),
		prGroup(),
		releaseGroup(),
		repoGroup(),
		runGroup(),
		searchGroup(),
		secretGroup(),
		skillGroup(),
		modelGroup(),
		variableGroup(),
		workflowGroup(),
	}
	for _, group := range groups {
		violations := LeafHelpViolations(group)
		if len(violations) != 0 {
			t.Errorf("%s\n%s", group.name, strings.Join(violations, "\n"))
		}
	}
}
