package menu

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cheater/sheets"
)

type mockAsker struct {
	texts             []string
	confirms          []bool
	selects           []int
	cancelableSelects []cancelablePick
}

type cancelablePick struct {
	idx      int
	canceled bool
}

func newMock() *mockAsker { return &mockAsker{} }

func (m *mockAsker) withText(v string) *mockAsker { m.texts = append(m.texts, v); return m }

func (m *mockAsker) withConfirm(v bool) *mockAsker {
	m.confirms = append(m.confirms, v)
	return m
}

func (m *mockAsker) withSelect(idx int) *mockAsker {
	m.selects = append(m.selects, idx)
	return m
}

func (m *mockAsker) withCancelableSelect(idx int) *mockAsker {
	m.cancelableSelects = append(m.cancelableSelects, cancelablePick{idx: idx})
	return m
}

func (m *mockAsker) withCancel() *mockAsker {
	m.cancelableSelects = append(m.cancelableSelects, cancelablePick{canceled: true})
	return m
}

func (m *mockAsker) Text(label string) string {
	if len(m.texts) == 0 {
		panic("mockAsker: no text response queued")
	}
	v := m.texts[0]
	m.texts = m.texts[1:]
	return v
}

func (m *mockAsker) Required(label string) string {
	for {
		v := m.Text(label)
		if v != "" {
			return v
		}
	}
}

func (m *mockAsker) TextWithDefault(label, def string) string {
	if len(m.texts) == 0 {
		panic("mockAsker: no text_with_default response queued")
	}
	v := m.texts[0]
	m.texts = m.texts[1:]
	if v == "" {
		return def
	}
	return v
}

func (m *mockAsker) Confirm(label string, defaultYes bool) bool {
	if len(m.confirms) == 0 {
		panic("mockAsker: no confirm response queued")
	}
	v := m.confirms[0]
	m.confirms = m.confirms[1:]
	return v
}

func (m *mockAsker) Select(label string, options []Option) int {
	if len(m.selects) == 0 {
		panic("mockAsker: no select response queued")
	}
	v := m.selects[0]
	m.selects = m.selects[1:]
	return v
}

func (m *mockAsker) SelectCancelable(label string, options []Option) (int, bool) {
	if len(m.cancelableSelects) == 0 {
		panic("mockAsker: no cancelable_select response queued")
	}
	v := m.cancelableSelects[0]
	m.cancelableSelects = m.cancelableSelects[1:]
	if v.canceled {
		return -1, false
	}
	return v.idx, true
}

var _ Asker = (*mockAsker)(nil)

func actionsOffset(personal, community int) int {
	n := personal + community
	if personal > 0 {
		n++
	}
	if community > 0 {
		n++
	}
	return n
}

func addSelect(personal, community int) int    { return actionsOffset(personal, community) }
func editSelect(personal, community int) int   { return actionsOffset(personal, community) + 1 }
func removeSelect(personal, community int) int { return actionsOffset(personal, community) + 2 }
func quitSelect(personal, community int) int   { return actionsOffset(personal, community) + 3 }

func fixture(t *testing.T) (string, sheets.Dirs) {
	t.Helper()
	dir := t.TempDir()
	return dir, sheets.Dirs{
		Personal:  dir,
		Community: filepath.Join(dir, "__nonexistent_community__"),
	}
}

func seed(t *testing.T, dir, app, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, app+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readApp(t *testing.T, dir, app string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, app+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var v []map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func runApp(t *testing.T, a Asker, dirs sheets.Dirs, app string) string {
	t.Helper()
	var w bytes.Buffer
	if err := ModeApp(a, dirs, app, &w); err != nil {
		t.Fatal(err)
	}
	return w.String()
}

func TestAddCheat_WithPlaceholdersAndDefault_Persists(t *testing.T) {
	dir, dirs := fixture(t)
	a := newMock().
		withSelect(addSelect(0, 0)).
		withText("squash N commits").
		withText("git rebase -i HEAD~<n>").
		withConfirm(true).
		withText("5").
		withConfirm(false).
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "git")

	v := readApp(t, dir, "git")
	if v[0]["description"] != "squash N commits" {
		t.Errorf("description: %v", v[0]["description"])
	}
	if v[0]["command"] != "git rebase -i HEAD~<n>" {
		t.Errorf("command: %v", v[0]["command"])
	}
	if got := v[0]["defaults"].(map[string]any)["n"]; got != "5" {
		t.Errorf("defaults[n]: %v", got)
	}
}

func TestAddCheat_NoPlaceholders_NeverAsksAboutDefaults(t *testing.T) {
	dir, dirs := fixture(t)
	a := newMock().
		withSelect(addSelect(0, 0)).
		withText("status").
		withText("git status").
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "git")

	raw, _ := os.ReadFile(filepath.Join(dir, "git.json"))
	if bytes.Contains(raw, []byte("defaults")) {
		t.Errorf("defaults key leaked: %s", raw)
	}
}

func TestRemoveCheat_Persists(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[
		{"description":"a","command":"git a"},
		{"description":"b","command":"git b"}
	]`)
	a := newMock().
		withSelect(removeSelect(2, 0)).
		withCancelableSelect(0).
		withConfirm(true).
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "git")

	v := readApp(t, dir, "git")
	if len(v) != 1 {
		t.Fatalf("expected 1 cheat, got %d", len(v))
	}
	if v[0]["description"] != "b" {
		t.Errorf("expected 'b', got %v", v[0]["description"])
	}
}

func TestRemoveLastCheat_DeletesTheAppFile(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[{"description":"only","command":"git status"}]`)
	a := newMock().
		withSelect(removeSelect(1, 0)).
		withCancelableSelect(0).
		withConfirm(true).
		withSelect(quitSelect(0, 0))
	runApp(t, a, dirs, "git")

	if _, err := os.Stat(filepath.Join(dir, "git.json")); !os.IsNotExist(err) {
		t.Errorf("git.json should be deleted; stat err=%v", err)
	}
}

func TestRemoveDeclined_KeepsCheat(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[{"description":"keep","command":"git status"}]`)
	a := newMock().
		withSelect(removeSelect(1, 0)).
		withCancelableSelect(0).
		withConfirm(false)
	runApp(t, a, dirs, "git")

	v := readApp(t, dir, "git")
	if v[0]["description"] != "keep" {
		t.Errorf("expected 'keep', got %v", v[0]["description"])
	}
}

func TestRemoveCanceledAtPicker_KeepsCheat(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[{"description":"keep","command":"git status"}]`)
	a := newMock().
		withSelect(removeSelect(1, 0)).
		withCancel()
	runApp(t, a, dirs, "git")

	v := readApp(t, dir, "git")
	if v[0]["description"] != "keep" {
		t.Errorf("expected 'keep', got %v", v[0]["description"])
	}
}

func TestRunCommunityCheat_WithNoPersonalCheats(t *testing.T) {

	root := t.TempDir()
	personal := filepath.Join(root, "personal")
	community := filepath.Join(root, "community")
	seed(t, community, "screen", `[{"description":"reattach","command":"screen -r"}]`)
	dirs := sheets.Dirs{Personal: personal, Community: community}

	a := newMock().
		withSelect(1).
		withSelect(0).
		withConfirm(false)

	var w bytes.Buffer
	if err := ModeApp(a, dirs, "screen", &w); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(w.Bytes(), []byte("screen -r")) {
		t.Errorf("expected preview to mention 'screen -r', got: %s", w.String())
	}
}

func TestRunCheat_CopyPath_DoesNotPromptToRun(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[{"description":"status","command":"git status"}]`)

	a := newMock().
		withSelect(1).
		withSelect(1)

	var w bytes.Buffer
	if err := ModeApp(a, dirs, "git", &w); err != nil {

		t.Fatal(err)
	}
	if !bytes.Contains(w.Bytes(), []byte("git status")) {
		t.Errorf("expected preview to mention 'git status', got: %s", w.String())
	}

}

func TestEditCheat_ChangesDescription_KeepsCommand(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "git", `[{"description":"old desc","command":"git status"}]`)
	a := newMock().
		withSelect(editSelect(1, 0)).
		withCancelableSelect(0).
		withText("new desc").
		withText("").
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "git")

	v := readApp(t, dir, "git")
	if v[0]["description"] != "new desc" || v[0]["command"] != "git status" {
		t.Errorf("got %v", v[0])
	}
}

func TestEditCommand_DropsOrphanedDefault(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "demo", `[{
		"description":"old",
		"command":"echo <a> <b>",
		"defaults":{"a":"1","b":"2"}
	}]`)
	a := newMock().
		withSelect(editSelect(1, 0)).
		withCancelableSelect(0).
		withText("").
		withText("echo <a>").
		withConfirm(false).
		withConfirm(false).
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "demo")

	v := readApp(t, dir, "demo")
	if v[0]["command"] != "echo <a>" {
		t.Errorf("command: %v", v[0]["command"])
	}
	defaults := v[0]["defaults"].(map[string]any)
	if defaults["a"] != "1" {
		t.Errorf("defaults[a]: %v", defaults["a"])
	}
	if _, exists := defaults["b"]; exists {
		t.Errorf("b should have been dropped: %v", defaults)
	}
}

func TestEditPreservesParamSpec_WhenNotResetting(t *testing.T) {
	dir, dirs := fixture(t)
	seed(t, dir, "7z", `[{
		"description":"extract",
		"command":"7z e <archive>",
		"params":{"archive":{"type":"existing_file","description":"the archive"}}
	}]`)
	a := newMock().
		withSelect(editSelect(1, 0)).
		withCancelableSelect(0).
		withText("").
		withText("").
		withConfirm(false).
		withConfirm(false).
		withSelect(quitSelect(1, 0))
	runApp(t, a, dirs, "7z")

	v := readApp(t, dir, "7z")
	p := v[0]["params"].(map[string]any)["archive"].(map[string]any)
	if p["type"] != "existing_file" || p["description"] != "the archive" {
		t.Errorf("params spec mangled: %v", p)
	}
}

func TestParseListLine(t *testing.T) {
	got := parseListLine(`a "b c" d`)
	want := []string{"a", "b c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(parseListLine("")) != 0 {
		t.Errorf("expected empty for blank line")
	}
}

func TestCmdQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo", "foo"},
		{"path/to/file.txt", "path/to/file.txt"},
		{"hello world", `"hello world"`},
		{"a & b", `"a & b"`},
		{`say "hi"`, `"say \"hi\""`},
		{"", `""`},
	}
	for _, c := range cases {
		if got := cmdQuote(c.in); got != c.want {
			t.Errorf("cmdQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPosixQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo", "foo"},
		{"path/to/file.txt", "path/to/file.txt"},
		{"hello world", `'hello world'`},
		{"a's", `'a'\''s'`},
		{"", `''`},
	}
	for _, c := range cases {
		if got := posixQuote(c.in); got != c.want {
			t.Errorf("posixQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestModeList_EmptyShowsHint(t *testing.T) {
	_, dirs := fixture(t)
	var w bytes.Buffer
	if err := ModeList(dirs, &w); err != nil {
		t.Fatal(err)
	}
	if got := w.String(); got != "(no cheats stored yet)\n" {
		t.Errorf("unexpected output: %q", got)
	}
}

func TestModeList_ShowsPersonalAndCommunitySections(t *testing.T) {
	dir := t.TempDir()
	personal := filepath.Join(dir, "personal")
	community := filepath.Join(dir, "community")
	seed(t, personal, "git", `[{"description":"a","command":"git a"}]`)
	seed(t, community, "7z", `[{"description":"x","command":"7z x <a>"}]`)
	dirs := sheets.Dirs{Personal: personal, Community: community}

	var w bytes.Buffer
	if err := ModeList(dirs, &w); err != nil {
		t.Fatal(err)
	}
	out := w.String()
	for _, want := range []string{"personal:", "community:", "git", "7z"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}
