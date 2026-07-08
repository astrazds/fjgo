package fjgoskill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesEmbeddedSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fjgo")
	result, err := Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != dir || len(result.Files) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "references", "workflows.md")); err != nil {
		t.Fatal(err)
	}
	status := Check(dir)
	if !status.Installed || !status.Current || len(status.Missing) != 0 || len(status.Outdated) != 0 {
		t.Fatalf("status = %#v", status)
	}
	if _, err := Install(dir, false); err == nil {
		t.Fatal("expected existing install to require --force")
	}
}

func TestCheckReportsMissingSkill(t *testing.T) {
	status := Check(filepath.Join(t.TempDir(), "missing"))
	if status.Installed || status.Current || len(status.Missing) == 0 {
		t.Fatalf("status = %#v", status)
	}
}

func TestCheckReportsOutdatedSkill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fjgo")
	if _, err := Install(dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := Check(dir)
	if !status.Installed || status.Current || len(status.Outdated) != 1 || status.Outdated[0] != "SKILL.md" {
		t.Fatalf("status = %#v", status)
	}
}

func TestEmbeddedSkillDocumentsAxiSetup(t *testing.T) {
	b, err := embedded.ReadFile("skill/fjgo/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"AXI-shaped", "fjgo setup hooks", "TOON", "--full"} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill missing %q:\n%s", want, text)
		}
	}
}

func TestEmbeddedSkillStaticGuidanceCurrent(t *testing.T) {
	current, want, err := EmbeddedGuidanceCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if !current {
		t.Fatalf("embedded static guidance drifted; expected block:\n%s", want)
	}
}
