package fjgoskill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skill/fjgo
var embedded embed.FS

type InstallResult struct {
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type Status struct {
	Path      string   `json:"path"`
	Installed bool     `json:"installed"`
	Current   bool     `json:"current"`
	Missing   []string `json:"missing,omitempty"`
	Outdated  []string `json:"outdated,omitempty"`
}

func DefaultInstallDir() string {
	return filepath.Join(".agents", "skills", "fjgo")
}

func Check(dir string) Status {
	if dir == "" {
		dir = DefaultInstallDir()
	}
	status := Status{Path: dir, Installed: true, Current: true}
	for _, rel := range skillFiles() {
		want, err := embedded.ReadFile(filepath.ToSlash(filepath.Join("skill/fjgo", rel)))
		if err != nil {
			status.Installed = false
			status.Current = false
			status.Missing = append(status.Missing, filepath.ToSlash(rel))
			continue
		}
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			status.Installed = false
			status.Current = false
			status.Missing = append(status.Missing, filepath.ToSlash(rel))
			continue
		}
		if string(got) != string(want) {
			status.Current = false
			status.Outdated = append(status.Outdated, filepath.ToSlash(rel))
		}
	}
	return status
}

func skillFiles() []string {
	return []string{"SKILL.md", "references/workflows.md", "agents/openai.yaml"}
}

func Install(dir string, force bool) (InstallResult, error) {
	if dir == "" {
		dir = DefaultInstallDir()
	}
	if _, err := os.Stat(dir); err == nil && !force {
		return InstallResult{}, fmt.Errorf("%s already exists; rerun with --force to overwrite embedded skill files", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	result := InstallResult{Path: dir}
	err := fs.WalkDir(embedded, "skill/fjgo", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "skill/fjgo")
		if rel == "" {
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return err
		}
		result.Files = append(result.Files, filepath.ToSlash(filepath.Clean(target)))
		return nil
	})
	return result, err
}
