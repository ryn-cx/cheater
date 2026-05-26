package menu

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"

	"cheater/placeholder"
	"cheater/sheets"
)

type Option struct {
	Title       string
	Description string
}

type Asker interface {
	Text(label string) string
	Required(label string) string
	TextWithDefault(label, def string) string
	Confirm(label string, defaultYes bool) bool
	Select(label string, options []Option) int

	SelectCancelable(label string, options []Option) (int, bool)
}

type actionKind int

const (
	actionRun actionKind = iota
	actionAdd
	actionEdit
	actionRemove
	actionQuit
	actionHeader
)

type menuItem struct {
	kind        actionKind
	origin      sheets.Origin
	idx         int
	description string
	command     string
}

func (m menuItem) String() string {
	switch m.kind {
	case actionHeader:
		return m.description + ":"
	case actionRun:
		return fmt.Sprintf("   %2d. %s  —  $ %s", m.idx+1, m.description, m.command)
	case actionAdd:
		return " +  Add a new cheat"
	case actionEdit:
		return " ~  Edit a personal cheat"
	case actionRemove:
		return " -  Remove a personal cheat"
	case actionQuit:
		return " q  Quit"
	}
	return ""
}

func buildMenuItems(personal, community []sheets.Cheat) []menuItem {
	items := make([]menuItem, 0, len(personal)+len(community)+6)
	if len(personal) > 0 {
		items = append(items, menuItem{kind: actionHeader, description: "personal"})
		for i, c := range personal {
			items = append(items, menuItem{kind: actionRun, origin: sheets.OriginPersonal, idx: i, description: c.Description, command: c.Command})
		}
	}
	if len(community) > 0 {
		items = append(items, menuItem{kind: actionHeader, description: "community"})
		for i, c := range community {
			items = append(items, menuItem{kind: actionRun, origin: sheets.OriginCommunity, idx: i, description: c.Description, command: c.Command})
		}
	}
	items = append(items,
		menuItem{kind: actionAdd},
		menuItem{kind: actionEdit},
		menuItem{kind: actionRemove},
		menuItem{kind: actionQuit},
	)
	return items
}

func itemOptions(items []menuItem) []Option {
	out := make([]Option, len(items))
	for i, it := range items {
		if it.kind == actionRun {
			out[i] = Option{
				Title:       fmt.Sprintf("   %2d. %s", it.idx+1, it.description),
				Description: "$ " + it.command,
			}
		} else {
			out[i] = Option{Title: it.String()}
		}
	}
	return out
}

func cheatOptions(cheats []sheets.Cheat) []Option {
	out := make([]Option, len(cheats))
	for i, c := range cheats {
		out[i] = Option{
			Title:       fmt.Sprintf("%2d. %s", i+1, c.Description),
			Description: "$ " + c.Command,
		}
	}
	return out
}

func stringOptions(labels []string) []Option {
	out := make([]Option, len(labels))
	for i, l := range labels {
		out[i] = Option{Title: l}
	}
	return out
}

func shellQuote(s string) string {
	if runtime.GOOS == "windows" {
		return cmdQuote(s)
	}
	return posixQuote(s)
}

func posixQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("@%+=:,./-_", r)) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

func cmdQuote(s string) string {
	if s == "" {
		return `""`
	}
	needs := false
	for _, r := range s {
		if unicode.IsSpace(r) || strings.ContainsRune(`&|<>^"%!()`, r) {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' {
			b.WriteString(`\"`)
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func parseListLine(line string) []string {
	var out []string
	var current strings.Builder
	inQuotes := false
	hasContent := false
	flush := func() {
		if hasContent {
			out = append(out, current.String())
			current.Reset()
			hasContent = false
		}
	}
	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			hasContent = true
		case unicode.IsSpace(r) && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
			hasContent = true
		}
	}
	flush()
	return out
}

func collectFileList(a Asker, name string, spec sheets.ParamSpec, w io.Writer) string {
	fmt.Fprintf(w, "  %s (list of existing files; one path per prompt; quote paths with spaces; blank line to finish)\n", name)
	if spec.Description != "" {
		fmt.Fprintf(w, "  (%s)\n", spec.Description)
	}
	var items []string
	for {
		raw := a.Text("  +")
		if raw == "" {
			break
		}
		for _, token := range parseListLine(raw) {
			if err := sheets.ExistingFile.Validate(token); err != nil {
				fmt.Fprintf(w, "  warning: %v\n", err)
				if !a.Confirm("  include this anyway?", false) {
					continue
				}
			}
			items = append(items, shellQuote(token))
		}
	}
	return strings.Join(items, " ")
}

func collectValue(a Asker, name string, def string, hasDefault bool, spec sheets.ParamSpec, w io.Writer) string {
	if spec.Kind == sheets.ExistingFileList {
		return collectFileList(a, name, spec, w)
	}
	if spec.Description != "" {
		fmt.Fprintf(w, "  (%s)\n", spec.Description)
	}
	label := "  " + name
	if spec.Kind != sheets.ParamUnknown {
		label = fmt.Sprintf("  %s [%s]", name, spec.Kind.Label())
	}
	for {
		var value string
		if hasDefault {
			value = a.TextWithDefault(label, def)
		} else {
			value = a.Text(label)
		}
		if spec.Kind != sheets.ParamUnknown {
			if err := spec.Kind.Validate(value); err != nil {
				fmt.Fprintf(w, "  warning: %v\n", err)
				if !a.Confirm("  use this value anyway?", false) {
					continue
				}
			}
		}
		if spec.Kind == sheets.String {
			return shellQuote(value)
		}
		return value
	}
}

type typeChoice struct {
	label string
	kind  sheets.ParamType
}

var typeChoices = []typeChoice{
	{"(skip — no type)", sheets.ParamUnknown},
	{"existing file", sheets.ExistingFile},
	{"existing dir", sheets.ExistingDir},
	{"existing path", sheets.ExistingPath},
	{"path", sheets.Path},
	{"integer", sheets.Integer},
	{"string", sheets.String},
	{"list of existing files", sheets.ExistingFileList},
}

func askType(a Asker, label string) sheets.ParamType {
	labels := make([]string, len(typeChoices))
	for i, c := range typeChoices {
		labels[i] = c.label
	}
	return typeChoices[a.Select(label, stringOptions(labels))].kind
}

type runAction int

const (
	runBuild runAction = iota
	runCopy
)

func askRunAction(a Asker, cheat sheets.Cheat) runAction {
	idx := a.Select(
		fmt.Sprintf("'%s' — what now?", cheat.Description),
		[]Option{
			{Title: "Build and run"},
			{Title: "Copy to clipboard"},
		},
	)
	if idx == 1 {
		return runCopy
	}
	return runBuild
}

func runCheat(a Asker, cheat sheets.Cheat, w io.Writer) {
	action := askRunAction(a, cheat)

	placeholders := placeholder.Extract(cheat.Command)
	values := make(map[string]string, len(placeholders))
	if len(placeholders) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Filling blanks for: %s\n", cheat.Description)
		for _, name := range placeholders {
			def, hasDefault := cheat.Defaults[name]
			spec := cheat.Params[name]
			values[name] = collectValue(a, name, def, hasDefault, spec, w)
		}
	}
	final := placeholder.Assemble(cheat.Command, values)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  $ %s\n", final)

	switch action {
	case runCopy:
		if err := clipboard.WriteAll(final); err != nil {
			fmt.Fprintf(os.Stderr, "failed to copy to clipboard: %v\n", err)
			return
		}
		fmt.Fprintln(w, "copied to clipboard.")
	case runBuild:
		if !a.Confirm("Run this?", true) {
			fmt.Fprintln(w, "aborted.")
			return
		}
		cmd := shellCommand(final)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				fmt.Fprintf(w, "\n(exit %d)\n", ee.ExitCode())
				return
			}
			fmt.Fprintf(os.Stderr, "\nfailed to spawn: %v\n", err)
		}
	}
}

func shellCommand(cmd string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmd)
	}
	return exec.Command("sh", "-c", cmd)
}

func addCheat(a Asker, w io.Writer) sheets.Cheat {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Add a new cheat.")
	description := a.Required("description")
	fmt.Fprintln(w, "Use <name> for blanks you'll be prompted to fill in later.")
	command := a.Required("command")

	placeholders := placeholder.Extract(command)
	defaults := map[string]string{}
	params := map[string]sheets.ParamSpec{}

	if len(placeholders) > 0 && a.Confirm("Set defaults for any placeholders?", false) {
		fmt.Fprintln(w, "(blank to skip a placeholder)")
		for _, name := range placeholders {
			if v := a.Text(fmt.Sprintf("  default for %s", name)); v != "" {
				defaults[name] = v
			}
		}
	}
	if len(placeholders) > 0 && a.Confirm("Set parameter types/descriptions?", false) {
		for _, name := range placeholders {
			kind := askType(a, fmt.Sprintf("  type for %s", name))
			desc := a.Text(fmt.Sprintf("  description for %s", name))
			if kind != sheets.ParamUnknown || desc != "" {
				params[name] = sheets.ParamSpec{Kind: kind, Description: desc}
			}
		}
	}
	return sheets.Cheat{Description: description, Command: command, Defaults: defaults, Params: params}
}

func editCheat(a Asker, existing sheets.Cheat, w io.Writer) sheets.Cheat {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Edit '%s'. Hit Enter to keep each current value.\n", existing.Description)
	description := a.TextWithDefault("description", existing.Description)
	command := a.TextWithDefault("command", existing.Command)

	placeholders := placeholder.Extract(command)
	defaults := editDefaults(a, placeholders, existing.Defaults, w)
	params := editParams(a, placeholders, existing.Params, w)

	return sheets.Cheat{Description: description, Command: command, Defaults: defaults, Params: params}
}

func editDefaults(a Asker, placeholders []string, existing map[string]string, w io.Writer) map[string]string {
	if len(placeholders) == 0 || !a.Confirm("Re-set defaults?", false) {

		if len(existing) == 0 {
			return map[string]string{}
		}
		out := make(map[string]string, len(existing))
		for _, name := range placeholders {
			if v, ok := existing[name]; ok {
				out[name] = v
			}
		}
		return out
	}
	fmt.Fprintln(w, "(blank to skip; Enter keeps the current default)")
	out := map[string]string{}
	for _, name := range placeholders {
		label := fmt.Sprintf("  default for %s", name)
		var v string
		if prev, ok := existing[name]; ok {
			v = a.TextWithDefault(label, prev)
		} else {
			v = a.Text(label)
		}
		if v != "" {
			out[name] = v
		}
	}
	return out
}

func editParams(a Asker, placeholders []string, existing map[string]sheets.ParamSpec, w io.Writer) map[string]sheets.ParamSpec {
	if len(placeholders) == 0 || !a.Confirm("Re-set parameter types/descriptions?", false) {
		if len(existing) == 0 {
			return map[string]sheets.ParamSpec{}
		}
		out := make(map[string]sheets.ParamSpec, len(existing))
		for _, name := range placeholders {
			if prev, ok := existing[name]; ok {
				out[name] = prev
			}
		}
		return out
	}
	out := map[string]sheets.ParamSpec{}
	for _, name := range placeholders {
		prev, hadPrev := existing[name]
		label := fmt.Sprintf("  type for %s", name)
		if hadPrev && prev.Kind != sheets.ParamUnknown {
			label = fmt.Sprintf("  type for %s (current: %s)", name, prev.Kind.Label())
		}
		kind := askType(a, label)
		descLabel := fmt.Sprintf("  description for %s", name)
		var desc string
		if hadPrev && prev.Description != "" {
			desc = a.TextWithDefault(descLabel, prev.Description)
		} else {
			desc = a.Text(descLabel)
		}
		if kind != sheets.ParamUnknown || desc != "" {
			out[name] = sheets.ParamSpec{Kind: kind, Description: desc}
		}
	}
	return out
}

func pickToEdit(a Asker, cheats []sheets.Cheat, w io.Writer) (int, bool) {
	if len(cheats) == 0 {
		fmt.Fprintln(w, "nothing to edit.")
		return 0, false
	}
	return a.SelectCancelable("Edit which? (Esc to cancel)", cheatOptions(cheats))
}

func pickToRemove(a Asker, cheats []sheets.Cheat, w io.Writer) (int, bool) {
	if len(cheats) == 0 {
		fmt.Fprintln(w, "nothing to remove.")
		return 0, false
	}
	idx, ok := a.SelectCancelable("Remove which? (Esc to cancel)", cheatOptions(cheats))
	if !ok {
		return 0, false
	}
	if !a.Confirm(fmt.Sprintf("Remove '%s'?", cheats[idx].Description), false) {
		return 0, false
	}
	return idx, true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func printSection(w io.Writer, header string, apps map[string][]sheets.Cheat) {
	if len(apps) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", header)
	keys := sortedKeys(apps)
	width := 0
	for _, k := range keys {
		if len(k) > width {
			width = len(k)
		}
	}
	for _, app := range keys {
		n := len(apps[app])
		plural := "s"
		if n == 1 {
			plural = ""
		}
		fmt.Fprintf(w, "  %-*s   %d cheat%s\n", width, app, n, plural)
	}
}

func ModeList(dirs sheets.Dirs, w io.Writer) error {
	personal, community, err := sheets.ListBoth(dirs)
	if err != nil {
		return fmt.Errorf("listing apps: %w", err)
	}
	if len(personal) == 0 && len(community) == 0 {
		fmt.Fprintln(w, "(no cheats stored yet)")
		return nil
	}
	printSection(w, "personal", personal)
	if len(personal) > 0 && len(community) > 0 {
		fmt.Fprintln(w)
	}
	printSection(w, "community", community)
	return nil
}

func ModeApp(a Asker, dirs sheets.Dirs, app string, w io.Writer) error {
	if err := sheets.ValidateName(app); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	personal, community, err := sheets.LoadBoth(dirs, app)
	if err != nil {
		return err
	}
	for {
		items := buildMenuItems(personal, community)
		label := fmt.Sprintf("'%s' — pick a cheat or action", app)
		if len(personal) == 0 && len(community) == 0 {
			label = fmt.Sprintf("'%s' has no cheats yet — pick an action", app)
		}
		chosen := items[a.Select(label, itemOptions(items))]
		switch chosen.kind {
		case actionHeader:

		case actionRun:
			var cheat sheets.Cheat
			if chosen.origin == sheets.OriginCommunity {
				cheat = community[chosen.idx]
			} else {
				cheat = personal[chosen.idx]
			}
			runCheat(a, cheat, w)
			return nil
		case actionAdd:
			personal = append(personal, addCheat(a, w))
			if err := sheets.Save(dirs.Personal, app, personal); err != nil {
				return err
			}
			fmt.Fprintln(w, "added.")
		case actionEdit:
			i, ok := pickToEdit(a, personal, w)
			if !ok {
				return nil
			}
			personal[i] = editCheat(a, personal[i], w)
			if err := sheets.Save(dirs.Personal, app, personal); err != nil {
				return err
			}
			fmt.Fprintln(w, "updated.")
		case actionRemove:
			i, ok := pickToRemove(a, personal, w)
			if !ok {
				return nil
			}
			removed := personal[i]
			personal = append(personal[:i], personal[i+1:]...)
			if err := sheets.Save(dirs.Personal, app, personal); err != nil {
				return err
			}
			fmt.Fprintf(w, "removed '%s'.\n", removed.Description)
		case actionQuit:
			return nil
		}
	}
}
