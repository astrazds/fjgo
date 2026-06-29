package fjgoskill

import (
	"os"
	"path/filepath"
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
	if _, err := Install(dir, false); err == nil {
		t.Fatal("expected existing install to require --force")
	}
}
