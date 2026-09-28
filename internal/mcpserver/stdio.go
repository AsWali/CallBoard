package mcpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func Stdio(in io.Reader, out io.Writer) mcp.Transport {
	w := &lockedWriter{w: out}
	pr, pw := io.Pipe()
	go func() {
		rd := bufio.NewReader(in)
		for {
			line, err := rd.ReadBytes('\n')
			if l := bytes.TrimSpace(line); len(l) > 0 {
				if reply := check(l); reply != nil {
					w.Write(reply)
				} else if _, werr := pw.Write(append(l, '\n')); werr != nil {
					return
				}
			}
			if err != nil {
				pw.Close()
				return
			}
		}
	}()
	return &mcp.IOTransport{Reader: pr, Writer: w}
}

func check(l []byte) []byte {
	reply := func(id json.RawMessage, code int, msg string) []byte {
		if len(id) == 0 {
			id = json.RawMessage("null")
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
		return append(b, '\n')
	}
	if !json.Valid(l) {
		return reply(nil, -32700, "Parse error: the line isn't JSON")
	}
	if l[0] == '[' {
		return nil
	}
	var m struct {
		JSONRPC *string         `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
	}
	if json.Unmarshal(l, &m) != nil || m.JSONRPC == nil || *m.JSONRPC != "2.0" {
		return reply(m.ID, -32600, `Invalid Request: a JSON-RPC 2.0 message is an object with "jsonrpc": "2.0"`)
	}
	return nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (l *lockedWriter) Close() error { return nil }
