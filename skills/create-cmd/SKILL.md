---
name: create-cmd
description: Create a new tt cobra subcommand by following the conventions already used under cmd/. Use when you need to add a new root subcommand or a nested subcommand, register flags, wire Run/RunE handlers, and keep naming, init registration, config loading, and output/error handling consistent with the existing tt CLI structure.
---

# Create tt cmd

Create new commands under `cmd/` by matching the current `tt` CLI layout.

## Workflow

1. Inspect whether the new command belongs directly under `rootCmd` or under an existing parent command such as `agentCmd`.
2. Add or update exactly the files needed in `cmd/`.
3. Keep all code in package `cmd`.
4. Prefer file names that match the command name, such as `foo.go` for `tt foo` and `agent_bar.go` or `bar.go` for nested commands when that keeps the directory flat and readable.
5. After editing, run formatting and a build check.

## Required conventions

### 1. Command declaration

Declare commands as package-level variables using `&cobra.Command{...}`.

Patterns in the codebase:

- Root command uses `var rootCmd = &cobra.Command{...}`.
- Top-level commands use names like `versionCmd`, `configCmd`, `agentCmd`, `conversationCmd`, `markdownCmd`.
- Nested commands use names like `agentInfoCmd`.

Use the same `somethingCmd` naming pattern.

### 2. Registration in init

Register commands inside `func init()` in the same file as the command definition.

- Top-level command: `rootCmd.AddCommand(yourCmd)`
- Nested command: `parentCmd.AddCommand(yourCmd)`

Register flags in that same `init()` block immediately after `AddCommand(...)`.

### 3. Use/Short/Long/Example fields

Follow the existing Cobra metadata style:

- `Use` is short and CLI-shaped, for example `"version"`, `"config"`, `"agent [message]"`, `"conversation [files...]"`.
- `Short` is a single concise sentence.
- Add `Long` only when extra behavior or context helps.
- Add `Example` for commands with multiple invocation modes or non-obvious usage.
- Add `Aliases` only when the alias is already part of product behavior.
- Add `Args` only when argument validation matters.

### 4. Run vs RunE

- Use `RunE` when the handler can fail.
- Use `Run` only for trivial no-error actions such as printing a version string.
- Prefer extracting non-trivial logic into a helper like `runAgent(...)` or `runConversationServer()` instead of placing everything inline.

### 5. Flags and package-level state

Define flag-backed variables at package scope in a `var (...)` block near the command.

Examples from the codebase include booleans, strings, slices, and ports.

Use Cobra flag helpers in `init()`, for example:

- `StringVar` / `StringVarP`
- `BoolVar` / `BoolVarP`
- `IntVarP`
- `StringSliceVar`

Only use short flags where they improve common usage and do not create ambiguity.

### 6. Output and error handling

- Return errors instead of calling `os.Exit` in subcommands.
- Prefer `fmt.Fprintln(cmd.OutOrStdout(), ...)` for command output when practical.
- For stderr warnings outside the command writer pattern, follow the existing style only when necessary.
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Keep `main.go` as the only place that exits the process.

### 7. Config-aware commands

If the command depends on tt config:

1. Call `loadTTConfig()` early.
2. Use the returned merged config or sources.
3. Use `projectRootFromConfig(...)` when project-root resolution matters.
4. Do not duplicate config loading helpers; reuse existing ones from `cmd/config_helpers.go`.

If the command does not need config, do not load it just for consistency.

### 8. File organization

Prefer one command-focused file per command or subcommand cluster.

Recommended pattern:

- Main command and its directly shared flags/helpers in one file.
- Shared helper functions in the same file when they are command-specific.
- Cross-command helpers belong in a dedicated helper file only if reused.

Keep package `cmd` flat unless the project structure changes globally.

### 9. Style expectations inferred from the repo

- Use tabs and normal `gofmt` layout.
- Keep imports grouped by standard library, blank line, third-party/internal packages.
- Keep command definitions near the top of the file.
- Put helper functions below `init()` unless a type definition needs to appear earlier.
- Keep handler names explicit, such as `runAgent`, `runMarkdownServer`, `initTTConfigFile`.

## Implementation template

Use this as the default pattern for a new top-level command:

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	fooOption string
	fooVerbose bool
)

var fooCmd = &cobra.Command{
	Use:   "foo",
	Short: "Describe the command briefly",
	Long:  "Add Long only if it provides meaningful extra context.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFoo(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(fooCmd)
	fooCmd.Flags().StringVar(&fooOption, "option", "", "option description")
	fooCmd.Flags().BoolVarP(&fooVerbose, "verbose", "v", false, "enable verbose output")
}

func runFoo(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "todo")
	return nil
}
```

For a nested command, replace `rootCmd.AddCommand(fooCmd)` with `parentCmd.AddCommand(fooCmd)`.

## Validation checklist

After creating or editing a command:

1. Run `gofmt` on touched Go files.
2. Run `go build ./...`.
3. If the command has user-visible output paths, verify the `Use` text and flags read naturally.
4. Confirm the command is registered under the intended parent.

## Repo-specific references

Use these existing files as the source of truth when in doubt:

- Root command shape: `cmd/root.go`
- Simple command example: `cmd/version.go`
- Flag-heavy command example: `cmd/config.go`
- Complex command with helper extraction: `cmd/agent.go`
- Nested subcommand example: `cmd/agent_info.go`
- Config helpers: `cmd/config_helpers.go`

## Notes

- Do not modify `main.go` when adding a normal command; command discovery already flows through `cmd.Execute()`.
- Do not create a new command generator framework unless explicitly requested; follow the existing hand-written style.
- When this skill has been created in the current session, tell the user to start a new session before expecting the skill to auto-load.
