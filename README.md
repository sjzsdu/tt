# tt

`tt` is a collection-style CLI toolkit for local task workflows. It provides small, focused commands for browsing/editing structured files, rendering markdown, inspecting configuration, running embedded Picoclaw agent workflows, and converting CLI commands to skill files.

## Features

- Local web UI for Markdown files with live reload.
- Local web UI for JSON files with formatted preview and editing.
- Local web UI for conversation-style JSON transcripts.
- Local web UI for skill Markdown files with frontmatter support.
- Embedded Picoclaw agent runtime command.
- Structured multi-agent debate command.
- CLI command to skill file conversion.
- Directory mirroring for config sharing.
- Global and project-level JSON configuration.

## Installation

### Build locally

```bash
git clone <repo-url>
cd tt
make build
./tt --help
```

### Install to user bin

By default this installs to `~/.local/bin/tt`:

```bash
make install
```

You can override the prefix:

```bash
make install PREFIX=/usr/local
```

### Run without installing

```bash
go run . --help
```

> Note: this repository currently uses a local Go module replacement for `github.com/sipeed/picoclaw`:
>
> ```go
> replace github.com/sipeed/picoclaw => ../picoclaw
> ```
>
> Make sure the sibling `../picoclaw` checkout exists. The CI workflow checks out <https://github.com/sjzsdu/picoclaw> at that sibling path so the local replacement resolves consistently.

## Usage

```bash
tt [command]
```

Available commands:

| Command | Description |
| --- | --- |
| `agent` | Run the embedded Picoclaw agent runtime. |
| `cmd2skill` | Convert CLI commands into skill files. |
| `config` | Inspect and initialize `tt` configuration. |
| `conversation` | Browse conversation-like JSON in a local web UI. |
| `debate` | Run a structured multi-agent debate on a topic. |
| `json` | Browse and edit JSON files in a local web UI. |
| `markdown` | Browse Markdown files in a local web UI. |
| `mirror` | Mirror selected files from a source directory. |
| `skill` | Browse and edit skill Markdown files. |
| `version` | Print the `tt` version. |

Use built-in help for full flag details:

```bash
tt --help
tt <command> --help
```

## Commands

### `tt markdown`

Start a local web service for browsing Markdown files in the current working tree. The UI is a React/Vite single-page app embedded into the Go binary at build time, while Go owns the local file APIs and websocket reloads.

```bash
tt markdown
tt markdown README.md
tt markdown docs --port 9595
tt markdown --pattern "**/*.md"
tt markdown --content "# Hello" --content-only
```

For development, run `make web-build` to rebuild embedded web assets or `cd web && npm run dev:markdown` for the Vite dev server.

Flags:

- `-p, --port int`: service port, default `9595`.
- `-c, --content string`: render provided Markdown content directly.
- `--content-only`: only show provided Markdown content.
- `-f, --pattern strings`: filter Markdown files by glob patterns.

### `tt json`

Start a local web service for browsing and editing JSON files.

```bash
tt json
tt json data/config.json
tt json ~/projects/sample-data
tt json --file data/config.json --port 9696
```

Flags:

- `-p, --port int`: service port, default `9696`.
- `-f, --file string`: open a specific JSON file.

### `tt conversation`

Start a local web service for browsing JSON files that contain conversation-style message flows.

```bash
tt conversation
tt conversation logs/*.json
tt conversation --file session.json
tt conversation --pattern "**/*.json" --port 9680
```

Flags:

- `-p, --port int`: service port, default `9680`.
- `-f, --file string`: open a specific JSON file.
- `--pattern strings`: filter JSON files by glob patterns.

### `tt skill`

Start a local web UI for skill files. It extracts frontmatter, renders the remaining Markdown body, and supports editing plus saving the full document.

```bash
tt skill
tt skill create-cmd
tt skill --file .forge/skills/create-cmd/SKILL.md --edit
tt skill --root ~/.config/skills
```

Flags:

- `-p, --port int`: service port, default `9695`.
- `-f, --file string`: open a specific skill Markdown file.
- `--root string`: override the skill root directory.
- `--edit`: open the current document in edit mode by default.

### `tt agent`

Run the embedded Picoclaw agent runtime and reuse existing Picoclaw configuration, models, sessions, and skills without invoking the `picoclaw` binary.

```bash
tt agent -m "summarize this project"
tt agent "explain the current directory"
tt agent --session cli:tt --model gpt-5.4 -m "review this idea"
tt agent --picoclaw-home ~/.picoclaw-dev -m "list available skills"
```

Alias:

```bash
tt pc ...
```

Flags:

- `-m, --message string`: send a single message to the agent.
- `-s, --session string`: session key, default `cli:default` unless configured.
- `--agent string`: agent id or name to use.
- `--model string`: model override.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

#### `tt agent info`

Show resolved agent runtime information as JSON:

```bash
tt agent info
tt agent info --picoclaw-home ~/.picoclaw-dev
```

### `tt debate`

Run two debater agents and one judge agent through multiple rounds, then render the transcript as text or JSON.

```bash
tt debate "Remote work improves team productivity"
tt debate --topic "AI should replace code review" --agents alpha,beta --judge referee
tt debate --topic "AI should replace code review" --agents alpha --agents beta --judge referee
tt debate "Should startups stay fully remote" --rounds 4 --output json --session cli:debate
tt debate "AI should replace code review" --output json --out debates/review.json
```

Flags:

- `-t, --topic string`: debate topic. Positional args are also supported.
- `--agents strings`: two debater agent ids or names.
- `--judge string`: agent id or name for the judge.
- `-r, --rounds int`: maximum number of debate rounds, default `3`.
- `-o, --output string`: output format, `text` or `json`, default `text`.
- `--out string`: write debate result to a file. JSON output auto-saves to `./debates` when omitted.
- `-s, --session string`: session key prefix, default `cli:debate`.
- `--model string`: model override for all participants.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

### `tt cmd2skill`

Parse a CLI command and its subcommands into a structured command tree, then generate agent-oriented skill files with usage, command references, safety guidance, and deterministic output.

```bash
tt cmd2skill git
tt cmd2skill git --depth 2
tt cmd2skill docker --examples
tt cmd2skill kubectl --target-dir ./.forge/skills
tt cmd2skill git --markdown
```

Flags:

- `--target-dir string`: directory to write skill files, default `~/.agents/skills`.
- `--dry-run`: print skill content to stdout instead of writing files.
- `--examples`: include examples extracted from help output.
- `-d, --depth int`: recursion depth for subcommand help (0 = top-level only), default `1`.
- `--timeout duration`: timeout for each help command, default `5s`.
- `--max-commands int`: maximum number of command help pages to discover, default `200`.
- `--markdown`: open generated skill content directly with markdown command instead of writing files.

### `tt mirror`

Mirror keeps a project-local directory in sync with a fuller source directory. It is useful for sharing and selectively importing tool configs such as opencode agents and commands.

```bash
tt mirror source
tt mirror target
tt mirror apply
tt mirror apply agents.foo commands.bar
tt mirror prune
tt mirror config
tt mirror config --set-source-dir ~/.config/opencode-full
```

Subcommands:

| Subcommand | Description |
| --- | --- |
| `source` | Show source directory tree. |
| `target` | Show target directory tree. |
| `apply` | Mirror all or selected keys from source to target. |
| `prune` | Remove mirrored target entries. |
| `config` | Show or set mirror paths in project tt config. |

Flags:

- `--source-dir string`: source directory path (default `~/.config/opencode-full`).
- `--target-dir string`: target directory path (default `.opencode`).
- `--config-file string`: config file name (default `opencode.json`).

`tt mirror source` / `tt mirror target`:
- `-l, --level int`: show items up to this depth level, 0 for all.

`tt mirror config`:
- `--set-source-dir string`: set source directory path.
- `--set-target-dir string`: set target directory path.
- `--set-config-file string`: set config file name.

### `tt config`

Inspect resolved configuration paths, initialize config files, or show merged configuration.

```bash
tt config
tt config --show
tt config --init-global
tt config --init-project
```

Flags:

- `--show`: show merged `tt` configuration.
- `--init-global`: create `~/.tt/config.json` if missing.
- `--init-project`: create project `.tt/config.json` if missing.

### `tt version`

```bash
tt version
```

## Configuration

`tt` loads configuration from two levels:

1. Global config: `~/.tt/config.json`, or `TT_CONFIG` when set.
2. Project config: nearest project `.tt/config.json`, or `TT_PROJECT_CONFIG` when set.

Project configuration overrides global configuration.

Example:

```json
{
  "picoclaw": {
    "home": "~/.picoclaw",
    "config": "~/.picoclaw/config.json"
  },
  "agent": {
    "session": "cli:default",
    "agent": "default",
    "model": "gpt-5.4",
    "debug": false
  },
  "debate": {
    "agents": ["alpha", "beta"],
    "judge": "referee",
    "rounds": 3,
    "output": "text"
  },
  "markdown": {
    "port": 9595,
    "content_only": false,
    "patterns": ["**/*.md"]
  },
  "conversation": {
    "port": 9680,
    "patterns": ["**/*.json"]
  },
  "mirror": {
    "source_dir": "~/.config/opencode-full",
    "target_dir": ".opencode",
    "config_file": "opencode.json"
  }
}
```

Initialize a starter config:

```bash
tt config --init-global
tt config --init-project
```

## Releases

When `cmd/version.go` changes on `main`, GitHub Actions builds Linux, macOS, and Windows binaries for `amd64` and `arm64`, then publishes them to a GitHub Release tagged with the version, for example `v0.1.0`.

## Development

Common targets:

```bash
make build          # Build ./tt
make install        # Install to $(PREFIX)/bin/tt, default ~/.local/bin/tt
make install-system # Install to /usr/local/bin/tt
make clean          # Remove local binary
make run            # Run with go run
make fmt            # Format Go files
make tidy           # Tidy Go modules
```

Run checks:

```bash
go test ./...
```

## Project layout

```text
.
├── cmd/                 # Cobra commands
├── internal/dirmirror/  # Directory mirror utilities
├── internal/mdutil/     # Markdown utilities
├── internal/picoclaw/   # Embedded Picoclaw integration
├── internal/ttconfig/   # tt config loading and merging
├── internal/webui/      # Web UI templates
├── web/                 # Web frontend assets
├── main.go              # CLI entrypoint
├── Makefile
├── go.mod
└── go.sum
```

## License

No license file is currently included in this repository.