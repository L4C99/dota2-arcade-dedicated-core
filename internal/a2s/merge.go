// Package a2s edits only the optional GameInfo/GMS/Advertise setting. It is
// independent of the manager and never runs as part of ordinary room startup.
package a2s

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

type token struct {
	value      string
	start, end int
	quoted     bool
}
type node struct {
	key, value string
	block      bool
	close      int
	children   []node
}

func lex(b []byte) ([]token, error) {
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, fmt.Errorf("gameinfo must be UTF-8 text without NUL")
	}
	if len(b) > 4<<20 {
		return nil, fmt.Errorf("gameinfo exceeds 4 MiB")
	}
	var out []token
	for i := 0; i < len(b); {
		if b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n' {
			i++
			continue
		}
		if i == 0 && bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
			i += 3
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '/' {
			for i < len(b) && b[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '*' {
			j := bytes.Index(b[i+2:], []byte("*/"))
			if j < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			i += j + 4
			continue
		}
		start := i
		if b[i] == '{' || b[i] == '}' {
			out = append(out, token{string(b[i]), i, i + 1, false})
			i++
			continue
		}
		quoted := b[i] == '"'
		var v strings.Builder
		if quoted {
			i++
			closed := false
			for i < len(b) {
				if b[i] == '"' {
					i++
					closed = true
					break
				}
				if b[i] == '\\' && i+1 < len(b) && (b[i+1] == '"' || b[i+1] == '\\') {
					i++
				}
				v.WriteByte(b[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted token")
			}
		} else {
			for i < len(b) && !strings.ContainsRune(" \t\r\n{}\"", rune(b[i])) {
				v.WriteByte(b[i])
				i++
			}
			if i == start {
				return nil, fmt.Errorf("invalid token")
			}
		}
		out = append(out, token{v.String(), start, i, quoted})
		if len(out) > 100000 {
			return nil, fmt.Errorf("too many tokens")
		}
	}
	return out, nil
}

func parse(ts []token, pos *int, depth int) ([]node, int, error) {
	if depth > 64 {
		return nil, 0, fmt.Errorf("nesting exceeds 64")
	}
	var nodes []node
	for *pos < len(ts) {
		t := ts[*pos]
		if !t.quoted && t.value == "}" {
			if depth == 0 {
				return nil, 0, fmt.Errorf("unexpected closing brace")
			}
			*pos++
			return nodes, t.start, nil
		}
		if !t.quoted && (t.value == "{" || strings.HasPrefix(t.value, "#") || strings.HasPrefix(t.value, "[")) {
			return nil, 0, fmt.Errorf("unsupported or malformed structure")
		}
		*pos++
		if *pos >= len(ts) {
			return nil, 0, fmt.Errorf("missing value")
		}
		v := ts[*pos]
		*pos++
		n := node{key: t.value}
		if !v.quoted && v.value == "{" {
			var e error
			n.block = true
			n.children, n.close, e = parse(ts, pos, depth+1)
			if e != nil {
				return nil, 0, e
			}
		} else {
			if !v.quoted && v.value == "}" {
				return nil, 0, fmt.Errorf("missing value")
			}
			n.value = v.value
		}
		nodes = append(nodes, n)
	}
	if depth > 0 {
		return nil, 0, fmt.Errorf("unclosed block")
	}
	return nodes, 0, nil
}

func unique(nodes []node, key string) (*node, error) {
	var found *node
	for i := range nodes {
		if strings.EqualFold(nodes[i].key, key) {
			if found != nil {
				return nil, fmt.Errorf("duplicate %s", key)
			}
			found = &nodes[i]
		}
	}
	return found, nil
}

// Merge preserves all original bytes except one insertion before a closing
// brace. Conflicting values and ambiguous structures are never rewritten.
func Merge(b []byte) ([]byte, bool, error) {
	ts, e := lex(b)
	if e != nil {
		return nil, false, e
	}
	pos := 0
	nodes, _, e := parse(ts, &pos, 0)
	if e != nil {
		return nil, false, e
	}
	root, e := unique(nodes, "GameInfo")
	if e != nil {
		return nil, false, e
	}
	if len(nodes) != 1 || root == nil || !root.block {
		return nil, false, fmt.Errorf("expected a single GameInfo block")
	}
	gms, e := unique(root.children, "GMS")
	if e != nil {
		return nil, false, e
	}
	nl := "\n"
	if bytes.Contains(b, []byte("\r\n")) {
		nl = "\r\n"
	}
	at := root.close
	insertion := nl + "\tGMS" + nl + "\t{" + nl + "\t\tAdvertise 1" + nl + "\t}" + nl
	if gms != nil {
		if !gms.block {
			return nil, false, fmt.Errorf("GMS must be a block")
		}
		adv, e := unique(gms.children, "Advertise")
		if e != nil {
			return nil, false, e
		}
		if adv != nil {
			if adv.block || adv.value != "1" {
				return nil, false, fmt.Errorf("Advertise already has a conflicting value")
			}
			return append([]byte(nil), b...), false, nil
		}
		at = gms.close
		insertion = nl + "\t\tAdvertise 1" + nl + "\t"
	}
	out := make([]byte, 0, len(b)+len(insertion))
	out = append(out, b[:at]...)
	out = append(out, insertion...)
	out = append(out, b[at:]...)
	return out, true, nil
}
