package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type Obj struct {
	keys []string
	vals map[string]any
	raw  map[string]source
	src  *source
}

type source struct {
	text, canon string
}

func newObj() *Obj { return &Obj{vals: map[string]any{}} }

func (o *Obj) Get(k string) any { return o.vals[k] }

func (o *Obj) Set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *Obj) Delete(k string) {
	if _, ok := o.vals[k]; ok {
		delete(o.vals, k)
		delete(o.raw, k)
		o.keys = slices.DeleteFunc(o.keys, func(x string) bool { return x == k })
	}
}

func (o *Obj) Len() int { return len(o.keys) }

func (o *Obj) Child(k string) *Obj {
	c, ok := o.vals[k].(*Obj)
	if !ok {
		c = newObj()
		o.Set(k, c)
	}
	return c
}

func (o *Obj) ChildIf(k string) *Obj {
	c, _ := o.vals[k].(*Obj)
	return c
}

func (o *Obj) List(k string) []any {
	l, _ := o.vals[k].([]any)
	return l
}

func parseObj(b []byte) (*Obj, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := decodeValue(dec, b)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*Obj)
	if !ok {
		return nil, fmt.Errorf("the top level isn't a JSON object")
	}
	if _, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("extra data after the JSON object")
	}
	return o, nil
}

func sourceOf(b []byte, from, to int64, v any) source {
	text := strings.TrimLeft(string(b[from:to]), " \t\r\n:,")
	return source{text: strings.ReplaceAll(text, "\r\n", "\n"), canon: canonical(v)}
}

func canonical(v any) string {
	var b strings.Builder
	write(&b, v, "\t", "", false)
	return b.String()
}

func decodeValue(dec *json.Decoder, b []byte) (any, error) {
	from := dec.InputOffset()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := newObj()
			o.raw = map[string]source{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				at := dec.InputOffset()
				v, err := decodeValue(dec, b)
				if err != nil {
					return nil, err
				}
				o.Set(k, v)
				if _, isObj := v.(*Obj); !isObj {
					o.raw[k] = sourceOf(b, at, dec.InputOffset(), v)
				}
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			src := sourceOf(b, from, dec.InputOffset(), o)
			o.src = &src
			return o, nil
		case '[':
			l := []any{}
			for dec.More() {
				v, err := decodeValue(dec, b)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			}
			_, err := dec.Token()
			return l, err
		}
	}
	return tok, nil
}

func encode(b *strings.Builder, v any, indent, pad string) {
	write(b, v, indent, pad, true)
}

func write(b *strings.Builder, v any, indent, pad string, keep bool) {
	switch t := v.(type) {
	case *Obj:
		if keep && t.src != nil && t.src.canon == canonical(t) {
			b.WriteString(t.src.text)
			return
		}
		if t.Len() == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range t.keys {
			b.WriteString(pad + indent + scalar(k) + ": ")
			r, ok := t.raw[k]
			switch {
			case keep && ok && r.canon == canonical(t.vals[k]):
				b.WriteString(r.text)
			case keep && ok && !strings.Contains(r.text, "\n") && strings.HasPrefix(r.text, "["):
				inline(b, t.vals[k], !strings.Contains(r.text, ",") || strings.Contains(r.text, ", "))
			default:
				write(b, t.vals[k], indent, pad+indent, keep)
			}
			if i < len(t.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "}")
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, x := range t {
			b.WriteString(pad + indent)
			write(b, x, indent, pad+indent, keep)
			if i < len(t)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "]")
	default:
		b.WriteString(scalar(t))
	}
}

func inline(b *strings.Builder, v any, spaced bool) {
	comma, colon := ",", ":"
	if spaced {
		comma, colon = ", ", ": "
	}
	switch t := v.(type) {
	case *Obj:
		b.WriteString("{")
		for i, k := range t.keys {
			if i > 0 {
				b.WriteString(comma)
			}
			b.WriteString(scalar(k) + colon)
			inline(b, t.vals[k], spaced)
		}
		b.WriteString("}")
	case []any:
		b.WriteString("[")
		for i, x := range t {
			if i > 0 {
				b.WriteString(comma)
			}
			inline(b, x, spaced)
		}
		b.WriteString("]")
	default:
		b.WriteString(scalar(t))
	}
}

func scalar(v any) string {
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

func indentOf(b []byte) string {
	for _, l := range strings.Split(string(b), "\n")[1:] {
		if t := strings.TrimLeft(l, " \t"); t != "" && len(t) < len(l) {
			return l[:len(l)-len(t)]
		}
	}
	return "  "
}
