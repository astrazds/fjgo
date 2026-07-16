package main

import (
	"fmt"
	"sort"
	"strings"
)

type Issue struct {
	Number       int
	Title        string
	State        string
	Assignee     string
	Dependencies []int
}

type Branch struct {
	Name string
	SHA  string
}

type Check struct {
	Name   string
	Status string
}

type Review struct {
	ID     int
	Author string
	State  string
}

type Comment struct {
	ReviewID int
	Author   string
	Body     string
	Resolved bool
}

type PullRequest struct {
	Number     int
	Title      string
	State      string
	HeadBranch string
	HeadSHA    string
	Closes     int
	Checks     []Check
	Reviews    []Review
	Comments   []Comment
}

type Notification struct {
	IssueNumber int
	PullNumber  int
	Reason      string
	Unread      bool
	Updated     string
}

type Snapshot struct {
	Actor         string
	Issues        []Issue
	Branches      []Branch
	PullRequests  []PullRequest
	Notifications []Notification
}

type View struct {
	Loop       string
	Position   string
	Focus      string
	Because    []string
	Next       string
	Command    string
	Alternates []string
	FactCount  int
}

// DeriveChange selects one next action from Forgejo-native change-loop facts.
// It stores no checkpoint or synthetic lifecycle state.
func DeriveChange(snapshot Snapshot) View {
	view := View{Loop: "change", FactCount: countFacts(snapshot)}
	for _, issue := range snapshot.Issues {
		if issue.State != "open" || issue.Assignee != snapshot.Actor {
			continue
		}
		pr, ok := pullForIssue(snapshot.PullRequests, issue.Number)
		if !ok {
			continue
		}
		view.Focus = fmt.Sprintf("issue:%d → pull:%d @ %s", issue.Number, pr.Number, shortSHA(pr.HeadSHA))
		branch, branchMatches := branchForPull(snapshot.Branches, pr)
		if !branchMatches {
			view.Position = "branch and pull request disagree"
			view.Because = []string{fmt.Sprintf("pull:%d expects %s at %s", pr.Number, pr.HeadBranch, shortSHA(pr.HeadSHA))}
			view.Next = "inspect the remote branch before deriving further progress"
			view.Command = fmt.Sprintf("fjgo -R origin repo branches get %s", pr.HeadBranch)
			return view
		}
		branchReason := fmt.Sprintf("branch %s matches pull head %s", branch.Name, shortSHA(branch.SHA))

		if review, comment, ok := unresolvedRequestedChange(pr); ok {
			view.Position = "review feedback blocks review-ready"
			view.Because = []string{
				branchReason,
				fmt.Sprintf("%s requested changes on pull:%d", review.Author, pr.Number),
				fmt.Sprintf("unresolved comment: %q", comment.Body),
			}
			if notificationForPull(snapshot.Notifications, pr.Number, "review_requested") {
				view.Because = append(view.Because, "unread review notification confirms the feedback is new")
			}
			view.Next = "inspect the requested-change thread before acting on checks"
			view.Command = fmt.Sprintf("fjgo api call repoGetPullReviewComments owner=astrazds repo=fjgo index=%d id=%d", pr.Number, review.ID)
			if failed := failedChecks(pr.Checks); len(failed) > 0 {
				view.Alternates = append(view.Alternates, fmt.Sprintf("defer failed check %s; review changes may obsolete it", strings.Join(failed, ", ")))
			}
			return view
		}

		if failed := failedChecks(pr.Checks); len(failed) > 0 {
			view.Position = "verification blocks review-ready"
			view.Because = []string{branchReason, fmt.Sprintf("failed checks: %s", strings.Join(failed, ", "))}
			view.Next = "inspect the failed checks"
			view.Command = fmt.Sprintf("fjgo -R origin pr checks %d", pr.Number)
			return view
		}

		if checksPassed(pr.Checks) && approved(pr.Reviews) {
			view.Position = "review-ready"
			view.Because = []string{branchReason, "all observed checks passed", "an approving review exists", "pull request remains open at the observed head"}
			view.Next = "hand off pull request for explicit merge authorization"
			view.Command = fmt.Sprintf("fjgo -R origin pr view %d --reviews --full", pr.Number)
			view.Alternates = []string{"do not merge automatically; merge is beyond the review-ready boundary"}
			return view
		}
	}

	view.Position = "no active assigned change found"
	view.Next = "inspect assigned open issues"
	view.Command = "fjgo -R origin issue list --state open --assignee agent"
	return view
}

// DeriveTriage filters the queue before selecting the highest-signal item.
func DeriveTriage(snapshot Snapshot) View {
	view := View{Loop: "triage", Position: "selecting from Forgejo activity", FactCount: countFacts(snapshot)}
	closed := map[int]bool{}
	for _, issue := range snapshot.Issues {
		closed[issue.Number] = issue.State == "closed"
	}
	active := map[int]bool{}
	for _, pr := range snapshot.PullRequests {
		if pr.State == "open" {
			active[pr.Closes] = true
		}
	}

	type candidate struct {
		issue  Issue
		reason string
		rank   int
	}
	var candidates []candidate
	for _, issue := range snapshot.Issues {
		if issue.State != "open" {
			continue
		}
		if issue.Assignee != snapshot.Actor {
			view.Alternates = append(view.Alternates, fmt.Sprintf("skip issue:%d: assigned to %s", issue.Number, emptyAs(issue.Assignee, "nobody")))
			continue
		}
		if blocker := openDependency(issue.Dependencies, closed); blocker != 0 {
			view.Alternates = append(view.Alternates, fmt.Sprintf("skip issue:%d: blocked by open issue:%d", issue.Number, blocker))
			continue
		}
		if active[issue.Number] {
			view.Alternates = append(view.Alternates, fmt.Sprintf("skip issue:%d: already has an open pull request", issue.Number))
			continue
		}
		reason, rank := notificationSignal(snapshot.Notifications, issue.Number)
		candidates = append(candidates, candidate{issue: issue, reason: reason, rank: rank})
	}
	if len(candidates) == 0 {
		view.Position = "no unblocked assigned issue needs triage"
		view.Next = "inspect unread notifications"
		view.Command = "fjgo api call notifyGetList status-types=unread"
		return view
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].rank > candidates[j].rank })
	selected := candidates[0]
	for _, deferred := range candidates[1:] {
		view.Alternates = append(view.Alternates, fmt.Sprintf("defer issue:%d: lower signal than %s", deferred.issue.Number, selected.reason))
	}
	view.Focus = fmt.Sprintf("issue:%d — %s", selected.issue.Number, selected.issue.Title)
	view.Because = []string{"assigned to agent", "all dependencies are closed", selected.reason, "no open pull request already carries the change"}
	view.Next = "inspect this issue and its conversation"
	view.Command = fmt.Sprintf("fjgo -R origin issue view %d --comments --full", selected.issue.Number)
	return view
}

func pullForIssue(pulls []PullRequest, issue int) (PullRequest, bool) {
	for _, pull := range pulls {
		if pull.State == "open" && pull.Closes == issue {
			return pull, true
		}
	}
	return PullRequest{}, false
}

func branchForPull(branches []Branch, pull PullRequest) (Branch, bool) {
	for _, branch := range branches {
		if branch.Name == pull.HeadBranch && branch.SHA == pull.HeadSHA {
			return branch, true
		}
	}
	return Branch{}, false
}

func unresolvedRequestedChange(pull PullRequest) (Review, Comment, bool) {
	for _, review := range pull.Reviews {
		if review.State != "REQUEST_CHANGES" {
			continue
		}
		for _, comment := range pull.Comments {
			if comment.ReviewID == review.ID && !comment.Resolved {
				return review, comment, true
			}
		}
	}
	return Review{}, Comment{}, false
}

func failedChecks(checks []Check) []string {
	var failed []string
	for _, check := range checks {
		if check.Status == "failure" || check.Status == "error" {
			failed = append(failed, check.Name)
		}
	}
	return failed
}

func checksPassed(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if check.Status != "success" {
			return false
		}
	}
	return true
}

func approved(reviews []Review) bool {
	for _, review := range reviews {
		if review.State == "APPROVED" {
			return true
		}
	}
	return false
}

func notificationForPull(notifications []Notification, pull int, reason string) bool {
	for _, notification := range notifications {
		if notification.Unread && notification.PullNumber == pull && notification.Reason == reason {
			return true
		}
	}
	return false
}

func notificationSignal(notifications []Notification, issue int) (string, int) {
	bestReason, bestRank := "no unread notification; selected from assignment", 0
	for _, notification := range notifications {
		if !notification.Unread || notification.IssueNumber != issue {
			continue
		}
		rank := map[string]int{"mention": 3, "assign": 2, "subscribed": 1}[notification.Reason]
		if rank > bestRank {
			bestReason = "unread " + notification.Reason + " notification"
			bestRank = rank
		}
	}
	return bestReason, bestRank
}

func openDependency(dependencies []int, closed map[int]bool) int {
	for _, dependency := range dependencies {
		if !closed[dependency] {
			return dependency
		}
	}
	return 0
}

func countFacts(snapshot Snapshot) int {
	count := len(snapshot.Issues) + len(snapshot.Branches) + len(snapshot.Notifications)
	for _, pull := range snapshot.PullRequests {
		count += 1 + len(pull.Checks) + len(pull.Reviews) + len(pull.Comments)
	}
	return count
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
