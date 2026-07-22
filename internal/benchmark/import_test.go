package benchmark

import (
	"fmt"
	"strings"
	"testing"
)

func TestImportHostRunConvertsCodexRecordToBenchmarkResult(t *testing.T) {
	record := []byte(`{
  "schema_version": "1",
  "benchmark_schema_version": "5",
  "catalog_revision": "6",
  "fjgo": {"version": "1.2.0", "source_revision": "abc123"},
  "host": {"name": "codex", "version": "2026.7"},
  "model": {"provider": "openai", "name": "gpt-5"},
  "scenario": {"id": "discovery.operation-inspect", "status": "passed", "completion_satisfied": true},
  "metrics": {"cli_invocations": 1, "api_requests": 0, "stdout_bytes": 240, "stderr_bytes": 0, "input_tokens": 120, "output_tokens": 45},
  "safety": {"unexpected_requests": 0, "unsafe_requests": 0, "credential_leaks": 0, "timed_out": false},
  "evidence": [{"id": "artifacts/discovery-operation-inspect.json", "kind": "host-artifact"}]
}`)

	result, err := ImportHostRun(record, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != SchemaVersion || result.CatalogRevision != CatalogRevision || result.SourceRevision != "abc123" {
		t.Fatalf("versions = schema %q catalog %q source %q", result.SchemaVersion, result.CatalogRevision, result.SourceRevision)
	}
	if result.Execution.Mode != "agent_host" || result.Execution.FJGOVersion != "1.2.0" || result.Execution.Host.Name != "codex" || result.Execution.Model.Name != "gpt-5" {
		t.Fatalf("execution = %+v", result.Execution)
	}
	if result.Scenario.ID != "discovery.operation-inspect" || result.Scenario.Category != "operation-discovery" || result.Scenario.Status != StatusPassed || !result.Scenario.Completion.Satisfied {
		t.Fatalf("scenario = %+v", result.Scenario)
	}
	if result.Metrics.InputTokens == nil || *result.Metrics.InputTokens != 120 || result.Metrics.OutputTokens == nil || *result.Metrics.OutputTokens != 45 {
		t.Fatalf("metrics = %+v", result.Metrics)
	}
	if len(result.Evidence) != 1 || !strings.Contains(result.Evidence[0].ID, "operation-inspect") {
		t.Fatalf("evidence = %+v", result.Evidence)
	}
}

func TestImportHostRunDistinguishesPortableRunStatuses(t *testing.T) {
	tests := []struct {
		name, status, completion, metrics, safety string
	}{
		{"passed", StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`},
		{"failed", StatusFailed, "false", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`},
		{"incomplete", StatusIncomplete, "false", `"manual_corrections":0,"clarifications":1`, `"timed_out":false`},
		{"timed out", StatusTimedOut, "false", `"manual_corrections":0,"clarifications":0`, `"timed_out":true`},
		{"manually corrected", StatusManuallyCorrected, "true", `"manual_corrections":1,"clarifications":0`, `"timed_out":false`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportHostRun(portableRecord(test.status, test.completion, test.metrics, test.safety, "codex"), nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.Scenario.Status != test.status {
				t.Fatalf("status = %q", result.Scenario.Status)
			}
		})
	}
}

func TestImportHostRunAcceptsSupportedHostMetadata(t *testing.T) {
	for _, host := range []string{"codex", "claude_code", "opencode", "future_host"} {
		t.Run(host, func(t *testing.T) {
			result, err := ImportHostRun(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, host), nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.Execution.Host.Name != host {
				t.Fatalf("host = %+v", result.Execution.Host)
			}
		})
	}
}

func TestImportHostRunRejectsUnsafeIncompatibleOrUnboundedRecords(t *testing.T) {
	valid := string(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, "codex"))
	tests := []struct {
		name, record, canary, want string
	}{
		{"incompatible host schema", strings.Replace(valid, `"schema_version":"1"`, `"schema_version":"0"`, 1), "", "host run schema version"},
		{"incompatible benchmark schema", strings.Replace(valid, `"benchmark_schema_version":"5"`, `"benchmark_schema_version":"0"`, 1), "", "benchmark schema version"},
		{"unknown scenario", strings.Replace(valid, "discovery.operation-inspect", "unknown.scenario", 1), "", "unknown benchmark scenario"},
		{"manual correction reported as passed", strings.Replace(valid, `"manual_corrections":0`, `"manual_corrections":1`, 1), "", "manual corrections"},
		{"clarification reported as passed", strings.Replace(valid, `"clarifications":0`, `"clarifications":1`, 1), "", "clarifications"},
		{"credential canary", strings.Replace(valid, `"host-version"`, `"secret-value"`, 1), "secret-value", "credential canary"},
		{"escaped credential canary", strings.Replace(valid, `"host-version"`, `"\u0073ecret-value"`, 1), "secret-value", "credential canary"},
		{"escaped punctuation credential canary", strings.Replace(valid, `"host-version"`, `"\u003csecret\u003e"`, 1), "<secret>", "credential canary"},
		{"duplicate field escaped credential canary", strings.Replace(valid, `"host":{"name":"codex"`, `"host":{"name":"\u003csecret\u003e","name":"codex"`, 1), "<secret>", "credential canary"},
		{"duplicate field", strings.Replace(valid, `"host":{"name":"codex"`, `"host":{"name":"first","name":"codex"`, 1), "", "duplicate JSON key"},
		{"retained transcript", strings.Replace(valid, `"evidence":`, `"transcript":"raw session","evidence":`, 1), "", "unknown field"},
		{"missing required byte count", strings.Replace(valid, `"stdout_bytes":10,`, "", 1), "", "stdout_bytes"},
		{"negative token count", strings.Replace(valid, `"output_tokens":4`, `"output_tokens":-1`, 1), "", "output tokens"},
		{"completed without evidence", strings.Replace(valid, `[{"id":"result.json","kind":"host-artifact"}]`, `[]`, 1), "", "completion evidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canaries := []string(nil)
			if test.canary != "" {
				canaries = []string{test.canary}
			}
			_, err := ImportHostRun([]byte(test.record), canaries)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	tooLarge := append(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, "codex"), make([]byte, maxHostRunRecordSize)...)
	if _, err := ImportHostRun(tooLarge, nil); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestImportHostRunEnforcesScenarioMutationContractForSuccessfulRuns(t *testing.T) {
	readOnly := string(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, "codex"))
	readOnly = strings.Replace(readOnly, `"mutating_requests":0`, `"mutating_requests":1`, 1)
	if _, err := ImportHostRun([]byte(readOnly), nil); err == nil || !strings.Contains(err.Error(), "mutation") {
		t.Fatalf("read-only mutation error = %v", err)
	}

	permitted := string(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, "codex"))
	permitted = strings.Replace(permitted, "discovery.operation-inspect", "mutation.permitted-repo-edit", 1)
	if _, err := ImportHostRun([]byte(permitted), nil); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing permitted mutation error = %v", err)
	}
	permitted = strings.Replace(permitted, `"mutating_requests":0`, `"mutating_requests":1`, 1)
	if _, err := ImportHostRun([]byte(permitted), nil); err != nil {
		t.Fatalf("permitted mutation: %v", err)
	}

	wiki := string(portableRecord(StatusPassed, "true", `"manual_corrections":0,"clarifications":0`, `"timed_out":false`, "codex"))
	wiki = strings.Replace(wiki, "discovery.operation-inspect", "wiki.lifecycle", 1)
	wiki = strings.Replace(wiki, `"mutating_requests":0`, `"mutating_requests":4`, 1)
	if _, err := ImportHostRun([]byte(wiki), nil); err != nil {
		t.Fatalf("wiki lifecycle mutations: %v", err)
	}
}

func TestPortableAgentPacketDescribesEveryCatalogScenario(t *testing.T) {
	packet := PortableAgentPacket()
	if packet.SchemaVersion != AgentPacketSchemaVersion || packet.BenchmarkSchemaVersion != SchemaVersion || len(packet.Scenarios) != len(catalogScenarios) {
		t.Fatalf("packet metadata = %+v scenarios = %d", packet, len(packet.Scenarios))
	}
	for _, scenario := range packet.Scenarios {
		if scenario.ID == "" || scenario.DelegatedOutcome == "" || len(scenario.StartingContext) == 0 || len(scenario.ForbiddenMutations) == 0 || len(scenario.CompletionEvidence) == 0 {
			t.Fatalf("incomplete scenario contract = %+v", scenario)
		}
	}
	permitted := packetScenarioByID(t, packet, "mutation.permitted-repo-edit")
	if len(permitted.PermittedMutations) != 1 || !strings.Contains(permitted.PermittedMutations[0], "PATCH /api/v1/repos/benchmark/target") {
		t.Fatalf("permitted mutation contract = %+v", permitted)
	}
	wiki := packetScenarioByID(t, packet, "wiki.lifecycle")
	if len(wiki.PermittedMutations) != 4 || !strings.Contains(strings.Join(wiki.PermittedMutations, "\n"), "DELETE /api/v1/repos/benchmark/target/wiki/page/Dogfood") {
		t.Fatalf("wiki mutation contract = %+v", wiki)
	}
	readOnly := packetScenarioByID(t, packet, "mutation.dry-run")
	if len(readOnly.PermittedMutations) != 0 {
		t.Fatalf("dry-run mutation contract = %+v", readOnly)
	}
	inspection := packetScenarioByID(t, packet, "inspection.repository-toon")
	joinedOracle := strings.Join(inspection.CompletionEvidence, "\n")
	for _, exact := range []string{"format=toon", "max_bytes=1024", "full_name: benchmark/target", "GET /api/v1/repos/benchmark/target"} {
		if !strings.Contains(joinedOracle, exact) {
			t.Fatalf("inspection oracle lacks %q: %+v", exact, inspection.CompletionEvidence)
		}
	}
}

func packetScenarioByID(t *testing.T, packet AgentPacket, id string) AgentScenarioContract {
	t.Helper()
	for _, scenario := range packet.Scenarios {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("packet scenario %q not found", id)
	return AgentScenarioContract{}
}

func portableRecord(status, completion, metrics, safety, host string) []byte {
	return []byte(fmt.Sprintf(`{
  "schema_version":"1",
  "benchmark_schema_version":"5",
  "catalog_revision":"6",
  "fjgo":{"version":"1.2.0","source_revision":"abc123"},
  "host":{"name":%q,"version":"host-version"},
  "model":{"provider":"provider","name":"model","version":"model-version"},
  "scenario":{"id":"discovery.operation-inspect","status":%q,"completion_satisfied":%s},
  "metrics":{"cli_invocations":1,"api_requests":0,"stdout_bytes":10,"stderr_bytes":0,%s,"input_tokens":8,"output_tokens":4},
  "safety":{"unexpected_requests":0,"mutating_requests":0,"unsafe_requests":0,"credential_leaks":0,%s},
  "evidence":[{"id":"result.json","kind":"host-artifact"}]
}`, host, status, completion, metrics, safety))
}
