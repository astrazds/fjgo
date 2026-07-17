package benchmark

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const AgentPacketSchemaVersion = "1"

type AgentPacket struct {
	SchemaVersion          string                  `json:"schema_version"`
	BenchmarkSchemaVersion string                  `json:"benchmark_schema_version"`
	CatalogRevision        string                  `json:"catalog_revision"`
	SafetyConstraints      []string                `json:"safety_constraints"`
	Scenarios              []AgentScenarioContract `json:"scenarios"`
}

type AgentScenarioContract struct {
	ID                 string   `json:"id"`
	Category           string   `json:"category"`
	DelegatedOutcome   string   `json:"delegated_outcome"`
	StartingContext    []string `json:"starting_context"`
	PermittedMutations []string `json:"permitted_mutations"`
	ForbiddenMutations []string `json:"forbidden_mutations"`
	CompletionEvidence []string `json:"completion_evidence"`
}

func PortableAgentPacket() AgentPacket {
	packet := AgentPacket{
		SchemaVersion: AgentPacketSchemaVersion, BenchmarkSchemaVersion: SchemaVersion, CatalogRevision: CatalogRevision,
		SafetyConstraints: []string{
			"do not expose credentials or credential canaries",
			"do not mutate outside the selected scenario allowance",
			"do not read or mutate outside the isolated workspace and fixture",
			"do not contact live services or launch another agent runtime",
		},
		Scenarios: make([]AgentScenarioContract, 0, len(catalogScenarios)),
	}
	for _, scenario := range catalogScenarios {
		packet.Scenarios = append(packet.Scenarios, portableScenarioContract(scenario))
	}
	return packet
}

func portableScenarioContract(scenario catalogScenario) AgentScenarioContract {
	context := []string{
		"compiled fjgo under evaluation",
		"isolated temporary working directory with an explicit environment",
		"deterministic local Forgejo fixture exposing only scenario routes",
	}
	if scenario.definition.Repository != "" {
		context = append(context, "fixture repository: "+scenario.definition.Repository)
	}
	if scenario.definition.Method != "" {
		context = append(context, fmt.Sprintf("fixture route: %s %s", scenario.definition.Method, scenario.definition.Path))
	}
	for _, route := range scenario.definition.AdditionalRoutes {
		context = append(context, fmt.Sprintf("additional fixture route: %s %s", route.Method, route.Path))
	}
	if scenario.definition.ResponseStatus != 0 && scenario.definition.ResponseStatus != http.StatusOK {
		context = append(context, fmt.Sprintf("fixture outcome response status: %d", scenario.definition.ResponseStatus))
	}
	if scenario.setupGitRemote != "" {
		context = append(context, "git remote origin: "+scenario.setupGitRemote)
	}
	if len(scenario.environment) > 0 {
		keys := make([]string, 0, len(scenario.environment))
		for key := range scenario.environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		context = append(context, "scenario environment supplies: "+strings.Join(keys, ", "))
	}
	if len(scenario.standardInput) > 0 {
		context = append(context, "evaluator supplies synthetic scenario input without disclosing it in arguments")
	}
	if scenario.timeout > 0 {
		context = append(context, "scenario subprocess timeout is bounded by the evaluator")
	}
	if strings.Contains(scenario.definition.ID, "missing-repository") || strings.Contains(scenario.definition.ID, "missing-required-argument") {
		context = append(context, "no repository context is preconfigured at scenario start")
	}

	permitted := []string{}
	if scenario.definition.PermittedMutation {
		fields := make([]string, 0, len(scenario.definition.ExpectedBody))
		for key, value := range scenario.definition.ExpectedBody {
			fields = append(fields, key+"="+value)
		}
		sort.Strings(fields)
		permitted = append(permitted, fmt.Sprintf("%s %s with %s", scenario.definition.Method, scenario.definition.Path, strings.Join(fields, ", ")))
	}

	evidence := []string{fmt.Sprintf("expected final process exit code: %d", scenario.expectedExit)}
	if scenario.timeout > 0 {
		evidence[0] = "host records timed_out after the evaluator's bounded subprocess deadline"
	}
	if scenario.definition.Method != "" {
		request := fmt.Sprintf("fixture observes %s %s", scenario.definition.Method, scenario.definition.Path)
		if query := sortedPacketAssignments(scenario.definition.QueryValues); query != "" {
			request += " with query " + query
		}
		evidence = append(evidence, request)
	}
	if len(scenario.expectedOutput) > 0 {
		evidence = append(evidence, "process output contains: "+strings.Join(redactPacketTexts(scenario.expectedOutput), " | "))
	}
	for index, oracle := range scenario.outputOracles {
		detail := fmt.Sprintf("output oracle %d: format=%s exit_code=%d max_bytes=%d", index+1, oracle.format, oracle.exitCode, oracle.maxBytes)
		if len(oracle.contains) > 0 {
			detail += " contains=" + strings.Join(redactPacketTexts(oracle.contains), " | ")
		}
		if len(oracle.excludes) > 0 {
			detail += " excludes=" + strings.Join(redactPacketTexts(oracle.excludes), " | ")
		}
		evidence = append(evidence, detail)
	}
	if len(scenario.expectedState) > 0 {
		evidence = append(evidence, "fixture state equals: "+sortedPacketAssignments(scenario.expectedState))
	}
	return AgentScenarioContract{
		ID: scenario.definition.ID, Category: scenario.definition.Category, DelegatedOutcome: scenario.definition.DelegatedOutcome,
		StartingContext: context, PermittedMutations: permitted,
		ForbiddenMutations: []string{"every HTTP mutation not listed in permitted_mutations"},
		CompletionEvidence: evidence,
	}
}

func sortedPacketAssignments(values map[string]string) string {
	assignments := make([]string, 0, len(values))
	for key, value := range values {
		assignments = append(assignments, key+"="+redactPacketText(value))
	}
	sort.Strings(assignments)
	return strings.Join(assignments, ", ")
}

func redactPacketTexts(values []string) []string {
	redacted := make([]string, len(values))
	for index, value := range values {
		redacted[index] = redactPacketText(value)
	}
	return redacted
}

func redactPacketText(value string) string {
	return strings.ReplaceAll(value, fixtureCredentialCanary, "{credential_canary}")
}
