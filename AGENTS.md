# AGENTS.md

Instructions for AI coding agents working on this repository.

## Build & Test

```bash
go build -o gh-ns8          # Build binary
go vet ./...                 # Lint
go test ./...                # Run all tests
go test ./internal/module_release/ -run TestIsSemver  # Single test example
```

Test shell completion:

```bash
./gh-ns8 __complete "module-release" ""              # List subcommands
./gh-ns8 __complete "module-release" "create" "--"   # List flags
```

Install locally as a `gh` extension for manual testing:

```bash
gh extension install .       # Install from working directory
gh ns8 module-release --help # Verify
```

Release builds are handled by CI (`gh-extension-precompile`) on `v*` tag
push. Do not commit the `gh-ns8` binary.

## Architecture

GitHub CLI extension (`gh ns8`) written in Go. Top-level namespace with
`module-release` as the first subcommand group.

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

| Package | Responsibility |
|---|---|
| `cmd/` | Cobra command definitions, flags, delegates to `internal/` |
| `internal/github/` | GitHub API client (`go-gh/v2`), REST + GraphQL + `gh` exec |
| `internal/module_release/` | Business logic: repo validation, semver, PR/issue scan, display |

### Subcommand registration

Subcommand packages register via `init()` functions. `main.go` uses a
blank import (`_ "github.com/NethServer/gh-ns8/cmd/module_release"`) to
trigger registration. New command groups must follow this pattern.

### GitHub API strategy

| Method | When to use |
|---|---|
| `rest.Get()` / `rest.Post()` | Simple REST reads/writes (repos, commits, issues, PRs) |
| `gh.Exec(...)` | Complex `gh` CLI operations (release create/delete) |
| `gh.Exec("api", "graphql", ...)` | GraphQL with preview headers (sub-issues) |

All three live in `internal/github/client.go`. Auth is automatic from
`gh` stored credentials.

## Key Conventions

- **Repository naming**: Must match `owner/ns8-*`
  (`ValidateRepository` enforces this). This is intentional.
- **Shared flags**: `--repo` and `--issues-repo` are persistent on the
  `module-release` parent command. Subcommand-specific flags are local.
- **`--issues-repo`** defaults to `NethServer/dev` (centralized tracker).
- **Issue labels drive progress**: `verified` → ✅, `testing` → 🔨,
  neither → 🚧. These labels are filtered out of display.
- **Testing version auto-generation**: `1.0.0` → `1.0.1-testing.1`,
  then `1.0.1-testing.2`.
- **Error handling**: `RunE` returns errors (no `os.Exit`). Non-fatal
  issues log warnings to stderr and continue.

## Git Commit Style

Follow [Conventional Commits v1.0.0](https://www.conventionalcommits.org/en/v1.0.0/).

- **50/72 rule**: Subject ≤50 chars, body lines ≤72 chars
- **Tone**: Polite imperative, impersonal, English
- **Subject**: Lowercase imperative verb, no period
- **Body**: Explain *what* and *why*, not *how*
- **Types**: `feat`, `fix`, `refactor`, `perf`, `docs`, `test`, `chore`

```
feat: add comment url to module-release output

When creating issue comments via the comment subcommand,
print the direct URL to the created comment for user
convenience. Fetch comment details from GitHub API after
creation to retrieve the html_url.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```
