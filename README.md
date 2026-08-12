# NTerm

NTerm is a native macOS block-based terminal written in Go. Commands are
composed in an editor and their command, streamed output, status, duration and
working directory stay together as selectable blocks.

## Current functionality

- independent tabs with persistent working directories and command history;
- native PTY execution, 24-bit colour and a bundled VT renderer for TUI apps;
- interactive input for prompts, password requests, REPLs and installers;
- deterministic history, command, option and path completion followed by an
  optional suggestion from a bundled 0.5B model;
- projects with local or remote paths and optional startup commands;
- native SSH transport with key, agent, password and keyboard-interactive
  authentication, host-key verification, keepalive, RTT and reconnect;
- remote path completion and Git context through a temporary helper removed at
  disconnect;
- SSH passwords and key passphrases stored in macOS Keychain; private keys stay
  in their original files;
- light, dark and system themes, configurable terminal font and size;
- YAML configuration at `~/Library/Application Support/NTerm/config.yml`;
- copying one block or a selection of commands and output.

NTerm currently targets macOS 26 or newer on Apple silicon. Windows ConPTY, Linux native
packaging and signed/notarized distribution are separate platform milestones.

## Development

Prerequisites: Go 1.25+ (the module selects the patched Go 1.26.5 toolchain) and
the macOS command-line developer tools. Node and npm
are not required because the frontend is dependency-free and committed as
static assets.

```sh
go test -race ./...
go run github.com/wailsapp/wails/v2/cmd/wails@v2.13.0 dev
```

Build the self-contained application:

```sh
./scripts/build-macos.sh
```

Run the full release gate with `./scripts/release-check.sh`. Distribution
builds can set `NTERM_CODESIGN_IDENTITY` to a Developer ID Application identity;
after configuring an `xcrun notarytool` Keychain profile, set
`NTERM_NOTARY_PROFILE` and run `./scripts/notarize-macos.sh`.

The build downloads pinned, checksum-verified AI assets when they are not
already present and packages them into `build/bin/NTerm.app`. End users do not
need Go, Node, Ollama, llama.cpp or a separately installed model.

Optional development overrides:

- `NTERM_AI_RUNTIME` — path to a development `llama-server` binary;
- `NTERM_AI_MODEL` — path to a development GGUF model.

Both values must be supplied together. The inference process is lazy,
loopback-only, shared between tabs and stopped after 90 seconds of inactivity.
Suggestions are inserted into the editor and never executed automatically.

## Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| `Enter` | Run the command |
| `Shift+Enter` | Insert a line |
| `Tab` / `→` | Accept a suggestion |
| `Option/Control+→` | Accept the next suggested word |
| `↑` / `↓` | Navigate suggestions or command history |
| `Control+C` | Send interrupt to the running process |
| `Control+L` | Clear blocks when no process is running |
| `Command+K` | Focus and select the composer |
| `Command+T` | Open a tab |
| `Command+W` | Close the active tab |
| `Command+1…9` | Switch tabs |
| `Command+,` | Open settings |

Architecture and security decisions are documented in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md); visual rules are in
[docs/STYLE_GUIDE.md](docs/STYLE_GUIDE.md). Third-party components and licenses
are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
