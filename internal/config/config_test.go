package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) Template {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Template{SchemaVersion: 1, Name: "测试 template", Executable: exe, WorkingDirectory: dir, Arguments: []string{"-dedicated", "-port", "{{game_port}}", "-con_logfile", "{{log_path}}", "+exec", "{{cfg_name}}"}, CFG: CFGConfig{Directory: dir, Lines: []string{`hostname "{{instance_id}}"`, `map n7 customgamemode="sample.vpk"`}}, Readiness: Readiness{SuccessAll: []string{"loaded", "ready"}, FailureAny: []string{"failed"}}}
}

func TestDecodeStrict(t *testing.T) {
	type nested struct {
		Value int `json:"value"`
	}
	type request struct {
		Inner nested   `json:"inner"`
		Items []nested `json:"items"`
	}
	for _, s := range []string{`{"inner":{"value":1,"value":2}}`, `{"inner":{"value":1,"\u0076alue":2}}`, `{"Inner":{"value":1}}`, `{"inner":{"Value":1}}`, `{"items":[{"unknown":1}]}`, `{"inner":{"value":1}} {}`, `{"inner":{"value":1},"extra":2}`, `{"inner":{"value":1.5}}`, "{\"inner\":\"\xff\"}"} {
		t.Run(s, func(t *testing.T) {
			var got request
			if err := DecodeStrict([]byte(s), &got); err == nil {
				t.Fatal("accepted invalid JSON", s)
			}
		})
	}
	var got request
	if err := DecodeStrict([]byte(`{"inner":{"value":2},"items":[{"value":3}]}`), &got); err != nil || got.Inner.Value != 2 {
		t.Fatalf("got %+v, %v", got, err)
	}
	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	if err := DecodeStrict([]byte(`{"params":{"nested":1,"nested":2}}`), &envelope); err == nil {
		t.Fatal("duplicate hidden in RawMessage")
	}
}

func TestValidateReadOnlyAndIndependent(t *testing.T) {
	in := fixture(t)
	before, _ := os.ReadDir(in.WorkingDirectory)
	out, err := Validate(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Timeouts != (Timeouts{120, 10, 5}) {
		t.Fatal(out.Timeouts)
	}
	out.Arguments[0] = "changed"
	out.CFG.Lines[0] = "changed"
	out.Readiness.SuccessAll[0] = "changed"
	if in.Arguments[0] != "-dedicated" || in.CFG.Lines[0] == "changed" || in.Readiness.SuccessAll[0] == "changed" {
		t.Fatal("mutated input")
	}
	after, _ := os.ReadDir(in.WorkingDirectory)
	if len(before) != len(after) {
		t.Fatal("validation wrote files")
	}
}

func TestInvalidTemplates(t *testing.T) {
	cases := map[string]func(*Template){
		"version":             func(v *Template) { v.SchemaVersion = 2 },
		"relative exe":        func(v *Template) { v.Executable = "dota.exe" },
		"directory exe":       func(v *Template) { v.Executable = v.WorkingDirectory },
		"missing dir":         func(v *Template) { v.CFG.Directory = filepath.Join(v.WorkingDirectory, "missing") },
		"name placeholder":    func(v *Template) { v.Name = "{{instance_id}}" },
		"unknown placeholder": func(v *Template) { v.Arguments = append(v.Arguments, "{{bad}}") },
		"broken placeholder":  func(v *Template) { v.CFG.Lines = append(v.CFG.Lines, "echo {{instance_id") },
		"closing placeholder": func(v *Template) { v.CFG.Lines = append(v.CFG.Lines, "echo instance_id}}") },
		"embedded argument":   func(v *Template) { v.Arguments = append(v.Arguments, "--name={{instance_id}}") },
		"duplicate port":      func(v *Template) { v.Arguments = append(v.Arguments, "-port", "{{game_port}}") },
		"wrong port":          func(v *Template) { v.Arguments[2] = "27015" },
		"duplicate case":      func(v *Template) { v.Arguments = append(v.Arguments, "-PORT", "27016") },
		"argument nul":        func(v *Template) { v.Arguments = append(v.Arguments, "foo\x00bar") },
		"cfg newline":         func(v *Template) { v.CFG.Lines = append(v.CFG.Lines, "echo foo\nquit") },
		"cfg quote":           func(v *Template) { v.CFG.Lines = append(v.CFG.Lines, `echo "unterminated`) },
		"empty readiness":     func(v *Template) { v.Readiness.SuccessAll = nil },
		"large readiness":     func(v *Template) { v.Readiness.SuccessAll = []string{strings.Repeat("x", 513)} },
		"negative timeout":    func(v *Template) { v.Timeouts.StopSeconds = -1 },
		"large timeout":       func(v *Template) { v.Timeouts.StartupSeconds = 3601 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			in := fixture(t)
			change(&in)
			_, err := Validate(in)
			if err == nil {
				t.Fatal("accepted invalid template")
			}
			if name == "version" {
				var e *Error
				if !errors.As(err, &e) || e.Code != "UNSUPPORTED_VERSION" {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestExpansionContextsAndNoShell(t *testing.T) {
	in := fixture(t)
	in.Arguments = append(in.Arguments, `literal $HOME; & echo nope`)
	in.CFG.Lines = append(in.CFG.Lines, `log {{log_path}}`, `log "{{log_path}}"`, `echo "escaped \" quote {{instance_id}}"`)
	v := Values{InstanceID: "i_abc", CfgName: "d2core_i_abc_g1.cfg", GamePort: 27015, LogPath: filepath.Join(in.WorkingDirectory, "with space", "engine.log")}
	out, err := Expand(in, v)
	if err != nil {
		t.Fatal(err)
	}
	if out.Arguments[4] != v.LogPath || out.Arguments[len(out.Arguments)-1] != `literal $HOME; & echo nope` {
		t.Fatal(out.Arguments)
	}
	expected := `log "` + strings.ReplaceAll(v.LogPath, `\`, `/`) + `"`
	if strings.Count(out.CFG, expected) != 2 {
		t.Fatalf("quoting mismatch %q", out.CFG)
	}
	if strings.Contains(out.CFG, "{{") || !strings.HasSuffix(out.CFG, "\n") {
		t.Fatal(out.CFG)
	}
	if _, err := os.Stat(filepath.Dir(v.LogPath)); !os.IsNotExist(err) {
		t.Fatal("expansion wrote filesystem")
	}
}

func TestExpansionRejectsUnsafeValues(t *testing.T) {
	in := fixture(t)
	base := Values{InstanceID: "i_ok", CfgName: "safe.cfg", LogPath: filepath.Join(in.WorkingDirectory, "engine.log"), GamePort: 27015}
	for _, change := range []func(*Values){func(v *Values) { v.InstanceID = "bad\";quit" }, func(v *Values) { v.CfgName = "../other.cfg" }, func(v *Values) { v.GamePort = 65536 }, func(v *Values) { v.LogPath = "relative.log" }, func(v *Values) { v.LogPath = filepath.Join(in.WorkingDirectory, "中文.log") }, func(v *Values) { v.LogPath += "\nquit" }} {
		v := base
		change(&v)
		if _, err := Expand(in, v); err == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
}

func TestLoadAndNormalizedRoundtrip(t *testing.T) {
	in := fixture(t)
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(in.WorkingDirectory, "template.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !reflect.DeepEqual(got, data) {
		t.Fatal("source modified")
	}
	encoded, _ := json.Marshal(loaded)
	var decoded Template
	if err := DecodeStrict(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	again, err := Validate(decoded)
	if err != nil || !reflect.DeepEqual(loaded, again) {
		t.Fatalf("normalization not stable: %v", err)
	}
	if _, err := Load("template.json"); err == nil {
		t.Fatal("relative template accepted")
	}
}

func TestLoadRejectsNullOversizeAndDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{"null.json": []byte("null"), "large.json": []byte(strings.Repeat(" ", MaxTemplateBytes+1))} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatal("accepted", name)
		}
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestPublicExampleSyntax(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "examples", "template."+platform+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var tmpl Template
		if err := DecodeStrict(data, &tmpl); err != nil {
			t.Fatal(err)
		}
		// Examples intentionally contain nonexistent deployment paths. Replace
		// only those fields to test the syntax and expansion on the current OS.
		f := fixture(t)
		tmpl.Executable = f.Executable
		tmpl.WorkingDirectory = f.WorkingDirectory
		tmpl.CFG.Directory = f.CFG.Directory
		if _, err := Validate(tmpl); err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
	}
}
