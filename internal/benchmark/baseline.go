package benchmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SourceRevisionPlaceholder = "{source_revision}"
	maxBaselineArtifactSize   = 4 << 20
	maxMarkdownSummarySize    = 64 << 10
	ComparisonSchemaVersion   = "1"

	ChangeAdded     = "added"
	ChangeRemoved   = "removed"
	ChangeImproved  = "improved"
	ChangeRegressed = "regressed"
	ChangeChanged   = "changed"
)

var (
	fixtureURLPattern  = regexp.MustCompile(`https?://(?:127\.0\.0\.1|localhost|\[::1\]):[0-9]+`)
	fixtureHostPattern = regexp.MustCompile(`(?:127\.0\.0\.1|localhost|\[::1\]):[0-9]+`)
	tempPathPattern    = regexp.MustCompile(`(?i)(?:[a-z]:[\\/]|/)(?:[^/\\"\s]+[\\/])*fjgo-benchmark-[A-Za-z0-9._-]+`)
	timestampPattern   = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-][0-9]{2}:[0-9]{2})`)
	randomIDPattern    = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
)

type Comparison struct {
	SchemaVersion           string          `json:"schema_version"`
	BaselineCatalogRevision string          `json:"baseline_catalog_revision"`
	CurrentCatalogRevision  string          `json:"current_catalog_revision"`
	Counts                  DeltaCounts     `json:"counts"`
	Deltas                  []ScenarioDelta `json:"deltas"`
}

type DeltaCounts struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Improved  int `json:"improved"`
	Regressed int `json:"regressed"`
	Changed   int `json:"changed"`
}

type ScenarioDelta struct {
	ScenarioID   string `json:"scenario_id"`
	Change       string `json:"change"`
	BeforeStatus string `json:"before_status,omitempty"`
	AfterStatus  string `json:"after_status,omitempty"`
}

func CatalogFromResult(result Result) CatalogResult {
	return CatalogResult{
		SchemaVersion: result.SchemaVersion, SourceRevision: result.SourceRevision,
		CatalogRevision: result.CatalogRevision, Results: []Result{result},
	}
}

func FilterCatalog(result CatalogResult, scenarioIDs, categories []string) (CatalogResult, error) {
	if len(scenarioIDs) == 0 && len(categories) == 0 {
		return result, nil
	}
	wantedIDs := make(map[string]bool, len(scenarioIDs))
	wantedCategories := make(map[string]bool, len(categories))
	for _, id := range scenarioIDs {
		wantedIDs[id] = true
	}
	for _, category := range categories {
		wantedCategories[category] = true
	}
	filtered := result
	filtered.Results = make([]Result, 0, len(result.Results))
	matchedIDs := map[string]bool{}
	matchedCategories := map[string]bool{}
	for _, scenario := range result.Results {
		idMatch := wantedIDs[scenario.Scenario.ID]
		categoryMatch := wantedCategories[scenario.Scenario.Category]
		if idMatch || categoryMatch {
			filtered.Results = append(filtered.Results, scenario)
			matchedIDs[scenario.Scenario.ID] = idMatch
			matchedCategories[scenario.Scenario.Category] = categoryMatch
		}
	}
	for _, id := range scenarioIDs {
		if !matchedIDs[id] {
			return CatalogResult{}, fmt.Errorf("unknown benchmark scenario %q", id)
		}
	}
	for _, category := range categories {
		if !matchedCategories[category] {
			return CatalogResult{}, fmt.Errorf("unknown benchmark category %q", category)
		}
	}
	return filtered, nil
}

func NormalizeCatalog(result CatalogResult) (CatalogResult, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return CatalogResult{}, err
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return CatalogResult{}, err
	}
	generic = normalizeJSONValue(generic, "")
	encoded, err = json.Marshal(generic)
	if err != nil {
		return CatalogResult{}, err
	}
	var normalized CatalogResult
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return CatalogResult{}, err
	}
	normalized.SourceRevision = SourceRevisionPlaceholder
	for i := range normalized.Results {
		normalized.Results[i].SourceRevision = SourceRevisionPlaceholder
	}
	return normalized, nil
}

func normalizeJSONValue(value any, key string) any {
	switch value := value.(type) {
	case map[string]any:
		for childKey, child := range value {
			value[childKey] = normalizeJSONValue(child, childKey)
		}
		return value
	case []any:
		for i := range value {
			value[i] = normalizeJSONValue(value[i], key)
		}
		return value
	case string:
		if informationalDurationField(key) {
			return "{duration}"
		}
		if volatileIdentifierField(key) {
			return "{random_id}"
		}
		return normalizeVolatileText(value)
	case float64:
		if informationalDurationField(key) {
			return float64(0)
		}
		return value
	default:
		return value
	}
}

func normalizeVolatileText(value string) string {
	value = fixtureURLPattern.ReplaceAllString(value, FixtureBaseURLPlaceholder)
	value = fixtureHostPattern.ReplaceAllString(value, "{fixture_host}")
	value = tempPathPattern.ReplaceAllString(value, "{workdir}")
	value = timestampPattern.ReplaceAllString(value, "{timestamp}")
	return randomIDPattern.ReplaceAllString(value, "{random_id}")
}

func informationalDurationField(key string) bool {
	switch strings.ToLower(key) {
	case "informational_duration", "informational_duration_ms":
		return true
	default:
		return false
	}
}

func volatileIdentifierField(key string) bool {
	switch strings.ToLower(key) {
	case "random_id":
		return true
	default:
		return false
	}
}

func CompareBaselines(baseline, current CatalogResult) (Comparison, error) {
	if err := validateBaselineMetadata(baseline); err != nil {
		return Comparison{}, err
	}
	if err := validateBaselineMetadata(current); err != nil {
		return Comparison{}, fmt.Errorf("current result: %w", err)
	}
	baseline, err := NormalizeCatalog(baseline)
	if err != nil {
		return Comparison{}, err
	}
	current, err = NormalizeCatalog(current)
	if err != nil {
		return Comparison{}, err
	}
	before := resultsByScenarioID(baseline.Results)
	after := resultsByScenarioID(current.Results)
	ids := make([]string, 0, len(before)+len(after))
	seen := map[string]bool{}
	for id := range before {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range after {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	comparison := Comparison{
		SchemaVersion: ComparisonSchemaVersion, BaselineCatalogRevision: baseline.CatalogRevision,
		CurrentCatalogRevision: current.CatalogRevision, Deltas: []ScenarioDelta{},
	}
	for _, id := range ids {
		oldResult, hadOld := before[id]
		newResult, hasNew := after[id]
		delta := ScenarioDelta{ScenarioID: id}
		switch {
		case !hadOld:
			delta.Change, delta.AfterStatus = ChangeAdded, newResult.Scenario.Status
			comparison.Counts.Added++
		case !hasNew:
			delta.Change, delta.BeforeStatus = ChangeRemoved, oldResult.Scenario.Status
			comparison.Counts.Removed++
		case resultEqual(oldResult, newResult):
			continue
		default:
			delta.BeforeStatus, delta.AfterStatus = oldResult.Scenario.Status, newResult.Scenario.Status
			oldRank, newRank := statusRank(delta.BeforeStatus), statusRank(delta.AfterStatus)
			switch {
			case newRank > oldRank:
				delta.Change = ChangeImproved
				comparison.Counts.Improved++
			case newRank < oldRank:
				delta.Change = ChangeRegressed
				comparison.Counts.Regressed++
			default:
				delta.Change = ChangeChanged
				comparison.Counts.Changed++
			}
		}
		comparison.Deltas = append(comparison.Deltas, delta)
	}
	return comparison, nil
}

func resultsByScenarioID(results []Result) map[string]Result {
	indexed := make(map[string]Result, len(results))
	for _, result := range results {
		indexed[result.Scenario.ID] = result
	}
	return indexed
}

func resultEqual(left, right Result) bool {
	left.SchemaVersion, right.SchemaVersion = "", ""
	left.SourceRevision, right.SourceRevision = "", ""
	left.CatalogRevision, right.CatalogRevision = "", ""
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func statusRank(status string) int {
	switch status {
	case StatusPassed:
		return 7
	case StatusRecovered:
		return 6
	case StatusManuallyCorrected:
		return 5
	case StatusClarificationRequired, StatusIncomplete:
		return 4
	case StatusFailed:
		return 3
	case StatusTimedOut:
		return 2
	default:
		return 1
	}
}

func EncodeBaseline(result CatalogResult) ([]byte, error) {
	if err := validateBaselineMetadata(result); err != nil {
		return nil, err
	}
	normalized, err := NormalizeCatalog(result)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded, leaks := sanitizeRetainedArtifact(encoded, fixtureCredentialCanary)
	if leaks > 0 {
		return nil, fmt.Errorf("baseline contained %d credential canary leak(s)", leaks)
	}
	return append(encoded, '\n'), nil
}

func SummaryFromBaselineJSON(baselineJSON []byte) ([]byte, error) {
	if len(baselineJSON) > maxBaselineArtifactSize {
		return nil, fmt.Errorf("baseline exceeds %d bytes", maxBaselineArtifactSize)
	}
	result, err := decodeBaseline(baselineJSON)
	if err != nil {
		return nil, err
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "# fjgo benchmark baseline\n\nSchema: `%s`  \nCatalog: `%s`  \nScenarios: %d\n\n", result.SchemaVersion, result.CatalogRevision, len(result.Results))
	summary.WriteString("| Status | Scenario | CLI | API | Unsafe | Leaks |\n")
	summary.WriteString("| --- | --- | ---: | ---: | ---: | ---: |\n")
	for _, scenario := range result.Results {
		fmt.Fprintf(&summary, "| %s | `%s` | %d | %d | %d | %d |\n",
			scenario.Scenario.Status, escapeMarkdownCode(scenario.Scenario.ID),
			scenario.Metrics.CLIInvocations, scenario.Metrics.APIRequests,
			scenario.Safety.UnsafeRequests, scenario.Safety.CredentialLeaks)
		if summary.Len() > maxMarkdownSummarySize {
			return nil, fmt.Errorf("Markdown summary exceeds %d bytes", maxMarkdownSummarySize)
		}
	}
	sanitized, leaks := sanitizeRetainedArtifact([]byte(summary.String()), fixtureCredentialCanary)
	if leaks > 0 {
		return nil, fmt.Errorf("Markdown summary contained %d credential canary leak(s)", leaks)
	}
	return sanitized, nil
}

func WriteBaseline(path string, result CatalogResult) error {
	baselineJSON, err := EncodeBaseline(result)
	if err != nil {
		return err
	}
	summary, err := SummaryFromBaselineJSON(baselineJSON)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, baselineJSON, 0o644); err != nil {
		return err
	}
	return os.WriteFile(summaryPath(path), summary, 0o644)
}

func CheckBaseline(path string, current CatalogResult) error {
	wantJSON, _, err := readBaseline(path)
	if err != nil {
		return err
	}
	currentJSON, err := EncodeBaseline(current)
	if err != nil {
		return err
	}
	if !bytes.Equal(wantJSON, currentJSON) {
		return errors.New("baseline artifact is stale")
	}
	wantSummary, err := readBaselineArtifact(summaryPath(path))
	if err != nil {
		return err
	}
	currentSummary, err := SummaryFromBaselineJSON(currentJSON)
	if err != nil {
		return err
	}
	if !bytes.Equal(wantSummary, currentSummary) {
		return errors.New("summary artifact is stale")
	}
	return nil
}

func ReadBaseline(path string) (CatalogResult, error) {
	_, result, err := readBaseline(path)
	return result, err
}

func readBaseline(path string) ([]byte, CatalogResult, error) {
	data, err := readBaselineArtifact(path)
	if err != nil {
		return nil, CatalogResult{}, err
	}
	result, err := decodeBaseline(data)
	return data, result, err
}

func decodeBaseline(data []byte) (CatalogResult, error) {
	var result CatalogResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CatalogResult{}, fmt.Errorf("decode baseline: %w", err)
	}
	if err := validateBaselineMetadata(result); err != nil {
		return CatalogResult{}, err
	}
	return result, nil
}

func validateBaselineMetadata(result CatalogResult) error {
	if result.SchemaVersion != SchemaVersion {
		return fmt.Errorf("baseline schema version %q is incompatible with %q", result.SchemaVersion, SchemaVersion)
	}
	if result.CatalogRevision == "" {
		return errors.New("baseline catalog revision is required")
	}
	if len(result.Results) == 0 {
		return errors.New("baseline contains no scenarios")
	}
	seen := make(map[string]bool, len(result.Results))
	for _, scenario := range result.Results {
		if scenario.SchemaVersion != result.SchemaVersion {
			return fmt.Errorf("scenario %q schema version %q does not match baseline %q", scenario.Scenario.ID, scenario.SchemaVersion, result.SchemaVersion)
		}
		if scenario.CatalogRevision != result.CatalogRevision {
			return fmt.Errorf("scenario %q catalog revision %q does not match baseline %q", scenario.Scenario.ID, scenario.CatalogRevision, result.CatalogRevision)
		}
		if scenario.Scenario.ID == "" {
			return errors.New("baseline scenario ID is required")
		}
		if seen[scenario.Scenario.ID] {
			return fmt.Errorf("baseline contains duplicate scenario %q", scenario.Scenario.ID)
		}
		seen[scenario.Scenario.ID] = true
	}
	return nil
}

func readBaselineArtifact(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBaselineArtifactSize {
		return nil, fmt.Errorf("artifact %s exceeds %d bytes", path, maxBaselineArtifactSize)
	}
	return os.ReadFile(path)
}

func summaryPath(path string) string {
	extension := filepath.Ext(path)
	return strings.TrimSuffix(path, extension) + ".md"
}

func escapeMarkdownCode(value string) string {
	return strings.ReplaceAll(value, "`", "\\`")
}
