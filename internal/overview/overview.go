package overview

import (
	"os"
	"path/filepath"

	"github.com/AsWali/CallBoard/internal/gitx"
	"github.com/AsWali/CallBoard/internal/store"
)

func Read(r gitx.Repo, branch, worktree string, k store.Kind) string {
	if worktree != "" {
		if b, err := os.ReadFile(filepath.Join(worktree, k.File)); err == nil {
			return string(b)
		}
		return ""
	}
	s, _ := r.Show("refs/heads/"+branch, k.File)
	return s
}
