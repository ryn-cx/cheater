# cheater

`cheater` is like [`cheat`](https://github.com/cheat/cheat) but it can also
build and run the commands for you.

## Install

```sh
git clone https://github.com/ryn-cx/cheater
cd cheater
go install .
```

## Use

```sh
cheater 7z          # open the interactive menu for 7z
cheater --list      # show every stored app + cheat count
cheater --import https://github.com/user/cheats   # add a repo's cheats to personal
cheater --help      # usage
```

Inside the per-app menu (arrow keys + Enter, powered by
[`survey`](https://github.com/AlecAivazis/survey)):

```
'7z' — pick a cheat or action
   personal:
>     1. Extract an archive  —  $ 7z x <archive>
   community:
      2. Create a max-compression archive  —  $ 7z a -mx=9 <archive> <files>
    +  Add a new cheat
    ~  Edit a personal cheat
    -  Remove a personal cheat
    q  Quit
```

Cheats are grouped by source. **personal** cheats are your own;
**community** cheats are read-only — Edit/Remove only act on personal.

## Placeholders

Recipes use the same `<name>` syntax as most `cheat` files:

```
7z x <archive>
ffmpeg -i <input> -c:v <codec> <output>
```

When you add a cheat, you can optionally attach a **type** and **description**
to each placeholder. Types are validated at run time:

| Type | Meaning |
|---|---|
| `existing_file` | path must exist and be a regular file |
| `existing_dir` | path must exist and be a directory |
| `existing_path` | path must exist (file or dir) |
| `path` | a syntactically valid path on the current OS; doesn't need to exist yet |
| `integer` | parses as a signed integer |
| `string` | any text; **shell-quoted** on substitution |
| `existing_file_list` | repeatedly prompts for files; each validated and quoted |

## Storage

One JSON file per app, split into `personal/` and `community/` under the
platform's user-config directory (via Go's `os.UserConfigDir()`):

- **Linux:** `$XDG_CONFIG_HOME/cheater/cheatsheets/{personal,community}/<app>.json` (default `~/.config/cheater/...`)
- **macOS:** `~/Library/Application Support/cheater/cheatsheets/{personal,community}/<app>.json`
- **Windows:** `%APPDATA%\cheater\cheatsheets\{personal,community}\<app>.json`

```json
[
  {
    "description": "Extract an archive",
    "command": "7z x <archive>",
    "params": {
      "archive": { "type": "existing_file", "description": "The archive to open" }
    }
  }
]
```

The `defaults` and `params` keys are optional. Empty arrays delete the file on
save so `cheater --list` doesn't show phantom apps.

## Development

```sh
go build ./...      # compile
go test ./...       # run all tests (store, placeholder, menu)
```

Layout:

- [`main.go`](main.go) — entry point
- [`store/`](store/) — JSON load/save, ParamSpec/Cheat types, path resolution
- [`placeholder/`](placeholder/) — `<name>` extraction & substitution
- [`asker/`](asker/) — prompt interface + `survey` implementation
- [`menu/`](menu/) — interactive menu, list view, shell quoting
