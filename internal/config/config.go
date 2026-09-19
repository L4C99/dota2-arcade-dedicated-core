// Package config validates trusted local templates without modifying the filesystem.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const SchemaVersion = 1
const MaxTemplateBytes = 1 << 20

type Template struct {
	SchemaVersion    int       `json:"schemaVersion"`
	Name             string    `json:"name"`
	Executable       string    `json:"executable"`
	WorkingDirectory string    `json:"workingDirectory"`
	Arguments        []string  `json:"arguments"`
	CFG              CFGConfig `json:"cfg"`
	Readiness        Readiness `json:"readiness"`
	Timeouts         Timeouts  `json:"timeouts"`
}
type CFGConfig struct {
	Directory string   `json:"directory"`
	Lines     []string `json:"lines"`
}
type Readiness struct {
	SuccessAll []string `json:"successAll"`
	FailureAny []string `json:"failureAny"`
}
type Timeouts struct {
	StartupSeconds int `json:"startupSeconds"`
	StopSeconds    int `json:"stopSeconds"`
	ForceSeconds   int `json:"forceSeconds"`
}
type Values struct {
	InstanceID, CfgName, LogPath string
	GamePort                     int
}
type Expanded struct {
	Arguments []string
	CFG       string
}

// Error carries a stable category and the failing field for CLI/API adapters.
type Error struct{ Code, Field, Message string }

func (e *Error) Error() string            { return e.Field + ": " + e.Message }
func invalid(field, message string) error { return &Error{"INVALID_TEMPLATE", field, message} }

// DecodeStrict rejects duplicate keys at every depth, unknown or incorrectly
// cased struct fields, invalid UTF-8 and trailing values. It does not impose a
// transport size limit; the caller must bound its input before calling it.
func DecodeStrict(data []byte, out any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON is not UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var scan func() error
	scan = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok {
					return fmt.Errorf("invalid object key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err := scan(); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := scan(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := scan(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("trailing JSON value")
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer || reflect.ValueOf(out).IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer")
	}
	var shape any
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	if err := exactFields(shape, t.Elem()); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func exactFields(value any, t reflect.Type) error {
	if value == nil && t.Kind() == reflect.Pointer {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	if value == nil && t.Kind() != reflect.Interface && t.Kind() != reflect.Map && t.Kind() != reflect.Slice {
		return fmt.Errorf("null is not valid for %s", t)
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			n := strings.Split(f.Tag.Get("json"), ",")[0]
			if n == "-" {
				continue
			}
			if n == "" {
				n = f.Name
			}
			fields[n] = f.Type
		}
		for key, v := range obj {
			ft, ok := fields[key]
			if !ok {
				return fmt.Errorf("unknown JSON field %q", key)
			}
			if err := exactFields(v, ft); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if arr, ok := value.([]any); ok {
			for _, v := range arr {
				if err := exactFields(v, t.Elem()); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		if obj, ok := value.(map[string]any); ok {
			for _, v := range obj {
				if err := exactFields(v, t.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func Load(path string) (Template, error) {
	var t Template
	if !filepath.IsAbs(path) {
		return t, &Error{"INVALID_PATH", "template", "absolute path required"}
	}
	st, err := os.Stat(path)
	if err != nil {
		return t, &Error{"INVALID_PATH", "template", err.Error()}
	}
	if !st.Mode().IsRegular() {
		return t, &Error{"INVALID_PATH", "template", "regular file required"}
	}
	f, err := os.Open(path)
	if err != nil {
		return t, &Error{"INVALID_PATH", "template", err.Error()}
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil {
		return t, &Error{"INVALID_PATH", "template", err.Error()}
	}
	if !st.Mode().IsRegular() {
		return t, &Error{"INVALID_PATH", "template", "regular file required"}
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxTemplateBytes+1))
	if err != nil {
		return t, invalid("template", err.Error())
	}
	if len(b) > MaxTemplateBytes {
		return t, invalid("template", "exceeds 1 MiB")
	}
	if err := DecodeStrict(b, &t); err != nil {
		return t, invalid("template", err.Error())
	}
	return Validate(t)
}

func safeText(s string) bool { return utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n") }
func plain(s string) bool {
	return safeText(s) && !strings.Contains(s, "{{") && !strings.Contains(s, "}}")
}
func pathCheck(field, path string, dir bool) (string, error) {
	if !plain(path) || !filepath.IsAbs(path) {
		return "", &Error{"INVALID_PATH", field, "literal absolute path required"}
	}
	path = filepath.Clean(path)
	st, err := os.Stat(path)
	if err != nil {
		return "", &Error{"INVALID_PATH", field, err.Error()}
	}
	if (dir && !st.IsDir()) || (!dir && !st.Mode().IsRegular()) {
		return "", &Error{"INVALID_PATH", field, "wrong file type"}
	}
	return path, nil
}

// Validate returns a normalized independent copy. It only stats input paths;
// writability, resources referenced by arbitrary cfg, and ports are not tested.
func Validate(t Template) (Template, error) {
	if t.SchemaVersion != SchemaVersion {
		return Template{}, &Error{"UNSUPPORTED_VERSION", "schemaVersion", "expected 1"}
	}
	if strings.TrimSpace(t.Name) == "" || !plain(t.Name) {
		return Template{}, invalid("name", "nonempty literal single-line name required")
	}
	var err error
	if t.Executable, err = pathCheck("executable", t.Executable, false); err != nil {
		return Template{}, err
	}
	if t.WorkingDirectory, err = pathCheck("workingDirectory", t.WorkingDirectory, true); err != nil {
		return Template{}, err
	}
	if t.CFG.Directory, err = pathCheck("cfg.directory", t.CFG.Directory, true); err != nil {
		return Template{}, err
	}
	t.Arguments = append([]string(nil), t.Arguments...)
	t.CFG.Lines = append([]string(nil), t.CFG.Lines...)
	t.Readiness.SuccessAll = append([]string(nil), t.Readiness.SuccessAll...)
	t.Readiness.FailureAny = append([]string{}, t.Readiness.FailureAny...)
	for i, a := range t.Arguments {
		field := fmt.Sprintf("arguments[%d]", i)
		if !safeText(a) {
			return Template{}, invalid(field, "invalid text or control character")
		}
		names, err := placeholders(a)
		if err != nil {
			return Template{}, invalid(field, err.Error())
		}
		if len(names) > 0 && (len(names) != 1 || a != "{{"+names[0]+"}}") {
			return Template{}, invalid(field, "placeholder must be a complete argument")
		}
	}
	for flag, value := range map[string]string{"-port": "{{game_port}}", "-con_logfile": "{{log_path}}", "+exec": "{{cfg_name}}"} {
		count := 0
		for i, a := range t.Arguments {
			if strings.EqualFold(a, flag) {
				count++
				if a != flag || i+1 == len(t.Arguments) || t.Arguments[i+1] != value {
					return Template{}, invalid("arguments", "required adjacent pair "+flag+" "+value)
				}
			}
		}
		if count != 1 {
			return Template{}, invalid("arguments", "expected exactly one "+flag)
		}
	}
	if len(t.CFG.Lines) == 0 {
		return Template{}, invalid("cfg.lines", "must not be empty")
	}
	for i, line := range t.CFG.Lines {
		if !safeText(line) {
			return Template{}, invalid(fmt.Sprintf("cfg.lines[%d]", i), "single line without NUL required")
		}
		if _, err := placeholders(line); err != nil {
			return Template{}, invalid("cfg.lines", err.Error())
		}
		if _, err := expandCFG(line, map[string]string{}); err != nil {
			return Template{}, invalid("cfg.lines", err.Error())
		}
	}
	if len(t.Readiness.SuccessAll) == 0 {
		return Template{}, invalid("readiness.successAll", "at least one rule required")
	}
	for name, list := range map[string][]string{"successAll": t.Readiness.SuccessAll, "failureAny": t.Readiness.FailureAny} {
		if len(list) > 32 {
			return Template{}, invalid("readiness."+name, "at most 32 rules")
		}
		for _, rule := range list {
			if len(rule) == 0 || len(rule) > 512 || !safeText(rule) {
				return Template{}, invalid("readiness."+name, "rules must be nonempty single-line UTF-8, at most 512 bytes")
			}
		}
	}
	for _, pair := range []struct {
		name string
		p    *int
		def  int
	}{{"startupSeconds", &t.Timeouts.StartupSeconds, 120}, {"stopSeconds", &t.Timeouts.StopSeconds, 10}, {"forceSeconds", &t.Timeouts.ForceSeconds, 5}} {
		if *pair.p < 0 || *pair.p > 3600 {
			return Template{}, invalid("timeouts."+pair.name, "expected 0..3600")
		}
		if *pair.p == 0 {
			*pair.p = pair.def
		}
	}
	return t, nil
}

func placeholders(s string) ([]string, error) {
	var names []string
	for len(s) > 0 {
		i := strings.Index(s, "{{")
		j := strings.Index(s, "}}")
		if i < 0 {
			if j >= 0 {
				return nil, fmt.Errorf("unmatched placeholder close")
			}
			break
		}
		if j < 0 || j < i {
			return nil, fmt.Errorf("unmatched placeholder")
		}
		name := s[i+2 : j]
		switch name {
		case "instance_id", "game_port", "cfg_name", "log_path":
		default:
			return nil, fmt.Errorf("unknown placeholder %q", name)
		}
		names = append(names, name)
		s = s[j+2:]
	}
	return names, nil
}

func token(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func Expand(t Template, v Values) (Expanded, error) {
	t, err := Validate(t)
	if err != nil {
		return Expanded{}, err
	}
	if !token(v.InstanceID) || !token(v.CfgName) {
		return Expanded{}, invalid("values", "instance ID and cfg name must be safe ASCII tokens")
	}
	if v.GamePort < 1 || v.GamePort > 65535 {
		return Expanded{}, invalid("gamePort", "expected 1..65535")
	}
	if !filepath.IsAbs(v.LogPath) || !plain(v.LogPath) {
		return Expanded{}, &Error{"INVALID_PATH", "logPath", "literal absolute ASCII path required"}
	}
	for _, c := range v.LogPath {
		if c < 32 || c > 126 {
			return Expanded{}, &Error{"INVALID_PATH", "logPath", "ASCII path required"}
		}
	}
	values := map[string]string{"instance_id": v.InstanceID, "cfg_name": v.CfgName, "game_port": strconv.Itoa(v.GamePort), "log_path": v.LogPath}
	out := Expanded{Arguments: make([]string, len(t.Arguments))}
	for i, a := range t.Arguments {
		if strings.HasPrefix(a, "{{") {
			a = values[a[2:len(a)-2]]
		}
		out.Arguments[i] = a
	}
	var cfg strings.Builder
	for _, line := range t.CFG.Lines {
		expanded, err := expandCFG(line, values)
		if err != nil {
			return Expanded{}, invalid("cfg.lines", err.Error())
		}
		cfg.WriteString(expanded)
		cfg.WriteByte('\n')
	}
	out.CFG = cfg.String()
	return out, nil
}

// expandCFG tracks the source quoting context; inserted text cannot close it.
func expandCFG(line string, values map[string]string) (string, error) {
	var out strings.Builder
	quoted := false
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "{{") {
			end := strings.Index(line[i:], "}}")
			if end < 0 {
				return "", fmt.Errorf("unmatched placeholder")
			}
			name := line[i+2 : i+end]
			value, ok := values[name]
			if !ok {
				value = "x"
			}
			if name == "log_path" {
				value = strings.ReplaceAll(value, "\\", "/")
				value = strings.ReplaceAll(value, "\"", "\\\"")
				if !quoted {
					value = "\"" + value + "\""
				}
			}
			out.WriteString(value)
			i += end + 2
			continue
		}
		c := line[i]
		if c == '\\' && i+1 < len(line) && (line[i+1] == '"' || line[i+1] == '\\') {
			out.WriteByte(c)
			out.WriteByte(line[i+1])
			i += 2
			continue
		}
		if c == '"' {
			quoted = !quoted
		}
		out.WriteByte(c)
		i++
	}
	if quoted {
		return "", fmt.Errorf("unclosed cfg quote")
	}
	return out.String(), nil
}
