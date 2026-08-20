# Delivery plan

## Phase 0 — decisions and risks

1. Fix the product vocabulary: workspace → tab → session → command block.
2. Separate terminal execution, suggestion providers, persistence, security and
   UI so none of them depends on Wails directly.
3. Treat PTY/VT support and secrets as security/reliability work, not UI polish.
4. Keep all AI providers opt-in, local-first and cancellable.

Exit criterion: architectural decisions are recorded and the MVP boundary is
testable.

## Phase 1 — block terminal prototype (complete)

1. Native shell window with automatic system light/dark appearance.
2. One session and one working directory.
3. A multiline editor with selection, clipboard, undo/redo and keyboard
   shortcuts.
4. Stream stdout/stderr into a command block; show exit status and duration.
5. Complete from history, executables and filesystem paths.
6. Add a small, cancellable bundled model after deterministic suggestions.

Exit criterion: ordinary shell commands can be written, suggested, run,
cancelled and reviewed without losing context.

## Phase 1.5 — tabs, settings and visual system (complete)

1. Multiple local tabs with independent sessions, cwd, drafts and history.
2. System, light and dark themes in a quiet neutral Liquid Glass visual system.
3. Configurable terminal font, size, default new-tab directory and shell.
4. Reduced-transparency and command-metadata preferences.
5. A standalone macOS application icon that is never reused inside the UI.

Exit criterion: tabs remain independent, preferences survive restart and the
interface retains the same minimal visual language in both themes.

## Phase 2 — real terminal semantics (Unix vertical slice complete)

1. Introduce a platform PTY adapter (Unix PTY complete; Windows ConPTY pending).
2. Add a VT parser and a cell-grid renderer for interactive screen regions (complete).
3. Define block boundaries using shell integration (OSC 133/633), rather than
   guessing from process completion.
4. Support resize, signals, bracketed paste, alternate screen and raw mode.
5. Add scrollback limits, backpressure and large-output virtualization.

Exit criterion: `vim`, `htop`, REPLs, password prompts and long-running
processes work correctly inside a block.

## Phase 2.5 — managed SSH vertical slice (complete)

1. Detect an interactive SSH invocation and retain one native encrypted transport per tab.
2. Authenticate with the configured key, agent or credential-vault password.
3. Execute blocks through multiplexed SSH channels with remote cwd persistence.
4. Detect dead and half-open links, report RTT and reconnect with bounded backoff.
5. Install a connection-scoped helper for remote paths, commands and Git data.
6. Remove the helper on `exit`, tab closure and app shutdown.

Exit criterion: key/agent-authenticated servers have the normal NTerm block,
completion and Git experience without a permanent server install.

## Phase 3 — advanced sessions

1. Restore local tab layout, drafts and bounded block history after restart
   without resurrecting processes or secrets (complete); window geometry remains.
2. Search, copy and rerun blocks (complete); pinning, command palettes and split panes remain.
3. Crash isolation and per-session resource limits.
4. Opt-in tmux control-mode integration for reattaching an interrupted remote
   TUI or long-running process; keep the current reconnect-only fallback when
   tmux is unavailable.

Exit criterion: a failed or noisy session cannot freeze other tabs.

## Phase 4 — SSH host vault

1. Host profiles and OpenSSH export are complete; tags, jump hosts, proxy
   commands and import from `~/.ssh/config` remain.
2. Host-key verification with explicit first-use and changed-key UX.
3. Credentials behind a `SecretStore` interface:
   macOS Keychain, Windows Credential Manager and Linux Secret Service.
4. SSH private keys remain in their original files when possible; NTerm stores
   references and keychain-protected passphrases, not copied plaintext keys.
5. Optional encrypted portable vault using an audited KDF and AEAD library.

Exit criterion: no password, passphrase or private key material is stored in the
application database or logs.

## Phase 5 — product hardening

1. SQLite migrations (initial workspace schema complete), backup/export,
   diagnostics and structured redacted logs.
2. Accessibility, IME, international keyboards and screen-reader review.
3. Signed/notarized builds, auto-update, sandbox and dependency audit.
4. Benchmarks for startup, input latency, scrollback memory and large output.
5. Integration tests on macOS, Windows and Linux.

Target budgets: warm start under 300 ms, keystroke-to-paint under 16 ms, local
suggestions under 20 ms, and bounded memory for scrollback.
