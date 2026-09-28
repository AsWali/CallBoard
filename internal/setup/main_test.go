package setup

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "callboard-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Setenv("GIT_AUTHOR_NAME", "t")
	os.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	os.Setenv("GIT_COMMITTER_NAME", "t")
	os.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
