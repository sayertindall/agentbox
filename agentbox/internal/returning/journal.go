package returning

import (
	"fmt"
	"os"
	"path/filepath"

	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/store"
)

type Journal struct {
	Dir       string
	Original  string
	Local     string
	Candidate Candidate
	Hook      func(string)
	FailApply bool
	Confirmed bool
}

func (j Journal) Apply() error {
	j.hook("verify")
	orig, err := manifest.Build(j.Original, manifest.Policy{})
	if err != nil {
		return err
	}
	local, err := manifest.Build(j.Local, manifest.Policy{})
	if err != nil {
		return err
	}
	if orig.SHA256 != local.SHA256 {
		return fmt.Errorf("local source diverged from the original handoff; conflict")
	}
	j.hook("journal")
	backup := filepath.Join(j.Dir, "backup")
	if err := os.MkdirAll(backup, 0o700); err != nil {
		return err
	}
	if err := copyTree(j.Local, backup); err != nil {
		return err
	}
	j.hook("apply")
	if j.FailApply {
		_ = copyTree(backup, j.Local)
		return fmt.Errorf("apply failed")
	}
	root, err := os.OpenRoot(j.Local)
	if err != nil {
		_ = copyTree(backup, j.Local)
		return err
	}
	defer root.Close()
	if err := manifest.Materialize(j.Candidate.Dir, j.Candidate.Manifest, root); err != nil {
		_ = copyTree(backup, j.Local)
		return err
	}
	return nil
}

func (j Journal) Resolve(confirm bool) (store.ReceiptState, error) {
	if !confirm {
		return "", fmt.Errorf("resolve requires explicit confirmation")
	}
	return store.LocalOwned, nil
}

func (j Journal) hook(event string) {
	if j.Hook != nil {
		j.Hook(event)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
