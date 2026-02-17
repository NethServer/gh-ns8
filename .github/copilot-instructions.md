# Copilot Instructions for gh-ns8

## Build & Test

```bash
go build -o gh-ns8          # Build binary
go vet ./...                 # Lint
go test ./...                # Run all tests
go test ./internal/module_release/ -run TestIsSemver  # Single test example
```

Test shell completion (internal):

```bash
./gh-ns8 __complete "module-release" ""              # List subcommands
./gh-ns8 __complete "module-release" "create" "--"   # List flags
```

Install locally as a `gh` extension for manual testing:

```bash
gh extension install .       # Install from working directory
gh ns8 module-release --help # Verify
```

Cross-compile (CI does this via `gh-extension-precompile`):

```bash
GOOS=linux GOARCH=amd64 go build -o gh-ns8-linux-amd64
```

## Architecture

This is a GitHub CLI extension (`gh ns8`) written in Go. It's designed as a top-level namespace — `module-release` is one subcommand group, with room for future sibling groups.

### Command tree

```
gh ns8                              → cmd/root.go
  └── module-release                → cmd/module_release/module_release.go
        ├── create                  → cmd/module_release/create.go
        ├── check                   → cmd/module_release/check.go
        ├── comment                 → cmd/module_release/comment.go
        └── clean                   → cmd/module_release/clean.go
```

### Package roles

- **`cmd/`** — Cobra command definitions. Each subcommand file owns its flags, calls into `internal/` for logic.
- **`internal/github/`** — GitHub API client wrapping `go-gh/v2`. Single `Client` struct with both REST and GraphQL. Used by all commands.
- **`internal/module_release/`** — Business logic for the module-release feature: repo validation, semver operations, PR/issue scanning, terminal display.

### How subcommands register

Subcommand packages register themselves via `init()` functions. `main.go` uses a blank import (`_ "github.com/NethServer/gh-ns8/cmd/module_release"`) to trigger registration. New command groups must follow this pattern.

### GitHub API strategy (hybrid)

The client uses **three** methods for GitHub interaction — choose based on the operation:

| Method | When to use | Example |
|---|---|---|
| `rest.Get()` / `rest.Post()` | Simple REST reads/writes | Repo info, commits, issues, PRs |
| `gh.Exec(...)` | Complex `gh` CLI operations | `gh release create --generate-notes`, `gh release delete --yes` |
| `gh.Exec("api", "graphql", ...)` | GraphQL with preview headers | Parent issue lookup (requires `GraphQL-Features: sub_issues` header) |

All three live in `internal/github/client.go`. The `go-gh` library's `api.DefaultRESTClient()` handles auth automatically from `gh`'s stored credentials.

## Key Conventions

- **Repositories must match `owner/ns8-*`** — The `ValidateRepository` function enforces the NethServer 8 naming convention. This is intentional, not a bug.
- **Shared flags** — `--repo` and `--issues-repo` are persistent flags on the `module-release` parent command. Subcommand-specific flags (e.g., `--testing`, `--draft`) are local to their command file.
- **`--issues-repo` defaults to `NethServer/dev`** — This is the centralized issue tracker for NethServer modules. Linked issues in PR bodies reference this repo.
- **Issue progress is label-driven** — The `check` command determines progress from GitHub labels: `verified` → ✅, `testing` → 🔨, neither → 🚧. These labels are filtered out of the displayed label list.
- **Testing version auto-generation** — When `--testing` is used without `--release-name`, the version bumps from the latest release: stable `1.0.0` → `1.0.1-testing.1`, testing `1.0.1-testing.1` → `1.0.1-testing.2`.
- **Error handling** — Command `RunE` functions return errors (not `os.Exit`). Cobra handles display. Non-fatal issues (e.g., a single issue API call failing) log warnings to stderr and continue.
- **Releases via `gh-extension-precompile`** — Push a `v*` tag to trigger cross-platform binary builds. Don't commit the `gh-ns8` binary.
