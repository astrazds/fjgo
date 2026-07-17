package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompareBaselinesReportsScenarioLevelDeltas(t *testing.T) {
	baseline := comparisonCatalog(
		comparisonResult("removed", StatusPassed, 1),
		comparisonResult("improved", StatusFailed, 1),
		comparisonResult("regressed", StatusPassed, 1),
		comparisonResult("changed", StatusPassed, 1),
	)
	current := comparisonCatalog(
		comparisonResult("improved", StatusPassed, 1),
		comparisonResult("regressed", StatusFailed, 1),
		comparisonResult("changed", StatusPassed, 2),
		comparisonResult("added", StatusPassed, 1),
	)

	comparison, err := CompareBaselines(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	want := []ScenarioDelta{
		{ScenarioID: "added", Change: ChangeAdded, AfterStatus: StatusPassed},
		{ScenarioID: "changed", Change: ChangeChanged, BeforeStatus: StatusPassed, AfterStatus: StatusPassed},
		{ScenarioID: "improved", Change: ChangeImproved, BeforeStatus: StatusFailed, AfterStatus: StatusPassed},
		{ScenarioID: "regressed", Change: ChangeRegressed, BeforeStatus: StatusPassed, AfterStatus: StatusFailed},
		{ScenarioID: "removed", Change: ChangeRemoved, BeforeStatus: StatusPassed},
	}
	if comparison.SchemaVersion != ComparisonSchemaVersion || !reflect.DeepEqual(comparison.Deltas, want) {
		t.Fatalf("comparison = %+v, want deltas %+v", comparison, want)
	}
	if comparison.Counts != (DeltaCounts{Added: 1, Removed: 1, Improved: 1, Regressed: 1, Changed: 1}) {
		t.Fatalf("counts = %+v", comparison.Counts)
	}
}

func TestCompareBaselinesRejectsSchemaMismatch(t *testing.T) {
	baseline := comparisonCatalog(comparisonResult("scenario", StatusPassed, 1))
	baseline.SchemaVersion = "older"
	_, err := CompareBaselines(baseline, comparisonCatalog(comparisonResult("scenario", StatusPassed, 1)))
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("schema mismatch error = %v", err)
	}
}

func TestCompareBaselinesDoesNotTurnCatalogRevisionIntoScenarioChanges(t *testing.T) {
	baseline := comparisonCatalog(comparisonResult("scenario", StatusPassed, 1))
	baseline.CatalogRevision = "older-catalog"
	baseline.Results[0].CatalogRevision = "older-catalog"
	comparison, err := CompareBaselines(baseline, comparisonCatalog(comparisonResult("scenario", StatusPassed, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if comparison.BaselineCatalogRevision != "older-catalog" || comparison.CurrentCatalogRevision != CatalogRevision || len(comparison.Deltas) != 0 {
		t.Fatalf("catalog-only comparison = %+v", comparison)
	}
}

func TestCheckBaselineRejectsStaleResultAndSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	baseline := comparisonCatalog(comparisonResult("scenario", StatusPassed, 1))
	if err := WriteBaseline(path, baseline); err != nil {
		t.Fatal(err)
	}
	current := comparisonCatalog(comparisonResult("scenario", StatusPassed, 2))
	if err := CheckBaseline(path, current); err == nil || !strings.Contains(err.Error(), "baseline artifact is stale") {
		t.Fatalf("stale baseline error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"schema_version": "`+SchemaVersion+`"`, `"schema_version": "older"`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckBaseline(path, baseline); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("schema check error = %v", err)
	}
}

func TestSummaryIsDerivedFromBaselineJSONAndSelectionIsExplicit(t *testing.T) {
	catalog := comparisonCatalog(
		comparisonResult("first", StatusPassed, 1),
		comparisonResult("second", StatusFailed, 2),
	)
	catalog.Results[0].Scenario.Category = "read"
	catalog.Results[1].Scenario.Category = "write"
	selected, err := FilterCatalog(catalog, []string{"second"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeBaseline(selected)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := SummaryFromBaselineJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(summary), "first") || !strings.Contains(string(summary), "second") || !strings.Contains(string(summary), "| failed |") {
		t.Fatalf("selected summary:\n%s", summary)
	}
	byCategory, err := FilterCatalog(catalog, nil, []string{"read"})
	if err != nil || len(byCategory.Results) != 1 || byCategory.Results[0].Scenario.ID != "first" {
		t.Fatalf("category selection = %+v, err = %v", byCategory.Results, err)
	}
	if _, err := FilterCatalog(catalog, []string{"missing"}, nil); err == nil {
		t.Fatal("unknown scenario selection succeeded")
	}
}

func TestNormalizationAndEncodingRemoveVolatileValues(t *testing.T) {
	result := comparisonCatalog(comparisonResult("scenario", StatusPassed, 1))
	result.SourceRevision = "abc123"
	result.Results[0].SourceRevision = "abc123"
	result.Results[0].Scenario.DelegatedOutcome = "fixture http://127.0.0.1:43123 path /tmp/fjgo-benchmark-a8Z/run at 2026-07-17T15:00:01+10:00 id 123e4567-e89b-12d3-a456-426614174000"

	encoded, err := EncodeBaseline(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, volatile := range []string{"abc123", "43123", "a8Z", "2026-07-17T15:00:01+10:00", "123e4567-e89b-12d3-a456-426614174000"} {
		if strings.Contains(string(encoded), volatile) {
			t.Fatalf("baseline retained volatile %q:\n%s", volatile, encoded)
		}
	}
	for _, placeholder := range []string{SourceRevisionPlaceholder, FixtureBaseURLPlaceholder, "{workdir}", "{timestamp}", "{random_id}"} {
		if !strings.Contains(string(encoded), placeholder) {
			t.Fatalf("baseline lacks placeholder %q:\n%s", placeholder, encoded)
		}
	}
	var decoded CatalogResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	second, err := EncodeBaseline(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(second) {
		t.Fatalf("encoding is not idempotent:\nfirst:\n%s\nsecond:\n%s", encoded, second)
	}
	normalized := normalizeJSONValue(map[string]any{
		"informational_duration_ms": float64(918), "informational_duration": "2.4s",
		"request_timeout_duration_ms": float64(5000), "random_id": "run-a8Z19",
	}, "").(map[string]any)
	if normalized["informational_duration_ms"] != float64(0) || normalized["informational_duration"] != "{duration}" ||
		normalized["request_timeout_duration_ms"] != float64(5000) || normalized["random_id"] != "{random_id}" {
		t.Fatalf("informational duration normalization = %#v", normalized)
	}
	for _, path := range []string{
		"/private/var/folders/xy/T/fjgo-benchmark-a8Z/run",
		`C:\Users\agent\AppData\Local\Temp\fjgo-benchmark-a8Z\run`,
	} {
		if got := normalizeVolatileText(path); !strings.HasPrefix(got, "{workdir}") {
			t.Fatalf("portable temp path %q normalized to %q", path, got)
		}
	}
	if got := normalizeVolatileText("fixture localhost:43123"); got != "fixture {fixture_host}" {
		t.Fatalf("fixture host normalization = %q", got)
	}
}

func comparisonCatalog(results ...Result) CatalogResult {
	return CatalogResult{SchemaVersion: SchemaVersion, CatalogRevision: CatalogRevision, SourceRevision: "test", Results: results}
}

func comparisonResult(id, status string, invocations int) Result {
	return Result{
		SchemaVersion: SchemaVersion, CatalogRevision: CatalogRevision, SourceRevision: "test",
		Scenario: ScenarioResult{ID: id, Status: status, Completion: Completion{Evidence: []string{}}},
		Metrics:  Metrics{CLIInvocations: invocations}, Commands: []CommandSummary{}, Requests: []RequestSummary{}, Evidence: []EvidenceRef{},
	}
}
