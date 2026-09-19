package engine

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
)

type Match struct {
	Rule       string    `json:"rule"`
	ObservedAt time.Time `json:"observedAt"`
}
type Evidence struct {
	Generation int       `json:"generation"`
	ObservedAt time.Time `json:"observedAt"`
	Source     string    `json:"source"`
	Matched    []Match   `json:"matched"`
	Valid      bool      `json:"valid"`
}
type Observation struct {
	Evidence *Evidence
	Ready    bool
	Failure  string
}
type Observer struct {
	generation int
	path       string
	rules      config.Readiness
	offset     int64
	previous   os.FileInfo
	suffix     string
	matched    map[string]time.Time
	failure    string
}

func NewObserver(generation int, path string, rules config.Readiness) *Observer {
	return &Observer{generation: generation, path: path, rules: rules, matched: map[string]time.Time{}}
}

// Scan reads only this generation's uniquely created engine log. Bounded reads
// prevent a noisy game from monopolizing the manager; the next scan resumes.
func (o *Observer) Scan(alive bool) (Observation, error) {
	f, err := os.Open(o.path)
	if os.IsNotExist(err) {
		return Observation{}, nil
	}
	if err != nil {
		return Observation{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Observation{}, err
	}
	if !info.Mode().IsRegular() {
		return Observation{}, fmt.Errorf("engine log is not a regular file")
	}
	if info.Size() < o.offset || o.previous != nil && !os.SameFile(o.previous, info) {
		o.offset = 0
		o.suffix = ""
		o.matched = map[string]time.Time{}
		o.failure = ""
	}
	o.previous = info
	if _, err = f.Seek(o.offset, io.SeekStart); err != nil {
		return Observation{}, err
	}
	b, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return Observation{}, err
	}
	o.offset += int64(len(b))
	text := o.suffix + string(b)
	now := time.Now().UTC()
	for _, rule := range o.rules.FailureAny {
		if strings.Contains(text, rule) {
			o.failure = rule
			break
		}
	}
	for _, rule := range o.rules.SuccessAll {
		if _, ok := o.matched[rule]; !ok && strings.Contains(text, rule) {
			o.matched[rule] = now
		}
	}
	if len(text) > 511 {
		o.suffix = text[len(text)-511:]
	} else {
		o.suffix = text
	}
	e := &Evidence{Generation: o.generation, ObservedAt: now, Source: o.path, Matched: []Match{}, Valid: alive && o.failure == ""}
	all := len(o.rules.SuccessAll) > 0
	for _, rule := range o.rules.SuccessAll {
		at, ok := o.matched[rule]
		if !ok {
			all = false
		} else {
			e.Matched = append(e.Matched, Match{rule, at})
		}
	}
	return Observation{Evidence: e, Ready: all && e.Valid && o.offset >= info.Size(), Failure: o.failure}, nil
}

func ReadTail(path string, lines int) (string, bool, error) {
	if lines < 1 || lines > 1000 {
		return "", false, fmt.Errorf("tail must be 1..1000")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	if !st.Mode().IsRegular() {
		return "", false, fmt.Errorf("log is not a regular file")
	}
	start := st.Size() - (256 << 10)
	truncated := start > 0
	if start < 0 {
		start = 0
	}
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return "", false, err
	}
	b, err := io.ReadAll(io.LimitReader(f, 256<<10))
	if err != nil {
		return "", false, err
	}
	if start > 0 {
		for len(b) > 0 && !utf8.RuneStart(b[0]) {
			b = b[1:]
		}
	}
	text := string(b)
	split := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(split) > lines {
		split = split[len(split)-lines:]
		truncated = true
		text = strings.Join(split, "\n")
		if len(b) > 0 && b[len(b)-1] == '\n' {
			text += "\n"
		}
	}
	return text, truncated, nil
}
