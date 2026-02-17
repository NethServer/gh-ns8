# `gh ns8` - GitHub CLI Extension for NethServer 8

A GitHub CLI (`gh`) extension for automating NethServer 8 module release management. Written in Go using the `cobra` CLI framework and `go-gh` library.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Shell Autocompletion](#shell-autocompletion)
- [Usage](#usage)
  - [Commands](#commands)
  - [Options](#options)
  - [Examples](#examples)
  - [Minimum PAT Permissions](#minimum-pat-permissions)
- [Testing Version Generation](#testing-version-generation)
- [Comment Generation](#comment-generation)
- [Check Command Documentation](#check-command-documentation)
- [Migration from Bash](#migration-from-bash)
- [Development](#development)
- [Prerequisites](#prerequisites)

## Features

- Validate Semantic Versioning (semver) format for release names
- Automatically generate the next testing release name
- Create releases with auto-generated release notes
- Include linked issues from PRs in release notes
- Check if a module is ready for release
- Comment on linked issues with release notifications
- Remove pre-releases between stable releases
- Parent/child issue hierarchy support via GitHub sub-issues API

## Installation

```bash
gh extension install NethServer/gh-ns8
```

## Shell Autocompletion

The extension includes built-in shell autocompletion support for Bash, Zsh, Fish, and PowerShell.

### Bash

**One-time setup** (add to `~/.bashrc` or `~/.bash_profile`):

```bash
# NethServer 8 extension completions
eval "$(gh ns8 completion bash)"
```

Or install system-wide:

```bash
# Linux
gh ns8 completion bash | sudo tee /etc/bash_completion.d/gh-ns8 > /dev/null

# macOS (with Homebrew)
gh ns8 completion bash > $(brew --prefix)/etc/bash_completion.d/gh-ns8
```

### Zsh

**One-time setup** (add to `~/.zshrc`):

```bash
# NethServer 8 extension completions
eval "$(gh ns8 completion zsh)"
```

Or install system-wide:

```bash
gh ns8 completion zsh > /usr/local/share/zsh/site-functions/_gh-ns8
```

Make sure you have the following in your `~/.zshrc`:

```bash
autoload -U compinit
compinit
```

### Fish

**One-time setup**:

```bash
gh ns8 completion fish > ~/.config/fish/completions/gh-ns8.fish
```

### PowerShell

**One-time setup** (add to your PowerShell profile):

```powershell
gh ns8 completion powershell | Out-String | Invoke-Expression
```

To find your PowerShell profile location:

```powershell
$PROFILE
```

### Testing Autocompletion

After installing, restart your shell or source your profile, then try:

```bash
gh ns8 module-release <TAB>         # Shows: create, check, comment, clean
gh ns8 module-release create --<TAB>  # Shows available flags
```

## Usage

```bash
gh ns8 module-release [create|check|comment|clean] [options]
```

### Commands

- `create`: Creates a new release
- `check`: Check the status of the `main` branch
- `comment`: Adds a comment to the release issues
- `clean`: Removes pre-releases between stable releases

### Options

#### Global Flags
- `--repo <repo-name>`: The GitHub repository (e.g., owner/ns8-module)
- `--issues-repo <repo-name>`: Issues repository (default: NethServer/dev)
- `--debug`: Enable debug mode

#### Create Command Flags
- `--release-refs <commit-sha>`: The commit SHA to associate with the release
- `--release-name <name>`: Specify the release name (must follow semver format)
- `--testing`: Create a testing release
- `--draft`: Create a draft release
- `--with-linked-issues`: Include linked issues from PRs in release notes

### Examples

Create a new release for the repository `NethServer/ns8-module`:

```bash
gh ns8 module-release create --repo NethServer/ns8-module --release-name 1.0.0
```

Create a new testing named release:

```bash
gh ns8 module-release create --repo NethServer/ns8-module --testing --release-name 1.0.0-testing.1
```

Create a new testing release with automatic release name generation:

```bash
gh ns8 module-release create --repo NethServer/ns8-module --testing
```

Create a new draft release:

```bash
gh ns8 module-release create --repo NethServer/ns8-module --release-name 1.0.0 --draft
```

Create a release with linked issues in the notes:

```bash
gh ns8 module-release create --repo NethServer/ns8-module --release-name 1.0.0 --with-linked-issues
```

Check the status of the `main` branch:

```bash
gh ns8 module-release check --repo NethServer/ns8-module
```

Add a comment to the release issues:

```bash
gh ns8 module-release comment --repo NethServer/ns8-module --release-name <release-name>
```

Remove pre-releases between stable releases:

```bash
gh ns8 module-release clean --repo NethServer/ns8-module --release-name <stable-release>
```

Remove pre-releases from latest stable release:

```bash
gh ns8 module-release clean --repo NethServer/ns8-module
```

### Minimum PAT Permissions

The following are the minimum Personal Access Token (PAT) permissions required for each command:

- **For operations on public repositories:**
  - `public_repo`

- **For operations on private repositories:**
  - `repo`

#### Command Permissions

- **`create`**:
  - Required Permissions:
    - `public_repo` (for public repositories) **or**
    - `repo` (for private repositories)

- **`check`**:
  - Required Permissions:
    - *(No additional permissions needed for public repositories)*
    - `repo` (for private repositories)

- **`comment`**:
  - Required Permissions:
    - `public_repo` (for public repositories) **or**
    - `repo` (for private repositories)

- **`clean`**:
  - Required Permissions:
    - `public_repo` (for public repositories) **or**
    - `repo` (for private repositories)

**Note:** For the `check` command on public repositories, no additional PAT permissions are required since it only performs read operations.

#### Using GitHub Actions Token

When using this extension within GitHub Actions workflows, you can utilize the
token provided by GitHub Actions. This token is available as `GITHUB_TOKEN` and
is automatically injected into workflows. It has sufficient permissions to
perform most operations required by this extension on the repository.

**Caution:** Be aware that using the default `GITHUB_TOKEN` provided by GitHub
Actions will not trigger downstream workflows, such as build and publication
processes of the module. This is due to security measures in place that prevent
accidental workflow triggers.

##### Proposed Solution

To allow workflows to trigger downstream events, you can use a Personal Access
Token (PAT) with the necessary permissions instead of the default
`GITHUB_TOKEN`. Store the PAT securely as a secret in your repository or
organization (e.g., `PAT_TOKEN`) and reference it in your workflow:

```yaml
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install gh extensions
        run: |
          gh extension install NethServer/gh-ns8
      - name: Create a testing release
        run: |
          gh ns8 module-release create --repo ${{ github.repository }} --testing
        env:
          GITHUB_TOKEN: ${{ secrets.PAT_TOKEN }}
```

**Important:** Ensure that your PAT is stored securely and has only the minimum
required permissions as specified above.

## Testing Version Generation

When creating testing releases without specifying a name (using `--testing` without `--release-name`), the version is automatically generated following these rules:

1. If the latest release is a stable release (no pre-release suffix):
   - Increments the patch version by 1
   - Adds `-testing.1` suffix
   - Example: `1.0.0` → `1.0.1-testing.1`

2. If the latest release is already a testing release:
   - Keeps the same version numbers
   - Increments only the testing number
   - Example: `1.0.1-testing.1` → `1.0.1-testing.2`

## Comment Generation

When using the `comment` command, the extension will:

1. Find all PRs merged between the current release and the previous one
2. Extract linked issues from the PR descriptions (looking for references like `NethServer/dev#1234` or `https://github.com/NethServer/dev/issues/1234`)
3. For each linked issue that is still open:
   - If the release is a pre-release (testing), add a comment:
     ```
     Testing release `owner/ns8-module` [1.0.0-testing.1](link-to-release)
     ```
   - If the release is stable, add a comment:
     ```
     Release `owner/ns8-module` [1.0.0](link-to-release)
     ```
4. Also comment on parent issues if the issue has a parent (via GitHub sub-issues API)

The comment command can be used with or without specifying a release name. If no release name is provided, it will use the latest release.

## Check Command Documentation

### Purpose

The `check` command is used to verify the status of the `main` branch and check
for pull requests (PRs) and issues since the latest release. It helps ensure
that the repository is ready for a new release by providing a summary of PRs
and issues.

### Usage

```bash
gh ns8 module-release check --repo <repo-name>
```

### Output Format

The `check` command outputs a summary of:

- PRs without linked issues
- Translation PRs
- Commits outside PRs (orphan commits)
- Issues with their status and progress

### Emojis Used

The `check` command uses emojis to indicate the status and progress of issues:

- **Issue status:**
  - 🟢 Open
  - 🟣 Closed

- **Progress status:**
  - 🚧 In Progress
  - 🔨 Testing
  - ✅ Verified

### Examples

Here are some examples of the `check` command output:

#### Example: Complete Check Output

```
Summary:
--------
PRs without linked issues:
https://github.com/NethServer/ns8-module/pull/123
https://github.com/NethServer/ns8-module/pull/456

Translation PRs:
https://github.com/NethServer/ns8-module/pull/789

Commits outside PRs:
https://github.com/NethServer/ns8-module/commit/abc123

Issues:
🟢 🚧 https://github.com/NethServer/dev/issues/101 (2) bug
🟣 ✅ https://github.com/NethServer/dev/issues/102 (1) enhancement
└─ 🟢 🔨 https://github.com/NethServer/dev/issues/103 (1) documentation

---
Issue status:    🟢 Open    🟣 Closed
Progress status: 🚧 In Progress    🔨 Testing    ✅ Verified
```

## Migration from Bash

This is the Go rewrite of the original `gh-ns8-release-module` bash extension. Key differences:

### New CLI Format

**Old:** `gh ns8-release-module create --repo owner/ns8-module --testing`  
**New:** `gh ns8 module-release create --repo owner/ns8-module --testing`

The new format prepares the extension for additional subcommands beyond `module-release`.

### Technical Improvements

- **Type Safety**: Go's strong typing prevents many runtime errors
- **Better Error Handling**: Structured error propagation and context
- **Cross-Platform**: Compiled binaries work on Windows, macOS, Linux
- **Performance**: Faster execution, especially for operations with many API calls
- **Maintainability**: Modular package structure with clear separation of concerns
- **Testing**: Built-in support for unit and integration testing

### Architecture

```
cmd/
  ├── root.go                    # Root "ns8" command
  └── module_release/            # Module-release subcommand
      ├── module_release.go      # Parent command
      ├── create.go              # Create subcommand
      ├── check.go               # Check subcommand
      ├── comment.go             # Comment subcommand
      └── clean.go               # Clean subcommand
internal/
  ├── github/
  │   └── client.go              # GitHub API client (REST + GraphQL)
  └── module_release/
      ├── repo.go                # Repository validation
      ├── semver.go              # Semver logic
      └── display.go             # Terminal output
```

## Development

### Building

```bash
go build -o gh-ns8
```

### Testing Locally

```bash
gh extension install .
gh ns8 module-release --help
```

### Releasing

The project uses the `gh-extension-precompile` GitHub Action for automated cross-platform releases:

1. Create and push a tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. The GitHub Action will automatically:
   - Build binaries for multiple platforms
   - Attach them to the GitHub release
   - Name them according to `gh` extension conventions

## Prerequisites

- [GitHub CLI](https://cli.github.com/): `gh` (v2.0.0 or later)
- Go 1.21+ (for development only)

## Troubleshooting

If you encounter any issues while using the `gh ns8` extension, consider the following troubleshooting steps:

1. Ensure you have the latest version of the GitHub CLI (`gh`) installed
2. Verify that you have the correct permissions to access the repository
3. Check for any error messages and refer to the GitHub CLI documentation for more information
4. Enable debug mode with `--debug` flag for detailed output
5. If the issue persists, consider opening an issue on the [GitHub repository](https://github.com/NethServer/gh-ns8/issues)

## Updating and Uninstalling

### Updating

To update the `gh ns8` extension to the latest version, run:

```bash
gh extension upgrade NethServer/gh-ns8
```

### Uninstalling

To uninstall the `gh ns8` extension, run:

```bash
gh extension remove NethServer/gh-ns8
```

## License

See the LICENSE file for details.
