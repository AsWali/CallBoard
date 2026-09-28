package main

import (
	"flag"
	"fmt"

	"github.com/AsWali/CallBoard/internal/overview"
	"github.com/AsWali/CallBoard/internal/store"
)

func branches(args []string) error {
	fs := flag.NewFlagSet("branches", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	st := store.Open(wd())
	if st.Common == "" {
		return fmt.Errorf("not a git repo: there are no branches to compare")
	}
	p := overview.BuildBranches(st.Repo, st.Branch())
	if *asJSON {
		return printJSON(p)
	}
	fmt.Print(overview.Format(p))
	return nil
}
