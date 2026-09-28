package gitx

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Commit struct {
	Sha     string    `json:"sha"`
	Short   string    `json:"short"`
	Subject string    `json:"subject"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Parents []string  `json:"-"`

	Merged string `json:"merged,omitempty"`
}

var (
	mergeBranch = regexp.MustCompile(`^Merge (?:remote-tracking )?branch '([^']+)'`)
	mergePR     = regexp.MustCompile(`^Merge pull request (#\d+) from [^/\s]+/(\S+)`)
)

func (r Repo) Log(args ...string) []Commit {
	out, err := r.Git(append([]string{"log", "--format=%H%x1f%h%x1f%ct%x1f%an%x1f%P%x1f%s"}, args...)...)
	if err != nil || out == "" {
		return nil
	}
	var cs []Commit
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\x1f", 6)
		if len(f) < 6 {
			continue
		}
		ts, _ := strconv.ParseInt(f[2], 10, 64)
		c := Commit{Sha: f[0], Short: f[1], At: time.Unix(ts, 0), By: f[3], Parents: strings.Fields(f[4]), Subject: f[5]}
		if len(c.Parents) > 1 {
			switch m, p := mergeBranch.FindStringSubmatch(c.Subject), mergePR.FindStringSubmatch(c.Subject); {
			case m != nil:
				c.Merged = m[1]
			case p != nil:
				c.Merged = p[2] + " (" + p[1] + ")"
			default:
				if n, err := r.Git("name-rev", "--name-only", "--no-undefined", c.Parents[1]); err == nil {
					c.Merged = strings.TrimPrefix(strings.FieldsFunc(n, func(c rune) bool { return c == '~' || c == '^' })[0], "remotes/")
				} else {
					c.Merged = c.Parents[1][:7]
				}
			}
		}
		cs = append(cs, c)
	}
	return cs
}

func (r Repo) Commit(ref string) Commit {
	if cs := r.Log("-1", ref, "--"); len(cs) > 0 {
		return cs[0]
	}
	return Commit{}
}

func (r Repo) Count(rng string) int {
	s, _ := r.Git("rev-list", "--count", rng, "--")
	n, _ := strconv.Atoi(s)
	return n
}

func (r Repo) ShowMany(specs []string) map[string]string {
	out := map[string]string{}
	if len(specs) == 0 {
		return out
	}
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = r.Root
	cmd.Stdin = strings.NewReader(strings.Join(specs, "\n") + "\n")
	b, err := cmd.Output()
	if err != nil {
		return out
	}
	rd := bufio.NewReader(bytes.NewReader(b))
	for _, spec := range specs {
		head, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		f := strings.Fields(head)
		if len(f) != 3 {
			continue
		}
		n, _ := strconv.Atoi(f[2])
		body := make([]byte, n+1)
		if _, err := io.ReadFull(rd, body); err != nil {
			break
		}
		if f[1] == "blob" {
			out[spec] = string(body[:n])
		}
	}
	return out
}
