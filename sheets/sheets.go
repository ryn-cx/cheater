package sheets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ParamType string

const (
	ParamUnknown     ParamType = ""
	ExistingFile     ParamType = "existing_file"
	ExistingDir      ParamType = "existing_dir"
	ExistingPath     ParamType = "existing_path"
	Path             ParamType = "path"
	Integer          ParamType = "integer"
	String           ParamType = "string"
	ExistingFileList ParamType = "existing_file_list"
)

func (p *ParamType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	candidate := ParamType(s)
	switch candidate {
	case ExistingFile, ExistingDir, ExistingPath, Path, Integer, String, ExistingFileList:
		*p = candidate
		return nil
	}
	return fmt.Errorf("unknown param type: %q", s)
}

func (p ParamType) Label() string {
	switch p {
	case ExistingFile:
		return "existing file"
	case ExistingDir:
		return "existing dir"
	case ExistingPath:
		return "existing path"
	case Path:
		return "path"
	case Integer:
		return "integer"
	case String:
		return "string"
	case ExistingFileList:
		return "list of existing files"
	}
	return ""
}

func (p ParamType) Validate(value string) error {
	switch p {
	case ExistingFile:
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("'%s' does not exist", value)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("'%s' is not a regular file", value)
		}
	case ExistingDir:
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("'%s' does not exist", value)
		}
		if !info.IsDir() {
			return fmt.Errorf("'%s' is not a directory", value)
		}
	case ExistingPath:
		if _, err := os.Stat(value); err != nil {
			return fmt.Errorf("'%s' does not exist", value)
		}
	case Path:
		if value == "" {
			return errors.New("path is empty")
		}

		if _, err := os.Stat(value); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("'%s' is not a valid path on this OS", value)
		}
	case Integer:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("'%s' is not an integer", value)
		}
	case String:
		if strings.TrimSpace(value) == "" {
			return errors.New("value is blank")
		}
	}
	return nil
}

type ParamSpec struct {
	Kind        ParamType `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
}

type Cheat struct {
	Description string               `json:"description"`
	Command     string               `json:"command"`
	Defaults    map[string]string    `json:"defaults,omitempty"`
	Params      map[string]ParamSpec `json:"params,omitempty"`
}

type Origin int

const (
	OriginPersonal Origin = iota
	OriginCommunity
)

func (o Origin) Label() string {
	switch o {
	case OriginPersonal:
		return "personal"
	case OriginCommunity:
		return "community"
	}
	return ""
}

type Dirs struct {
	Personal  string
	Community string
}

func DefaultRoot() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cheater"), nil
}

func DefaultDirs() (Dirs, error) {
	root, err := DefaultRoot()
	if err != nil {
		return Dirs{}, err
	}
	sheets := filepath.Join(root, "cheatsheets")
	return Dirs{
		Personal:  filepath.Join(sheets, "personal"),
		Community: filepath.Join(sheets, "community"),
	}, nil
}

func ValidateName(app string) error {
	if app == "" {
		return errors.New("app name cannot be empty")
	}
	if app == "." || app == ".." {
		return fmt.Errorf("invalid app name: '%s'", app)
	}
	for _, r := range app {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			return fmt.Errorf("invalid character in app name: '%c'", r)
		}
	}
	return nil
}

func appPath(dir, app string) string {
	return filepath.Join(dir, app+".json")
}

var bom = []byte{0xEF, 0xBB, 0xBF}

func parseCheats(raw []byte, path string) ([]Cheat, error) {
	var cheats []Cheat
	if err := json.Unmarshal(bytes.TrimPrefix(raw, bom), &cheats); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return cheats, nil
}

func Load(dir, app string) ([]Cheat, error) {
	path := appPath(dir, app)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return parseCheats(raw, path)
}

func List(dir string) (map[string][]Cheat, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]Cheat{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	out := make(map[string][]Cheat, len(entries))
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".json" || !e.Type().IsRegular() {
			continue
		}
		full := filepath.Join(dir, name)
		raw, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", full, err)
			continue
		}
		cheats, err := parseCheats(raw, full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", full, err)
			continue
		}
		if len(cheats) > 0 {
			out[strings.TrimSuffix(name, ".json")] = cheats
		}
	}
	return out, nil
}

func Save(dir, app string, cheats []Cheat) error {
	path := appPath(dir, app)
	if len(cheats) == 0 {
		err := os.Remove(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cheats); err != nil {
		return fmt.Errorf("serializing cheats: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func ImportRepo(url, dst string, force bool, w io.Writer) (imported, skipped int, err error) {
	tmp, err := os.MkdirTemp("", "cheater-import-*")
	if err != nil {
		return 0, 0, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("git", "clone", "--depth", "1", url, tmp)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("cloning %s: %w", url, err)
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, 0, fmt.Errorf("cannot create %s: %w", dst, err)
	}

	walkErr := filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cheats, err := parseCheats(raw, path)
		if err != nil || len(cheats) == 0 {
			return nil
		}
		target := filepath.Join(dst, d.Name())
		exists := true
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			exists = false
		} else if err != nil {
			return err
		}
		if exists && !force {
			fmt.Fprintf(w, "skipped %s (already exists)\n", d.Name())
			skipped++
			return nil
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return err
		}
		if exists {
			fmt.Fprintf(w, "overwrote %s\n", d.Name())
		} else {
			fmt.Fprintf(w, "imported %s\n", d.Name())
		}
		imported++
		return nil
	})
	if walkErr != nil {
		return imported, skipped, walkErr
	}
	return imported, skipped, nil
}

func LoadBoth(d Dirs, app string) (personal, community []Cheat, err error) {
	if personal, err = Load(d.Personal, app); err != nil {
		return nil, nil, err
	}
	if community, err = Load(d.Community, app); err != nil {
		return nil, nil, err
	}
	return personal, community, nil
}

func ListBoth(d Dirs) (personal, community map[string][]Cheat, err error) {
	if personal, err = List(d.Personal); err != nil {
		return nil, nil, err
	}
	if community, err = List(d.Community); err != nil {
		return nil, nil, err
	}
	return personal, community, nil
}
