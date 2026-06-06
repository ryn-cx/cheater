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

  To import cheatsheets from a git repository into your personal cheats:
    cheater -i https://github.com/user/cheats

Flags:
  -h, --help            help for cheater
  -l, --list            List cheatsheets
  -i, --import <repo>   Clone a git repository and add its cheatsheets to your
                        personal cheats (existing files are kept on conflict)
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

	args := os.Args[1:]

	switch {
	case len(args) == 0, args[0] == "--help", args[0] == "-h":
		fmt.Print(usageHelp)
	case args[0] == "--list", args[0] == "-l":
		if len(args) > 1 {
			fmt.Fprintf(os.Stderr, "accepts at most 1 arg(s), received %d\n", len(args))
			os.Exit(1)
		}
		if err := menu.ModeList(dirs, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case args[0] == "--import", args[0] == "-i":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "error: --import requires a single git repository URL")
			os.Exit(1)
		}
		imported, skipped, err := sheets.ImportRepo(args[1], dirs.Personal, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("imported %d cheatsheet(s), skipped %d existing\n", imported, skipped)
	default:
		if len(args) > 1 {
			fmt.Fprintf(os.Stderr, "accepts at most 1 arg(s), received %d\n", len(args))
			os.Exit(1)
		}
		a := asker.New()
		if err := menu.ModeApp(a, dirs, args[0], os.Stdout); err != nil {
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
