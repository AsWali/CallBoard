package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const settings = `{
    "permissions": {
        "allow": ["Bash(ls)"]
    },
    "zeta": 1,
    "alpha": true,
    "hooks": {
        "Stop": [
            {"hooks": [{"type": "command", "command": "say done"}]}
        ]
    }
}
`

func TestEditJSONKeepsOrderAndIndent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(settings), 0o644)
	add := func(m *Obj) {
		for _, h := range hooks {
			if !hasHook(m, h[0], h[1]) {
				cmd := newObj()
				cmd.Set("type", "command")
				cmd.Set("command", h[1])
				e := newObj()
				e.Set("hooks", []any{cmd})
				m.Child("hooks").Set(h[0], append(m.Child("hooks").List(h[0]), e))
			}
		}
	}
	changed, err := editJSON(p, add)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if strings.Index(s, `"zeta"`) > strings.Index(s, `"alpha"`) || strings.Index(s, `"Stop"`) > strings.Index(s, `"SessionStart"`) {
		t.Fatalf("keys reordered:\n%s", s)
	}
	if !strings.Contains(s, "\n    \"zeta\": 1,") {
		t.Fatalf("indent not kept:\n%s", s)
	}
	if changed, _ := editJSON(p, add); changed {
		t.Fatal("second run changed the file")
	}

	editJSON(p, func(m *Obj) {
		hs := m.ChildIf("hooks")
		for _, h := range hooks {
			hs.Delete(h[0])
		}
	})
	o, err := readJSON(p)
	if err != nil || !hasHook(o, "Stop", "say done") || hasHook(o, "SessionStart", "callboard") {
		t.Fatalf("after removal: %v", err)
	}
}

func TestEditJSONRemovesEmptyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(p, []byte(`{"a": 1}`), 0o644)
	editJSON(p, func(m *Obj) { m.Delete("a") })
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("empty file kept")
	}
	os.WriteFile(p, []byte(`{"a": `), 0o644)
	if _, err := editJSON(p, func(*Obj) {}); err == nil || !strings.Contains(err.Error(), "isn't valid JSON") {
		t.Fatalf("bad JSON: %v", err)
	}
}

func TestEditJSONKeepsHowValuesAreWritten(t *testing.T) {
	const orig = `{
  "permissions": {"allow": ["Bash(npm run build && npm test)", "Read"], "deny": []},
  "enabledMcpjsonServers": ["other", "callboard"],
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "say done"}]}
    ]
  }
}
`
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(orig), 0o644)
	if _, err := editJSON(p, addHooks); err != nil {
		t.Fatal(err)
	}
	if _, err := editJSON(p, dropHooks); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != orig {
		t.Fatalf("adding and removing the hooks changed the file:\n%s", b)
	}
	editJSON(p, func(m *Obj) {
		m.Set("enabledMcpjsonServers", []any{"other"})
	})
	b, _ = os.ReadFile(p)
	if want := strings.Replace(orig, `["other", "callboard"]`, `["other"]`, 1); string(b) != want {
		t.Fatalf("a compact list was spread out:\n%s", b)
	}
}
