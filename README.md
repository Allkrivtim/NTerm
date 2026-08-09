# WARNING! ALPHA VERSION WAS BUILD WITH AI. USE AT YOUR OWN RISK.
## NTerm

<<<<<<< HEAD
Block based terminal. Liquid Glass style.
=======

NTerm is a Go desktop terminal prototype built around command blocks. Write a
command in a real multiline editor, execute it, and keep the command, output,
status and working directory together as one immutable block. Independent tabs
provide separate local sessions without adding visual noise.

## Prototype scope

Included now:

- native desktop window powered by Wails and the operating system WebView;
- streamed command output grouped into blocks;
- native PTY execution and a built-in VT renderer for `micro`, `nano`, `vim`,
  pagers, process monitors and other alternate-screen applications;
- persistent working directory for ordinary `cd` usage;
- multiline command editor (`Shift+Enter` inserts a line, `Enter` runs it);
- history, executable, completion-spec and filesystem suggestions;
- cancellable AI completion through a bundled llama.cpp runtime;
- independent local tabs with their own cwd, history and running process;
- managed SSH sessions with persistent ControlMaster connections, port
  forwarding and remote-aware paths, commands and Git context;
- a temporary zero-configuration server helper that is removed on disconnect;
- one-click copying of a complete command block (command and output);
- system, light and dark themes;
- configurable terminal font, size, shell and new-tab directory;
- optional reduced transparency and command metadata;
- cancellation with `Ctrl+C` and clearing with `Ctrl+L`.

Not in this milestone: the SSH host library, credentials and key storage. Their
boundaries and rollout are documented in [docs/ROADMAP.md](docs/ROADMAP.md).

Preferences are stored in the operating system configuration directory as
`NTerm/settings.json`. The default directory and shell apply to newly created
tabs; appearance settings apply immediately.

## Run

Prerequisites: Go 1.25+, Node 15+ and Wails 2.13+.

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
wails dev
```

Build a self-contained production application:

```sh
./scripts/build-macos.sh
```

The frontend has no npm runtime dependencies. It is plain HTML, CSS and
JavaScript embedded into the Go binary. The macOS build hooks also package the
inference runtime and quantized model into `NTerm.app`; end users do not install
Ollama, llama.cpp, Node, Go or any model separately.

### Local AI completion

Deterministic suggestions appear immediately and never wait for a model. After
a short idle pause, NTerm asks its built-in Qwen coder model for one contextual
completion using the cwd and six recent commands. The 0.5B Q4_K_M model uses a
1024-token context, generates at most 64 tokens and unloads after 90 seconds of
inactivity. The runtime starts lazily, binds only to a random loopback port and
is terminated with NTerm.

Optional developer configuration:

- `NTERM_AI_RUNTIME` — path to a development `llama-server` binary;
- `NTERM_AI_MODEL` — path to a development GGUF model.

Both overrides must be set together. AI failures are silent and never delay or
disable local suggestions. Suggestions are inserted into the editor and are
never executed automatically. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
for bundled component licenses.

### SSH sessions

Run an ordinary OpenSSH command such as `ssh user@host`. NTerm keeps that
connection alive for the tab and runs subsequent blocks through multiplexed
channels. OpenSSH options for identity files, jump hosts, config profiles and
`-L`/`-R`/`-D` forwarding are preserved. Type `exit` or close the tab to return
to the local session.

NTerm copies a small POSIX helper to a private remote temporary directory. It
provides remote path completion, command discovery and Git status, starts no
daemon, changes no shell configuration and is deleted at disconnect. The
current vertical slice uses the user's existing OpenSSH config, agent and keys;
password/key storage UI comes with the host vault. Local and remote full-screen
programs use the bundled VT renderer and return to the block list when they exit.

## Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| `Enter` | Run the command |
| `Shift+Enter` | Insert a new line |
| `Tab` / `→` | Accept the selected suggestion |
| `Ctrl/Option+→` | Accept the next suggested word |
| `↑` / `↓` | Navigate suggestions or command history |
| `Ctrl+C` | Cancel the running block |
| `Ctrl+L` | Clear visible blocks |
| `Cmd/Ctrl+K` | Focus and select the command editor |
| `Cmd/Ctrl+T` | Open a new tab |
| `Cmd/Ctrl+W` | Close the active tab |
| `Cmd/Ctrl+1…9` | Switch tabs |
| `Cmd/Ctrl+,` | Open settings |

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for decisions, package
boundaries, event flow and the production SSH/security design.

The persistent visual rules live in [docs/STYLE_GUIDE.md](docs/STYLE_GUIDE.md).
>>>>>>> aa5b643 (Fixes)
