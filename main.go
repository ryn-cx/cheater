package main

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cheater/asker"
	"cheater/menu"
	"cheater/sheets"
)

//go:embed cheatsheets/community/*.json
var communityFiles embed.FS

const embeddedCommunityDir = "cheatsheets/community"

const usageHelp = `cheater is like cheat but it can also build and run commands for you. Pick a
cheatsheet, pick a recipe, fill in the <blanks>, and cheater assembles and
runs the command.

Usage:
  cheater [cheatsheet] [flags]

Examples:
  To view (and run) cheats for 7z:
    cheater 7z

  To list every stored cheatsheet:
    cheater -l

Flags:
  -h, --help   help for cheater
  -l, --list   List cheatsheets
`

func main() {
	dirs, err := sheets.DefaultDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := installCommunity(dirs.Community); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not install community cheats: %v\n", err)
	}

	if len(os.Args) > 2 {
		fmt.Fprintf(os.Stderr, "accepts at most 1 arg(s), received %d\n", len(os.Args)-1)
		os.Exit(1)
	}

	var target string
	if len(os.Args) > 1 {
		target = os.Args[1]
	}

	switch target {
	case "", "--help", "-h":
		fmt.Print(usageHelp)
	case "--list", "-l":
		if err := menu.ModeList(dirs, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		a := asker.New()
		if err := menu.ModeApp(a, dirs, target, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}

func installCommunity(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := communityFiles.ReadDir(embeddedCommunityDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := communityFiles.ReadFile(embeddedCommunityDir + "/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}
