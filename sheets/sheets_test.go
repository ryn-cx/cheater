package sheets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateName_AcceptsNormalNames(t *testing.T) {
	for _, name := range []string{"git", "ffmpeg", "git-flow", "g++", "i3wm", "kubectl_get"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("%q: unexpected error %v", name, err)
		}
	}
}

func TestValidateName_RejectsBadNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "../etc", "a:b", "a*b", "a|b"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("%q: expected error, got nil", name)
		}
	}
}

func TestRoundTrip_PreservesEscapesAndDefaults(t *testing.T) {
	dir := t.TempDir()
	multi := "echo line1\nline2 <name>"
	cheats := []Cheat{{
		Description: "multi",
		Command:     multi,
		Defaults:    map[string]string{"name": "world"},
		Params:      map[string]ParamSpec{},
	}}
	if err := Save(dir, "demo", cheats); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(appPath(dir, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `\n`) {
		t.Errorf("newline not JSON-escaped: %s", raw)
	}
	if !strings.Contains(string(raw), `"defaults"`) {
		t.Errorf("defaults missing: %s", raw)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		t.Errorf("top-level should be an array: %s", raw)
	}
	loaded, err := Load(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Command != multi {
		t.Errorf("command round-trip: got %q, want %q", loaded[0].Command, multi)
	}
	if loaded[0].Defaults["name"] != "world" {
		t.Errorf("default round-trip: got %q, want %q", loaded[0].Defaults["name"], "world")
	}
}

func TestLoad_OptionalFieldsMayBeOmitted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(appPath(dir, "git"), []byte(`[{"description":"old","command":"git status"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, "git")
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Description != "old" {
		t.Errorf("description: got %q", loaded[0].Description)
	}
	if len(loaded[0].Defaults) != 0 {
		t.Errorf("expected empty defaults, got %v", loaded[0].Defaults)
	}
	if len(loaded[0].Params) != 0 {
		t.Errorf("expected empty params, got %v", loaded[0].Params)
	}
}

func TestEmptyDefaultsAndParams_OmittedFromJSON(t *testing.T) {
	dir := t.TempDir()
	cheats := []Cheat{{Description: "d", Command: "echo hi"}}
	if err := Save(dir, "x", cheats); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(appPath(dir, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "defaults") {
		t.Errorf("expected no defaults key: %s", raw)
	}
	if strings.Contains(string(raw), "params") {
		t.Errorf("expected no params key: %s", raw)
	}
}

func TestBOMPrefixedFileLoads(t *testing.T) {
	dir := t.TempDir()
	body := `[{"description":"x","command":"git status"}]`
	if err := os.WriteFile(appPath(dir, "git"), append([]byte{0xEF, 0xBB, 0xBF}, body...), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, "git")
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Description != "x" {
		t.Errorf("got %q", loaded[0].Description)
	}
}

func TestSaveEmpty_DeletesExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "app", []Cheat{{Description: "x", Command: "echo hi"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(appPath(dir, "app")); err != nil {
		t.Fatal("file should exist")
	}
	if err := Save(dir, "app", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(appPath(dir, "app")); !os.IsNotExist(err) {
		t.Errorf("file should be gone, got %v", err)
	}
}

func TestSaveEmpty_OnMissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "ghost", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(appPath(dir, "ghost")); !os.IsNotExist(err) {
		t.Errorf("expected file to not exist, got %v", err)
	}
}

func TestList_SkipsNonJSONAndEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "git", []Cheat{{Description: "a", Command: "git a"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	apps, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Errorf("expected 1 app, got %d", len(apps))
	}
	if _, ok := apps["git"]; !ok {
		t.Errorf("expected 'git' to be present")
	}
}

func TestList_OnMissingDirReturnsEmpty(t *testing.T) {
	apps, err := List(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Errorf("expected empty, got %v", apps)
	}
}

func TestParamSpec_SerializesTypeAndDescription(t *testing.T) {
	c := Cheat{Description: "d", Command: "c", Params: map[string]ParamSpec{
		"archive": {Kind: ExistingFile, Description: "The archive"},
	}}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"archive":{`, `"type":"existing_file"`, `"description":"The archive"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestParamSpec_OmitsEmptyDescription(t *testing.T) {
	c := Cheat{Description: "d", Command: "c", Params: map[string]ParamSpec{
		"n": {Kind: Integer},
	}}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"type":"integer"`) {
		t.Errorf("missing type: %s", s)
	}
	if strings.Contains(s, `"description":""`) {
		t.Errorf("empty description should be omitted: %s", s)
	}
}

func TestParamSpec_OmitsMissingType(t *testing.T) {
	c := Cheat{Description: "d", Command: "c", Params: map[string]ParamSpec{
		"files": {Description: "Space-separated list"},
	}}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"description":"Space-separated list"`) {
		t.Errorf("missing description: %s", s)
	}
	if strings.Contains(s, `"type"`) {
		t.Errorf("type should be omitted: %s", s)
	}
}

func TestParamSpec_RejectsUnknownType(t *testing.T) {
	dir := t.TempDir()
	body := `[{"description":"x","command":"c","params":{"n":{"type":"weird_type"}}}]`
	if err := os.WriteFile(appPath(dir, "foo"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, "foo")
	if err == nil {
		t.Fatal("expected error for unknown param type, got nil")
	}
	if !strings.Contains(err.Error(), "weird_type") {
		t.Errorf("expected error to mention 'weird_type', got: %v", err)
	}
}

func TestParamSpec_RoundTripsThroughJSON(t *testing.T) {
	raw := `{
		"description":"d",
		"command":"c",
		"params": {
			"archive": {"type": "existing_file", "description": "Input archive"}
		}
	}`
	var c Cheat
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	spec := c.Params["archive"]
	if spec.Kind != ExistingFile {
		t.Errorf("kind: got %v", spec.Kind)
	}
	if spec.Description != "Input archive" {
		t.Errorf("description: got %q", spec.Description)
	}
}

func TestValidate_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExistingFile.Validate(file); err != nil {
		t.Errorf("expected ok, got %v", err)
	}
	if err := ExistingFile.Validate(dir); err == nil {
		t.Errorf("expected error for dir")
	}
	if err := ExistingFile.Validate(filepath.Join(dir, "missing")); err == nil {
		t.Errorf("expected error for missing path")
	}
}

func TestValidate_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExistingDir.Validate(dir); err != nil {
		t.Errorf("dir: got %v", err)
	}
	if err := ExistingDir.Validate(file); err == nil {
		t.Errorf("expected error for file")
	}
	if err := ExistingDir.Validate(filepath.Join(dir, "missing")); err == nil {
		t.Errorf("expected error for missing path")
	}
}

func TestValidate_ExistingPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExistingPath.Validate(dir); err != nil {
		t.Errorf("dir: got %v", err)
	}
	if err := ExistingPath.Validate(file); err != nil {
		t.Errorf("file: got %v", err)
	}
	if err := ExistingPath.Validate(filepath.Join(dir, "missing")); err == nil {
		t.Errorf("expected error for missing path")
	}
}

func TestValidate_Path_AcceptsTypicalPaths(t *testing.T) {
	for _, v := range []string{
		"foo.txt",
		"path/to/file.7z",
		"/absolute/unix/path",
		`C:\Users\me\file.txt`,
		"../relative/path",
		"name with spaces.txt",
		"no-extension",
	} {
		if err := Path.Validate(v); err != nil {
			t.Errorf("%q: got %v", v, err)
		}
	}
}

func TestValidate_Path_DoesNotTouchFilesystem(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	phantom := filepath.Join(dir, "phantom.txt")

	if err := Path.Validate(existing); err != nil {
		t.Errorf("existing: %v", err)
	}
	if err := Path.Validate(phantom); err != nil {
		t.Errorf("phantom: %v", err)
	}

	raw, _ := os.ReadFile(existing)
	if string(raw) != "original" {
		t.Errorf("existing file mutated: %q", raw)
	}
	if _, err := os.Stat(phantom); !os.IsNotExist(err) {
		t.Errorf("phantom file should not exist, got %v", err)
	}
}

func TestValidate_Path_RejectsEmpty(t *testing.T) {
	if err := Path.Validate(""); err == nil {
		t.Errorf("expected error for empty")
	}
}

func TestValidate_Path_RejectsWindowsIllegalChars(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only: these characters are valid in Unix filenames")
	}
	for _, v := range []string{
		`C:\foo<bar.txt`,
		`C:\foo>bar.txt`,
		`C:\foo|bar.txt`,
		`C:\foo*bar.txt`,
		`C:\foo?bar.txt`,
		`C:\foo"bar.txt`,
	} {
		if err := Path.Validate(v); err == nil {
			t.Errorf("%q: expected error, got nil", v)
		}
	}
}

func TestValidate_String_RejectsBlank(t *testing.T) {
	for _, v := range []string{"", " ", "   ", "\t", "\n", " \t\n "} {
		if err := String.Validate(v); err == nil {
			t.Errorf("%q: expected error, got nil", v)
		}
	}
}

func TestValidate_String_AcceptsNonBlank(t *testing.T) {
	for _, v := range []string{"x", "hello world", "  has content  ", "0", "-"} {
		if err := String.Validate(v); err != nil {
			t.Errorf("%q: unexpected error %v", v, err)
		}
	}
}

func TestValidate_Integer(t *testing.T) {
	for _, v := range []string{"42", "-7", "0"} {
		if err := Integer.Validate(v); err != nil {
			t.Errorf("%q: %v", v, err)
		}
	}
	for _, v := range []string{"3.14", "nope", ""} {
		if err := Integer.Validate(v); err == nil {
			t.Errorf("%q: expected error", v)
		}
	}
}
