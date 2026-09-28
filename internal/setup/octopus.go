package setup

import (
	"os"
	"path/filepath"
	"runtime"
)

const strategyScript = `#!/bin/sh
if callboard octopus --check >/dev/null 2>&1; then exec callboard octopus "$@"; fi
exec git merge-octopus "$@"
`

func StrategyPath() string { return filepath.Join(filepath.Dir(LinkPath()), "git-merge-callboard") }

func writeStrategy() (bool, error) {
	p := StrategyPath()
	if readString(p) == strategyScript {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(p, []byte(strategyScript), 0o755); err != nil {
		return false, err
	}
	return true, os.Chmod(p, 0o755)
}

type gitConfig func(args ...string) (string, error)

func octopusReady(cfg gitConfig) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	cur, _ := cfg("--get", "pull.octopus")
	switch cur {
	case "":
		return !onPath(filepath.Dir(StrategyPath()))
	case "callboard":
		return readString(StrategyPath()) == strategyScript
	}
	return true
}

func setOctopus(s *Step, cfg gitConfig, where string) {
	if runtime.GOOS == "windows" {
		return
	}
	cur, _ := cfg("--get", "pull.octopus")
	if cur != "" && cur != "callboard" {
		s.same("git: pull.octopus is your own (%s), so merges of several branches at once were left alone", cur)
		return
	}
	if dir := filepath.Dir(StrategyPath()); cur == "" && !onPath(dir) {
		s.same("git: %s isn't on your PATH, so merges of several branches at once stay git's own", tilde(dir))
		return
	}
	wrote, err := writeStrategy()
	if err != nil {
		s.bad("couldn't write %s: %v", tilde(StrategyPath()), err)
		return
	}
	if cur == "callboard" && !wrote {
		s.same("git: merges of several branches at once go through callboard (already there)")
		return
	}
	if _, err := cfg("pull.octopus", "callboard"); err != nil {
		s.bad("couldn't set pull.octopus: git config %s pull.octopus callboard", where)
		return
	}
	s.ok("git: merges of several branches at once (git merge a b) merge the lists per item too (%s, pull.octopus in %s)", tilde(StrategyPath()), where)
}

func dropOctopus(s *Step, cfg gitConfig, where string) {
	if cur, _ := cfg("--get", "pull.octopus"); cur == "callboard" {
		cfg("--unset", "pull.octopus")
		s.ok("git: removed pull.octopus from %s", where)
	}
}
