package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAPI(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run()
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("# Backlog\n\n## v1\n\n## Later\n"), 0o644)
	s := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.watch(ctx)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	do := func(method, path, body string, hdr ...string) (*http.Response, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp, m
	}

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan string, 10)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if e, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				events <- e
			}
		}
	}()
	if e := <-events; e != "hello" {
		t.Fatalf("first event %q", e)
	}

	r, m := do("POST", "/api/tasks", `{"title":"From the page","section":"Later"}`)
	if r.StatusCode != 201 || !strings.HasPrefix(m["key"].(string), "B-") {
		t.Fatalf("add: %d %v", r.StatusCode, m)
	}
	key := m["key"].(string)
	got := map[string]bool{}
	for deadline := time.After(3 * time.Second); !(got["board"] && got["change"]); {
		select {
		case e := <-events:
			got[e] = true
		case <-deadline:
			t.Fatalf("events so far: %v", got)
		}
	}

	r, m = do("PATCH", "/api/tasks/"+key, `{"done":true}`)
	if r.StatusCode != 200 || m["done"] != true {
		t.Fatalf("tick: %d %v", r.StatusCode, m)
	}
	r, _ = do("PATCH", "/api/tasks/B-nope", `{"done":true}`)
	if r.StatusCode != 404 {
		t.Fatalf("unknown key: %d", r.StatusCode)
	}
	r, _ = do("POST", "/api/tasks", `{"title":"evil"}`, "Origin", "https://evil.example")
	if r.StatusCode != 403 {
		t.Fatalf("cross-site add: %d", r.StatusCode)
	}

	_, m = do("GET", "/api/board", "")
	lists := m["lists"].([]any)
	backlog := lists[0].(map[string]any)
	secs := backlog["sections"].([]any)
	if len(lists) != 3 || len(secs) != 2 || secs[1].(map[string]any)["name"] != "Later" || backlog["done"].(float64) != 1 || m["last"].(map[string]any)["what"] != "ticked" {
		t.Fatalf("board %v", m)
	}

	item := secs[1].(map[string]any)["items"].([]any)[0].(map[string]any)
	v := item["version"].(string)
	r, m = do("PATCH", "/api/items/"+key, `{"title":"Renamed on the page","version":"`+v+`"}`)
	if r.StatusCode != 200 || m["title"] != "Renamed on the page" {
		t.Fatalf("rename: %d %v", r.StatusCode, m)
	}
	r, _ = do("PATCH", "/api/items/"+key, `{"done":false,"version":"`+v+`"}`)
	if r.StatusCode != 409 {
		t.Fatalf("stale version: %d", r.StatusCode)
	}

	_, m = do("POST", "/api/tasks", `{"title":"Second","section":"Later"}`)
	second := m["key"].(string)
	r, m = do("PATCH", "/api/items/"+second, `{"before":"`+key+`"}`)
	if r.StatusCode != 200 {
		t.Fatalf("place before: %d %v", r.StatusCode, m)
	}
	_, m = do("GET", "/api/board", "")
	later := m["lists"].([]any)[0].(map[string]any)["sections"].([]any)[1].(map[string]any)["items"].([]any)
	if later[0].(map[string]any)["key"] != second {
		t.Fatalf("order after placing: %v", later)
	}
	if r, _ = do("PATCH", "/api/items/"+second, `{"after":"R-nope"}`); r.StatusCode != 400 {
		t.Fatalf("placed next to another list's item: %d", r.StatusCode)
	}

	r, m = do("POST", "/api/items", `{"kind":"question","title":"Which port?","options":["4700","From the path"],"recommended":2}`)
	if r.StatusCode != 201 {
		t.Fatalf("add question: %d %v", r.StatusCode, m)
	}
	q := m["key"].(string)
	r, m = do("POST", "/api/items", `{"kind":"task","title":"Serve the page","needs":["`+q+`"]}`)
	if r.StatusCode != 201 {
		t.Fatalf("add waiting task: %d %v", r.StatusCode, m)
	}
	waiting := m["key"].(string)
	r, m = do("POST", "/api/items/"+q+"/answer", `{"answer":"2","why":"bookmarks keep working"}`)
	if r.StatusCode != 200 || m["answer"] != "From the path" || len(m["unblocked"].([]any)) != 1 || m["unblocked"].([]any)[0] != waiting {
		t.Fatalf("answer: %d %v", r.StatusCode, m)
	}
	_, m = do("GET", "/api/board", "")
	qs := m["lists"].([]any)[2].(map[string]any)["sections"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if qs["done"] != true || qs["answer"] != "From the path" || qs["why"] != "bookmarks keep working" {
		t.Fatalf("answered question %v", qs)
	}
	r, m = do("POST", "/api/items/"+q+"/reopen", `{}`)
	if r.StatusCode != 200 || m["key"] != q {
		t.Fatalf("reopen: %d %v", r.StatusCode, m)
	}

	_, m = do("POST", "/api/items", `{"kind":"request","title":"Make an API key"}`)
	rk := m["key"].(string)
	if r, m = do("PATCH", "/api/items/"+rk, `{"done":true}`); r.StatusCode != 200 || m["done"] != true {
		t.Fatalf("tick request: %d %v", r.StatusCode, m)
	}

	r, m = do("PATCH", "/api/items/"+waiting, `{"fields":{"status":"doing","area":"page ui","prio":"high"},"section":"Later"}`)
	if r.StatusCode != 200 || m["section"] != "Later" || m["fields"].(map[string]any)["area"] != "page ui" {
		t.Fatalf("set fields: %d %v", r.StatusCode, m)
	}
	if r, _ = do("PATCH", "/api/items/"+waiting, `{"fields":{"done":"x"}}`); r.StatusCode != 400 {
		t.Fatalf("a reserved field was set: %d", r.StatusCode)
	}
	r, m = do("POST", "/api/views", `{"name":"By status","layout":"board","group":"status","order":["todo","doing","done"]}`)
	if r.StatusCode != 201 || m["layout"] != "board" || m["list"] != "backlog" {
		t.Fatalf("add view: %d %v", r.StatusCode, m)
	}
	vk := m["key"].(string)
	if r, m = do("PATCH", "/api/views/"+vk, `{"layout":"table","sort":["-prio"]}`); r.StatusCode != 200 || m["group"] != "status" || m["layout"] != "table" {
		t.Fatalf("change view: %d %v", r.StatusCode, m)
	}
	if r, _ = do("POST", "/api/views", `{"name":"x","layout":"grid"}`); r.StatusCode != 400 {
		t.Fatalf("bad layout: %d", r.StatusCode)
	}
	_, m = do("GET", "/api/board", "")
	if vs := m["views"].([]any); len(vs) != 1 || m["lists"].([]any)[0].(map[string]any)["fields"].(map[string]any)["status"].([]any)[0] != "doing" {
		t.Fatalf("board views/fields %v %v", m["views"], m["lists"].([]any)[0].(map[string]any)["fields"])
	}
	if r, _ = do("DELETE", "/api/views/"+vk, ""); r.StatusCode != 200 {
		t.Fatalf("delete view: %d", r.StatusCode)
	}

	_, m = do("GET", "/api/branches", "")
	if m["main"] != "main" || m["branches"] == nil {
		t.Fatalf("branches %v", m)
	}

	res, _ := http.Get(ts.URL + "/")
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func TestDefaultPortStable(t *testing.T) {
	a, b := DefaultPort("/x/one"), DefaultPort("/x/two")
	if a != DefaultPort("/x/one") || a < 4700 || a > 4999 || b < 4700 || b > 4999 {
		t.Fatal(a, b)
	}
}

func TestAddBadFields(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run()
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("# Backlog\n"), 0o644)
	ts := httptest.NewServer(New(dir).Handler())
	defer ts.Close()
	for _, body := range []string{
		`{"title":"A","kind":"task","fields":{"":"x"}}`,
		`{"title":"B","kind":"task","fields":{"Bad Name":"x"}}`,
		`{"title":"C","kind":"task","fields":{"id":"B-0000"}}`,
		`{"title":"D","kind":"task","fields":{"prio":"x-->y"}}`,
	} {
		resp, err := http.Post(ts.URL+"/api/items", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: %d, want 400", body, resp.StatusCode)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "backlog.md")); strings.Contains(string(b), "- [ ]") {
		t.Errorf("a refused item was added:\n%s", b)
	}
}

func TestOnlyLocalHosts(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run()
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("# Backlog\n"), 0o644)
	ts := httptest.NewServer(New(dir).Handler())
	defer ts.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	for host, want := range map[string]int{
		"localhost:" + port: 200, "127.0.0.1:" + port: 200, "[::1]:" + port: 200, "cb.localhost:" + port: 200,
		"evil.example:" + port: 403, "evil.example": 403, "localhost.evil.example:" + port: 403, "192.168.1.5:" + port: 403,
	} {
		for _, m := range []string{"GET", "POST"} {
			req, _ := http.NewRequest(m, ts.URL+"/api/board", nil)
			if m == "POST" {
				req, _ = http.NewRequest(m, ts.URL+"/api/items", strings.NewReader(`{"title":"From `+host+`"}`))
			}
			req.Host = host
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if got := resp.StatusCode; (want == 403) != (got == 403) || m == "GET" && got != want {
				t.Errorf("%s with Host %s: %d, want %d", m, host, got, want)
			}
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "backlog.md")); strings.Contains(string(b), "evil") || strings.Contains(string(b), "192.168") {
		t.Errorf("a foreign host added an item:\n%s", b)
	}
}
