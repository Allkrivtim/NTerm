(() => {
  "use strict";

  const els = {
    appShell: document.querySelector(".app-shell"),
    terminal: document.getElementById("terminal"),
    homeScreen: document.getElementById("home-screen"),
    homeButton: document.getElementById("home-button"),
    homeGreeting: document.getElementById("home-greeting"),
    homeSearch: document.getElementById("home-search"),
    homeNotice: document.getElementById("home-notice"),
    projectGroups: document.getElementById("project-groups"),
    serverGroups: document.getElementById("server-groups"),
    projectsCount: document.getElementById("projects-count"),
    serversCount: document.getElementById("servers-count"),
    projectsEmpty: document.getElementById("projects-empty"),
    serversEmpty: document.getElementById("servers-empty"),
    addProject: document.getElementById("add-project"),
    addServer: document.getElementById("add-server"),
    tuiShell: document.getElementById("tui-shell"),
    tuiTerminal: document.getElementById("tui-terminal"),
    blocks: document.getElementById("blocks"),
    editor: document.getElementById("command-editor"),
    syntax: document.getElementById("syntax-highlight"),
    ghost: document.getElementById("ghost"),
    composer: document.getElementById("composer"),
    selectionToolbar: document.getElementById("selection-toolbar"),
    selectionCount: document.getElementById("selection-count"),
    copySelectedCommands: document.getElementById("copy-selected-commands"),
    copySelectedBlocks: document.getElementById("copy-selected-blocks"),
    clearBlockSelection: document.getElementById("clear-block-selection"),
    suggestions: document.getElementById("suggestions"),
    cwd: document.getElementById("cwd"),
    sshSeparator: document.getElementById("ssh-separator"),
    sshHealth: document.getElementById("ssh-health"),
    sshHealthLabel: document.getElementById("ssh-health-label"),
    sshHealthLatency: document.getElementById("ssh-health-latency"),
    gitSeparator: document.getElementById("git-separator"),
    gitContext: document.getElementById("git-context"),
    gitRepository: document.getElementById("git-repository"),
    gitBranch: document.getElementById("git-branch"),
    gitSync: document.getElementById("git-sync"),
    gitChanges: document.getElementById("git-changes"),
    tabList: document.getElementById("tab-list"),
    newTab: document.getElementById("new-tab-button"),
    settings: document.getElementById("settings-button"),
    settingsLayer: document.getElementById("settings-layer"),
    settingsBackdrop: document.getElementById("settings-backdrop"),
    settingsPanel: document.getElementById("settings-panel"),
    settingsClose: document.getElementById("settings-close"),
    settingsForm: document.getElementById("settings-form"),
    settingsReset: document.getElementById("settings-reset"),
    settingsError: document.getElementById("settings-error"),
    fontFamily: document.getElementById("font-family"),
    fontSize: document.getElementById("font-size"),
    defaultPath: document.getElementById("default-path"),
    shellPath: document.getElementById("shell-path"),
    aiEnabled: document.getElementById("ai-enabled"),
    aiStatus: document.getElementById("ai-status"),
    aiStatusMessage: document.getElementById("ai-status-message"),
    reduceTransparency: document.getElementById("reduce-transparency"),
    showBlockMetadata: document.getElementById("show-block-metadata"),
    openHomeOnLaunch: document.getElementById("open-home-on-launch"),
    runProjectCommands: document.getElementById("run-project-commands"),
    sshHelperEnabled: document.getElementById("ssh-helper-enabled"),
    configPath: document.getElementById("config-path"),
    reloadConfig: document.getElementById("reload-config"),
    morningGreetings: document.getElementById("morning-greetings"),
    dayGreetings: document.getElementById("day-greetings"),
    eveningGreetings: document.getElementById("evening-greetings"),
    nightGreetings: document.getElementById("night-greetings"),
    resourceEditorLayer: document.getElementById("resource-editor-layer"),
    resourceEditorBackdrop: document.getElementById("resource-editor-backdrop"),
    resourceEditorClose: document.getElementById("resource-editor-close"),
    resourceEditorKind: document.getElementById("resource-editor-kind"),
    resourceEditorTitle: document.getElementById("resource-editor-title"),
    projectForm: document.getElementById("project-form"),
    projectID: document.getElementById("project-id"),
    projectName: document.getElementById("project-name"),
    projectGroup: document.getElementById("project-group"),
    projectIcon: document.getElementById("project-icon"),
    projectIconPath: document.getElementById("project-icon-path"),
    projectServer: document.getElementById("project-server"),
    projectPath: document.getElementById("project-path"),
    projectCommand: document.getElementById("project-command"),
    projectFormError: document.getElementById("project-form-error"),
    deleteProject: document.getElementById("delete-project"),
    serverForm: document.getElementById("server-form"),
    serverID: document.getElementById("server-id"),
    serverName: document.getElementById("server-name"),
    serverGroup: document.getElementById("server-group"),
    serverHost: document.getElementById("server-host"),
    serverPort: document.getElementById("server-port"),
    serverUser: document.getElementById("server-user"),
    serverKey: document.getElementById("server-key"),
    serverPassword: document.getElementById("server-password"),
    serverPasswordState: document.getElementById("server-password-state"),
    clearPasswordRow: document.getElementById("clear-password-row"),
    clearPassword: document.getElementById("clear-password"),
    serverCommand: document.getElementById("server-command"),
    serverFormError: document.getElementById("server-form-error"),
    deleteServer: document.getElementById("delete-server"),
  };

  const defaults = {
    theme: "system",
    fontFamily: "SF Mono",
    fontSize: 13,
    defaultPath: "~",
    shell: "",
    reduceTransparency: false,
    showBlockMetadata: true,
    aiEnabled: true,
    aiModel: "qwen2.5-coder:0.5b",
    openHomeOnLaunch: true,
    runProjectCommands: true,
    sshHelperEnabled: true,
    morningGreetings: ["Morning. Ready when you are.", "A fresh start for good work.", "Good morning — let's build something."],
    dayGreetings: ["Good afternoon. Keep the momentum.", "Back to the craft.", "Your workspace is ready."],
    eveningGreetings: ["Good evening. One more thoughtful step.", "A quiet evening for focused work.", "Welcome back. Let's finish strong."],
    nightGreetings: ["Still creating? Your workspace is ready.", "A calm night for deep focus.", "Late hours, clear thoughts."],
  };

  const fontStacks = {
    "SF Mono": '"SFMono-Regular", "SF Mono", Menlo, monospace',
    Menlo: 'Menlo, "SF Mono", monospace',
    Monaco: 'Monaco, Menlo, monospace',
    "Cascadia Code": '"Cascadia Code", "SF Mono", monospace',
    "JetBrains Mono": '"JetBrains Mono", "SF Mono", monospace',
  };

  const state = {
    tabs: new Map(),
    activeTabID: "",
    settings: { ...defaults },
    settingsSnapshot: null,
    suggestions: [],
    localSuggestions: [],
    aiSuggestion: null,
    selectedSuggestion: 0,
    localRequestSequence: 0,
    aiRequestSequence: 0,
    home: "",
    commands: new Set(["cd", "echo", "export", "history", "jobs", "kill", "pwd", "source", "type", "unset", "which"]),
    gitRequestSequence: 0,
    settingsAnimationSequence: 0,
    resourceEditorAnimationSequence: 0,
    vtTerminal: null,
    fitAddon: null,
    terminalBlockID: "",
    terminalResizeTimer: 0,
    terminalResizeObserver: null,
    terminalReplaying: false,
    catalog: { projects: [], servers: [] },
    configPath: "",
    homeActive: false,
    lastTabID: "",
    deleteConfirmationTimer: 0,
    greetingPeriod: "",
    greetingText: "",
    openingResource: "",
    homeNoticeTimer: 0,
    syntaxValidationTimer: 0,
    collapsedGroups: { project: new Set(), server: new Set() },
    runningCompositionDraft: null,
    settingsReturnFocus: null,
    resourceEditorReturnFocus: null,
    settingsBusy: false,
    resourceEditorBusy: false,
    closingTabs: new Set(),
    localErrorSequence: 0,
  };

  const bridge = () => window.go?.main?.App;
  const activeTab = () => state.tabs.get(state.activeTabID);

  function sendTerminalInput(tab, value) {
    if (!tab?.runningID || !value || !bridge()) return;
    // Keep bridge writes serialized. PTYs are byte streams, so reordering two
    // fast key events would be observable by readline, prompts and TUIs.
    tab.inputQueue = (tab.inputQueue || Promise.resolve())
      .catch(() => {})
      .then(() => bridge().TerminalInput(tab.id, value))
      .catch(() => {});
  }

  function terminalSequenceForKey(event) {
    if (event.metaKey || event.isComposing) return null;
    const key = event.key;
    if (event.ctrlKey) {
      const lower = key.toLowerCase();
      if (lower.length === 1 && lower >= "a" && lower <= "z") {
        return String.fromCharCode(lower.charCodeAt(0) - 96);
      }
      const control = { "@": "\x00", " ": "\x00", "[": "\x1b", "\\": "\x1c", "]": "\x1d", "^": "\x1e", "_": "\x1f", "?": "\x7f" };
      if (control[key] !== undefined) return control[key];
    }
    const sequences = {
      Enter: "\r", Backspace: "\x7f", Tab: event.shiftKey ? "\x1b[Z" : "\t", Escape: "\x1b",
      ArrowUp: "\x1b[A", ArrowDown: "\x1b[B", ArrowRight: "\x1b[C", ArrowLeft: "\x1b[D",
      Home: "\x1b[H", End: "\x1b[F", Insert: "\x1b[2~", Delete: "\x1b[3~",
      PageUp: "\x1b[5~", PageDown: "\x1b[6~",
      F1: "\x1bOP", F2: "\x1bOQ", F3: "\x1bOR", F4: "\x1bOS", F5: "\x1b[15~", F6: "\x1b[17~",
      F7: "\x1b[18~", F8: "\x1b[19~", F9: "\x1b[20~", F10: "\x1b[21~", F11: "\x1b[23~", F12: "\x1b[24~",
    };
    if (sequences[key] !== undefined) return sequences[key];
    if (["Shift", "Control", "Alt", "Meta", "CapsLock", "Fn", "Dead", "Process", "Unidentified"].includes(key)) return null;
    if (event.ctrlKey) return null;
    return event.altKey ? `\x1b${key}` : key;
  }

  function capturesRunningInput() {
    return Boolean(
      activeTab()?.runningID
      && document.body.dataset.terminalMode !== "true"
      && els.settingsLayer.hidden
      && els.resourceEditorLayer.hidden
    );
  }

  function setComposerRunning(running) {
    els.composer.classList.toggle("is-running", Boolean(running));
    els.editor.placeholder = running ? "Input is sent to the running command…" : "Type a command…";
  }
  const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const nextAnimationFrame = () => new Promise((resolve) => requestAnimationFrame(resolve));
  const debounce = (fn, wait) => {
    let timer;
    const wrapped = (...args) => {
      clearTimeout(timer);
      timer = setTimeout(() => fn(...args), wait);
    };
    wrapped.cancel = () => clearTimeout(timer);
    return wrapped;
  };

  function escapeHTML(value) {
    return String(value).replace(/[&<>"']/g, (char) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;",
    })[char]);
  }

  const fullscreenCommands = new Set([
    "btop", "emacs", "fzf", "htop", "lazygit", "less", "man", "micro", "more",
    "nano", "nvim", "pico", "screen", "tig", "tmux", "top", "vi", "vim", "watch",
  ]);
  const maxBlockOutput = 1024 * 1024;
  const outputTruncatedMarker = "… older output truncated …\n";

  function commandExecutable(command) {
    const words = String(command || "").trim().match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) || [];
    let index = 0;
    while (["command", "env", "sudo"].includes(words[index])) index += 1;
    while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[index] || "")) index += 1;
    return (words[index] || "").replace(/^.*\//, "").replace(/["']/g, "");
  }

  function prefersFullscreen(command) {
    return fullscreenCommands.has(commandExecutable(command));
  }

  function terminalTheme() {
    const forcedLight = document.documentElement.dataset.theme === "light";
    const systemLight = document.documentElement.dataset.theme === "system"
      && window.matchMedia("(prefers-color-scheme: light)").matches;
    return forcedLight || systemLight
      ? {
        background: "#efeff1", foreground: "#242426", cursor: "#242426", selectionBackground: "#00000020",
        black: "#343438", red: "#b7474c", green: "#367846", yellow: "#946f24",
        blue: "#356fa8", magenta: "#845495", cyan: "#267d82", white: "#d9d9dc",
        brightBlack: "#727278", brightRed: "#d15a60", brightGreen: "#46975a", brightYellow: "#b38b35",
        brightBlue: "#4b89c5", brightMagenta: "#a16bb3", brightCyan: "#369ca2", brightWhite: "#ffffff",
      }
      : {
        background: "#141415", foreground: "#ededee", cursor: "#ededee", selectionBackground: "#ffffff28",
        black: "#252527", red: "#d66b70", green: "#66b77e", yellow: "#c6a35b",
        blue: "#72a3d8", magenta: "#ad86bd", cyan: "#66aaa8", white: "#d7d7da",
        brightBlack: "#737378", brightRed: "#ed8589", brightGreen: "#82c996", brightYellow: "#dbc078",
        brightBlue: "#8ab8e8", brightMagenta: "#c49bd1", brightCyan: "#82c4c2", brightWhite: "#ffffff",
      };
  }

  function resizeInteractiveTerminal() {
    if (!state.fitAddon || !state.vtTerminal) return;
    try { state.fitAddon.fit(); } catch (_) { return; }
    const tab = activeTab();
    if (!tab || !bridge()) return;
    window.clearTimeout(state.terminalResizeTimer);
    state.terminalResizeTimer = window.setTimeout(() => {
      bridge().ResizeTerminal(tab.id, state.vtTerminal.cols, state.vtTerminal.rows).catch(() => {});
    }, 30);
  }

  function scheduleInteractiveResize() {
    requestAnimationFrame(() => {
      resizeInteractiveTerminal();
      requestAnimationFrame(() => {
        resizeInteractiveTerminal();
        if (state.vtTerminal) {
          state.vtTerminal.refresh(0, Math.max(0, state.vtTerminal.rows - 1));
        }
      });
    });
  }

  function initializeInteractiveTerminal() {
    if (!window.Terminal || !window.FitAddon?.FitAddon || state.vtTerminal) return;
    state.vtTerminal = new window.Terminal({
      allowProposedApi: false,
      convertEol: false,
      cursorBlink: true,
      cursorStyle: "block",
      fontFamily: fontStacks[state.settings.fontFamily] || fontStacks["SF Mono"],
      fontSize: Math.max(10, Math.min(24, Number(state.settings.fontSize) || 13)),
      scrollback: 5000,
      theme: terminalTheme(),
    });
    state.fitAddon = new window.FitAddon.FitAddon();
    state.vtTerminal.loadAddon(state.fitAddon);
    state.vtTerminal.open(els.tuiTerminal);
    state.vtTerminal.onWriteParsed(() => {
      if (document.body.dataset.terminalMode === "true" && state.vtTerminal) {
        state.vtTerminal.refresh(0, Math.max(0, state.vtTerminal.rows - 1));
      }
    });
    state.vtTerminal.onData((data) => {
      const tab = activeTab();
      if (!tab?.runningID || !bridge() || state.terminalReplaying || state.terminalBlockID !== tab.runningID) return;
      sendTerminalInput(tab, data);
    });
    state.terminalResizeObserver?.disconnect();
    state.terminalResizeObserver = new ResizeObserver(resizeInteractiveTerminal);
    state.terminalResizeObserver.observe(els.tuiTerminal);
    scheduleInteractiveResize();
  }

  function disposeInteractiveTerminal() {
    state.terminalResizeObserver?.disconnect();
    state.terminalResizeObserver = null;
    state.vtTerminal?.dispose();
    state.vtTerminal = null;
    state.fitAddon = null;
    els.tuiTerminal.replaceChildren();
  }

  function interactiveTerminalHasGeometry() {
    const screen = els.tuiTerminal.querySelector(".xterm-screen");
    const bounds = screen?.getBoundingClientRect();
    return Boolean(
      state.vtTerminal
      && state.vtTerminal.cols > 1
      && state.vtTerminal.rows > 1
      && bounds
      && bounds.width > 8
      && bounds.height > 8
    );
  }

  async function waitForInteractiveGeometry(frameLimit = 10) {
    for (let frame = 0; frame < frameLimit; frame += 1) {
      resizeInteractiveTerminal();
      if (interactiveTerminalHasGeometry()) return true;
      await nextAnimationFrame();
    }
    return interactiveTerminalHasGeometry();
  }

  async function prepareInteractiveRenderer(tab, record) {
    if (!tab || !record) return;

    if (record.interactive) {
      document.body.dataset.terminalMode = "true";
      els.tuiShell.setAttribute("aria-hidden", "false");
    }

    await nextAnimationFrame();

    // xterm must be opened in a visible element with dimensions. Recreate it for
    // full-screen programs so a renderer opened during application bootstrap can
    // never retain a zero-sized rendering surface.
    if (record.interactive || !state.vtTerminal) {
      disposeInteractiveTerminal();
      initializeInteractiveTerminal();
    }

    prepareTerminalRecord(tab, record);
    if (record.interactive) showInteractiveTerminal(tab, record, false);
    const rendererReady = await waitForInteractiveGeometry();
    if (record.interactive && !rendererReady) {
      disposeInteractiveTerminal();
      initializeInteractiveTerminal();
      prepareTerminalRecord(tab, record);
      showInteractiveTerminal(tab, record, false);
      if (!await waitForInteractiveGeometry()) {
        throw new Error("interactive terminal renderer has no drawable area");
      }
    }
    state.vtTerminal?.refresh(0, Math.max(0, state.vtTerminal.rows - 1));
  }

  function showInteractiveTerminal(tab, record, replay = true) {
    if (!tab || !record) return;
    const changed = state.terminalBlockID !== record.data.id;
    state.terminalBlockID = record.data.id;
    document.body.dataset.terminalMode = "true";
    els.tuiShell.setAttribute("aria-hidden", "false");
    if (!state.vtTerminal) return;
    if (changed || replay) {
      state.terminalReplaying = true;
      state.vtTerminal.reset();
      if (record.rawOutput) state.vtTerminal.write(record.rawOutput, () => { state.terminalReplaying = false; });
      else state.terminalReplaying = false;
    }
    scheduleInteractiveResize();
    requestAnimationFrame(() => state.vtTerminal?.focus());
  }

  function prepareTerminalRecord(tab, record, replay = false) {
    if (!state.vtTerminal || !tab || !record) return;
    state.terminalBlockID = record.data.id;
    state.terminalReplaying = replay;
    state.vtTerminal.reset();
    if (replay && record.rawOutput) {
      state.vtTerminal.write(record.rawOutput, () => { state.terminalReplaying = false; });
    } else {
      state.terminalReplaying = false;
    }
    requestAnimationFrame(resizeInteractiveTerminal);
  }

  function hideInteractiveTerminal(focusEditor = true) {
    document.body.dataset.terminalMode = "false";
    els.tuiShell.setAttribute("aria-hidden", "true");
    state.terminalBlockID = "";
    if (focusEditor) requestAnimationFrame(() => els.editor.focus());
  }

  function syncInteractiveTerminal(tab) {
    const record = tab?.runningID ? tab.blocks.get(tab.runningID) : null;
    if (!record) {
      hideInteractiveTerminal(false);
      return;
    }
    prepareTerminalRecord(tab, record, true);
    if (record.interactive) showInteractiveTerminal(tab, record, false);
    else {
      document.body.dataset.terminalMode = "false";
      els.tuiShell.setAttribute("aria-hidden", "true");
    }
  }

  function syntaxToken(value, type, extraClass = "") {
    const classes = [`syntax-${type}`, extraClass].filter(Boolean).join(" ");
    return `<span class="${classes}">${escapeHTML(value)}</span>`;
  }

  // This lexer deliberately accepts unfinished input. A strict shell parser is
  // a poor fit while the user is still halfway through a quote or substitution.
  const shellOperators = [
    ";;&", "&>>", "<<<", "<<-", "&&", "||", "|&", ">>", "<<", "<>", ">&", "<&", "&>", ">|",
    ";;", ";&", "(", ")", ";", "|", "&", "<", ">",
  ];
  const commandSeparators = new Set(["&&", "||", "|", "|&", ";", ";;", ";&", ";;&", "&", "(", ")"]);
  const redirectionOperators = new Set(["<", ">", ">>", "<<", "<<-", "<<<", "<>", ">&", "<&", "&>", "&>>", ">|"]);
  const shellKeywords = new Set(["!", "[[", "]]", "case", "coproc", "do", "done", "elif", "else", "esac", "fi", "for", "function", "if", "in", "select", "then", "time", "until", "while", "{", "}"]);
  const keywordStartsCommand = new Set(["!", "coproc", "do", "elif", "else", "if", "then", "time", "until", "while", "{"]);
  const commandFamilies = new Set([
    "aws", "brew", "bun", "cargo", "composer", "deno", "docker", "dotnet", "gh", "git", "go",
    "helm", "heroku", "jj", "kubectl", "make", "mix", "npm", "npx", "pip", "pip3", "pnpm",
    "podman", "poetry", "rails", "rustup", "swift", "systemctl", "terraform", "uv", "vagrant", "yarn",
  ]);
  const pathArgumentCommands = new Set([
    ".", "cat", "cd", "code", "cp", "du", "find", "head", "less", "ln", "ls", "micro", "mkdir",
    "mv", "nano", "nvim", "open", "readlink", "realpath", "rm", "rmdir", "rsync", "scp", "source",
    "stat", "tail", "touch", "tree", "vi", "vim",
  ]);
  const commandOptionValues = new Map([
    ["aws", new Set(["--ca-bundle", "--cli-connect-timeout", "--cli-read-timeout", "--endpoint-url", "--output", "--profile", "--region"])],
    ["docker", new Set(["--config", "--context", "--host", "-H", "--log-level"])],
    ["git", new Set(["-C", "-c", "--exec-path", "--git-dir", "--namespace", "--super-prefix", "--work-tree"])],
    ["kubectl", new Set(["--as", "--as-group", "--cache-dir", "--certificate-authority", "--client-certificate", "--client-key", "--cluster", "--context", "--kubeconfig", "-n", "--namespace", "--request-timeout", "--server", "--token", "--user"])],
    ["podman", new Set(["--connection", "--events-backend", "--identity", "--log-level", "--root", "--runroot", "--storage-driver", "--tmpdir", "--url"])],
    ["terraform", new Set(["-chdir"])],
  ]);
  const commandWrappers = new Map([
    ["command", new Set(["-p", "-v", "-V"])],
    ["builtin", new Set()],
    ["exec", new Set(["-a"])],
    ["env", new Set(["-C", "--chdir", "-S", "--split-string", "-u", "--unset"])],
    ["nice", new Set(["-n", "--adjustment"])],
    ["nohup", new Set()],
    ["sudo", new Set(["-C", "-D", "-g", "-h", "-p", "-R", "-r", "-t", "-T", "-u", "--chdir", "--close-from", "--group", "--host", "--prompt", "--role", "--type", "--user"])],
    ["time", new Set(["-f", "--format", "-o", "--output"])],
    ["xargs", new Set(["-a", "--arg-file", "-d", "--delimiter", "-E", "-I", "-L", "-n", "-P", "-s"])],
  ]);

  function shellOperatorAt(value, index) {
    return shellOperators.find((operator) => value.startsWith(operator, index)) || "";
  }

  function scanShellTokens(value) {
    const tokens = [];
    let index = 0;
    while (index < value.length) {
      const start = index;
      if (/\s/.test(value[index])) {
        while (index < value.length && /\s/.test(value[index])) index += 1;
        tokens.push({ kind: "space", raw: value.slice(start, index) });
        continue;
      }
      if (value[index] === "#") {
        const end = value.indexOf("\n", index);
        index = end < 0 ? value.length : end;
        tokens.push({ kind: "comment", raw: value.slice(start, index) });
        continue;
      }
      const operator = shellOperatorAt(value, index);
      if (operator) {
        index += operator.length;
        tokens.push({ kind: "operator", raw: operator });
        continue;
      }

      let quote = "";
      let substitutionDepth = 0;
      while (index < value.length) {
        const char = value[index];
        if (char === "\\" && quote !== "'") {
          index += Math.min(2, value.length - index);
          continue;
        }
        if (quote) {
          if (char === quote) quote = "";
          index += 1;
          continue;
        }
        if (char === "'" || char === "\"") {
          quote = char;
          index += 1;
          continue;
        }
        if (value.startsWith("$(", index)) {
          substitutionDepth += 1;
          index += 2;
          continue;
        }
        if (substitutionDepth && char === "(") {
          substitutionDepth += 1;
          index += 1;
          continue;
        }
        if (substitutionDepth && char === ")") {
          substitutionDepth -= 1;
          index += 1;
          continue;
        }
        if (!substitutionDepth && (/\s/.test(char) || shellOperatorAt(value, index))) break;
        index += 1;
      }
      // Always make progress for malformed input that starts with an unknown control.
      if (index === start) index += 1;
      const raw = value.slice(start, index);
      const nextOperator = shellOperatorAt(value, index);
      const fileDescriptor = /^\d+$/.test(raw) && redirectionOperators.has(nextOperator);
      tokens.push({ kind: fileDescriptor ? "operator" : "word", raw });
    }
    return tokens;
  }

  function decodeShellWord(word) {
    let decoded = "";
    let quote = "";
    for (let index = 0; index < word.length; index += 1) {
      const char = word[index];
      if (char === "\\" && quote !== "'" && index + 1 < word.length) {
        decoded += word[++index];
      } else if (!quote && (char === "'" || char === "\"")) {
        quote = char;
      } else if (quote === char) {
        quote = "";
      } else {
        decoded += char;
      }
    }
    return decoded;
  }

  function isShellAssignment(word) {
    return /^[A-Za-z_][A-Za-z0-9_]*(?:\+)?=/.test(word);
  }

  function isLikelyPath(word) {
    const decoded = decodeShellWord(word);
    if (/^(?:~|\.{1,2})(?:\/|$)|^\//.test(decoded) || decoded.includes("/")) return true;
    if (/[*?\[]/.test(decoded)) return true;
    return /\.(?:[cm]?[jt]sx?|c|cc|cpp|css|env|go|h|hpp|html?|ini|java|json|jsx|kt|lock|lua|md|mjs|pdf|php|plist|py|rb|rs|sh|sql|swift|toml|tsx?|txt|vue|xml|ya?ml|zsh)$/i.test(decoded);
  }

  function variableEnd(word, start) {
    if (word[start] !== "$" || start + 1 >= word.length) return start + 1;
    const next = word[start + 1];
    if (next === "{" || next === "(") {
      const open = next;
      const close = open === "{" ? "}" : ")";
      let depth = 1;
      let index = start + 2;
      if (open === "(" && word[index] === "(") { depth = 2; index += 1; }
      while (index < word.length && depth) {
        if (word[index] === "\\") index += 2;
        else {
          if (word[index] === open) depth += 1;
          if (word[index] === close) depth -= 1;
          index += 1;
        }
      }
      return index;
    }
    const special = next.match(/[0-9@*#?$!_-]/);
    if (special) return start + 2;
    const name = word.slice(start + 1).match(/^[A-Za-z_][A-Za-z0-9_]*/)?.[0] || "";
    return name ? start + 1 + name.length : start + 1;
  }

  function shellVariableRanges(word) {
    const ranges = [];
    let quote = "";
    let index = 0;
    while (index < word.length) {
      const char = word[index];
      if (char === "\\" && quote !== "'") {
        index += Math.min(2, word.length - index);
        continue;
      }
      if (char === "'" && quote !== "\"") {
        quote = quote === "'" ? "" : "'";
        index += 1;
        continue;
      }
      if (char === "\"" && quote !== "'") {
        quote = quote === "\"" ? "" : "\"";
        index += 1;
        continue;
      }
      if (char === "$" && quote !== "'") {
        const end = variableEnd(word, index);
        if (end > index + 1) ranges.push([index, end]);
        index = Math.max(index + 1, end);
        continue;
      }
      index += 1;
    }
    return ranges;
  }

  function renderWordParts(word, baseType, extraClass = "") {
    let result = "";
    let index = 0;
    const quoted = /(^|[^\\])["']/.test(word);
    const fallbackType = quoted && ["argument", "path"].includes(baseType) ? "string" : baseType;
    for (const [start, end] of shellVariableRanges(word)) {
      if (start > index) result += syntaxToken(word.slice(index, start), fallbackType, extraClass);
      result += syntaxToken(word.slice(start, end), "variable");
      index = end;
    }
    if (index < word.length) result += syntaxToken(word.slice(index), fallbackType, extraClass);
    return result || syntaxToken(word, fallbackType, extraClass);
  }

  function renderAssignment(word) {
    const equals = word.indexOf("=");
    const head = syntaxToken(word.slice(0, equals + 1), "variable");
    const value = word.slice(equals + 1);
    if (!value) return head;
    return head + renderWordParts(value, isLikelyPath(value) ? "path" : "argument");
  }

  function renderOption(word) {
    const equals = word.indexOf("=");
    if (equals < 0) return renderWordParts(word, "option");
    const value = word.slice(equals + 1);
    return syntaxToken(word.slice(0, equals + 1), "option")
      + renderWordParts(value, isLikelyPath(value) ? "path" : "argument");
  }

  function commandNameForWord(word) {
    const decoded = decodeShellWord(word);
    return decoded.includes("/") ? decoded.split("/").filter(Boolean).at(-1) || decoded : decoded;
  }

  function commandIsKnown(word, commands) {
    const decoded = decodeShellWord(word);
    if (!decoded || /[$`*?]/.test(decoded)) return true;
    if (decoded.includes("/")) return true;
    return commands.has(decoded) || shellKeywords.has(decoded);
  }

  function freshShellContext() {
    return { expectsCommand: true, command: "", argumentIndex: 0, wrapper: "", wrapperValue: false, optionValue: false, redirectTarget: false };
  }

  function renderShellSyntax(source, commands = state.commands, validateCommands = false) {
    const value = String(source || "");
    let context = freshShellContext();
    let result = "";
    for (const token of scanShellTokens(value)) {
      if (token.kind === "space") {
        result += escapeHTML(token.raw);
        if (token.raw.includes("\n")) context = freshShellContext();
        continue;
      }
      if (token.kind === "comment") {
        result += syntaxToken(token.raw, "comment");
        continue;
      }
      if (token.kind === "operator") {
        result += syntaxToken(token.raw, "operator");
        if (redirectionOperators.has(token.raw)) context.redirectTarget = true;
        else if (commandSeparators.has(token.raw)) context = freshShellContext();
        continue;
      }

      const word = token.raw;
      const decoded = decodeShellWord(word);
      if (context.redirectTarget) {
        result += renderWordParts(word, "path");
        context.redirectTarget = false;
        continue;
      }

      if (context.expectsCommand) {
        if (isShellAssignment(word)) {
          result += renderAssignment(word);
          continue;
        }
        if (context.wrapperValue) {
          result += renderWordParts(word, isLikelyPath(word) ? "path" : "argument");
          context.wrapperValue = false;
          continue;
        }
        if (context.wrapper && /^-/.test(decoded)) {
          result += renderOption(word);
          if (commandWrappers.get(context.wrapper)?.has(decoded.split("=")[0]) && !decoded.includes("=")) context.wrapperValue = true;
          continue;
        }
        if (shellKeywords.has(decoded)) {
          result += syntaxToken(word, "keyword");
          context.expectsCommand = keywordStartsCommand.has(decoded);
          continue;
        }

        const commandName = commandNameForWord(word);
        const known = commandIsKnown(word, commands);
        result += renderWordParts(word, "command", validateCommands && !known ? "syntax-invalid" : "");
        if (commandWrappers.has(commandName)) {
          context.wrapper = commandName;
          context.expectsCommand = true;
        } else {
          context.command = commandName;
          context.argumentIndex = 0;
          context.wrapper = "";
          context.expectsCommand = false;
        }
        continue;
      }

      if (context.optionValue) {
        result += renderWordParts(word, isLikelyPath(word) ? "path" : "argument");
        context.optionValue = false;
      } else if (shellKeywords.has(decoded)) {
        result += syntaxToken(word, "keyword");
        context.expectsCommand = keywordStartsCommand.has(decoded);
      } else if (/^--?[^-]/.test(decoded)) {
        result += renderOption(word);
        const optionName = decoded.split("=")[0];
        if (!decoded.includes("=") && commandOptionValues.get(context.command)?.has(optionName)) context.optionValue = true;
      } else if (context.argumentIndex === 0 && commandFamilies.has(context.command)) {
        result += renderWordParts(word, "subcommand");
        context.argumentIndex += 1;
      } else if (isShellAssignment(word)) {
        result += renderAssignment(word);
        context.argumentIndex += 1;
      } else if (isLikelyPath(word) || pathArgumentCommands.has(context.command)) {
        result += renderWordParts(word, "path");
        context.argumentIndex += 1;
      } else {
        result += renderWordParts(word, "argument");
        context.argumentIndex += 1;
      }
    }
    return result;
  }

  function syncEditorLayers() {
    els.syntax.scrollTop = els.editor.scrollTop;
    els.syntax.scrollLeft = els.editor.scrollLeft;
    els.ghost.scrollTop = els.editor.scrollTop;
    els.ghost.scrollLeft = els.editor.scrollLeft;
  }

  function refreshSyntax(validateCommands = false) {
    window.clearTimeout(state.syntaxValidationTimer);
    const source = els.editor.value;
    els.syntax.innerHTML = renderShellSyntax(source, state.commands, validateCommands);
    if (!validateCommands && source.trim()) {
      state.syntaxValidationTimer = window.setTimeout(() => {
        if (els.editor.value === source) refreshSyntax(true);
      }, 420);
    }
    syncEditorLayers();
  }

  async function refreshCommandInventory(tabID) {
    if (!bridge() || !tabID) return;
    try {
      const commands = await bridge().CommandNames(tabID);
      if (!Array.isArray(commands) || !commands.length) return;
      const tab = state.tabs.get(tabID);
      if (!tab) return;
      tab.commands = new Set(commands);
      if (state.activeTabID === tabID) {
        state.commands = new Set(tab.commands);
        refreshSyntax();
      }
    } catch (_) { /* keep the fast PATH inventory */ }
  }

  function renderGitContext(info) {
    const visible = Boolean(info?.isRepository);
    els.gitSeparator.hidden = !visible;
    els.gitContext.hidden = !visible;
    if (!visible) return;

    els.gitRepository.textContent = info.name || "repository";
    els.gitBranch.textContent = info.branch || (info.detached ? "detached" : "no branch");
    const sync = [
      info.ahead ? `↑${info.ahead}` : "",
      info.behind ? `↓${info.behind}` : "",
    ].filter(Boolean).join(" ");
    els.gitSync.textContent = sync;
    els.gitSync.hidden = !sync;

    const changes = [
      info.staged ? `+${info.staged}` : "",
      info.modified ? `~${info.modified}` : "",
      info.untracked ? `?${info.untracked}` : "",
    ].filter(Boolean);
    const dirty = changes.length > 0;
    els.gitChanges.dataset.state = dirty ? "dirty" : "clean";
    els.gitChanges.textContent = dirty ? changes.join(" ") : "clean";
    els.gitContext.title = `${info.root || info.name}${info.branch ? ` · ${info.branch}` : ""}`;
  }

  async function refreshGitContext(tabID) {
    const tab = state.tabs.get(tabID);
    if (!tab || !bridge()) return;
    const requestID = ++state.gitRequestSequence;
    tab.gitRequestID = requestID;
    try {
      const info = await bridge().GitContext(tabID);
      if (!state.tabs.has(tabID) || tab.gitRequestID !== requestID) return;
      tab.gitInfo = info;
      if (state.activeTabID === tabID) renderGitContext(info);
    } catch (_) {
      if (state.activeTabID === tabID) renderGitContext(null);
    }
  }

  function cleanError(error) {
    return String(error).replace(/^Error:\s*/, "").trim() || "Unexpected error";
  }

  function shortPath(path) {
    if (!path) return "";
    return state.home && path.startsWith(state.home) ? `~${path.slice(state.home.length)}` : path;
  }

  function displayPath(tab) {
    const path = tab?.remote ? tab.cwd : shortPath(tab?.cwd);
    return tab?.remote ? `${tab.remote}:${path}` : path;
  }

  function titleForPath(path) {
    const short = shortPath(path);
    if (short === "~") return "~";
    const parts = String(path || "Shell").split("/").filter(Boolean);
    return parts.at(-1) || "/";
  }

  function applyVisualSettings(settings) {
    const theme = ["system", "light", "dark"].includes(settings.theme) ? settings.theme : "system";
    document.documentElement.dataset.theme = theme;
    document.documentElement.dataset.reduceTransparency = String(Boolean(settings.reduceTransparency));
    document.body.dataset.showMeta = String(settings.showBlockMetadata !== false);
    const size = Math.max(10, Math.min(24, Number(settings.fontSize) || 13));
    document.documentElement.style.setProperty("--terminal-font-size", `${size}px`);
    document.documentElement.style.setProperty("--mono", fontStacks[settings.fontFamily] || fontStacks["SF Mono"]);
    if (state.vtTerminal) {
      state.vtTerminal.options.fontFamily = fontStacks[settings.fontFamily] || fontStacks["SF Mono"];
      state.vtTerminal.options.fontSize = size;
      state.vtTerminal.options.theme = terminalTheme();
      requestAnimationFrame(resizeInteractiveTerminal);
    }
    autoSize();
  }

  function autoSize() {
    els.editor.style.height = "0px";
    els.editor.style.height = `${Math.min(150, Math.max(32, els.editor.scrollHeight))}px`;
  }

  function setEditor(value, caretAtEnd = true) {
    els.editor.value = value;
    if (caretAtEnd) els.editor.setSelectionRange(value.length, value.length);
    autoSize();
    refreshSyntax();
    refreshGhost();
  }

  function addTab(meta) {
    if (!meta || state.tabs.has(meta.id)) return state.tabs.get(meta?.id);
    const pane = document.createElement("section");
    pane.className = "tab-pane";
    pane.dataset.tabId = meta.id;
    pane.hidden = true;
    els.blocks.appendChild(pane);

    const element = document.createElement("div");
    element.className = "tab-button";
    element.dataset.tabId = meta.id;
    element.dataset.running = String(Boolean(meta.running));
    element.setAttribute("role", "tab");
    element.setAttribute("aria-selected", "false");
    element.tabIndex = -1;
    element.innerHTML = `
      <span class="tab-running" aria-hidden="true"></span>
      <span class="tab-title">${escapeHTML(meta.title || titleForPath(meta.cwd))}</span>
      <button class="tab-close" data-close-tab="${escapeHTML(meta.id)}" aria-label="Close tab" title="Close tab">
        <svg aria-hidden="true" viewBox="0 0 12 12"><path d="m3 3 6 6M9 3 3 9"/></svg>
      </button>`;
    els.tabList.appendChild(element);

    const tab = {
      id: meta.id,
      title: meta.title || titleForPath(meta.cwd),
      cwd: meta.cwd || "",
      remote: meta.remote || "",
      running: Boolean(meta.running),
      runningID: "",
      blocks: new Map(),
      history: [],
      historyIndex: -1,
      draftBeforeHistory: "",
      editorDraft: "",
      scrollTop: 0,
      gitInfo: null,
      gitRequestID: 0,
      launchQueue: [],
      fixedTitle: "",
      selectedBlocks: new Set(),
      selectionAnchor: "",
      sshStatus: meta.remote ? { state: "connected", label: meta.remote, latencyMs: 0 } : null,
      inputQueue: Promise.resolve(),
      commands: new Set(state.commands),
      pane,
      element,
    };
    state.tabs.set(tab.id, tab);
    renderTab(tab);
    return tab;
  }

  function renderTab(tab) {
    tab.element.dataset.running = String(Boolean(tab.running));
    tab.element.title = tab.remote ? `${tab.remote}:${tab.cwd}` : tab.cwd;
    tab.element.querySelector(".tab-title").textContent = tab.title;
  }

  function renderSSHStatus(tab = activeTab()) {
    const status = tab?.sshStatus;
    const visible = Boolean(status || tab?.remote);
    els.sshSeparator.hidden = !visible;
    els.sshHealth.hidden = !visible;
    if (!visible) return;
    const stateName = status?.state || "connected";
    els.sshHealth.dataset.state = stateName;
    els.sshHealthLabel.textContent = stateName === "connected"
      ? "SSH"
      : stateName === "connecting"
        ? "Connecting"
        : stateName === "reconnecting" ? `Reconnecting${status?.attempt ? ` · ${status.attempt}` : ""}` : "Offline";
    const latency = Number(status?.latencyMs) || 0;
    els.sshHealth.dataset.quality = stateName === "connected" && latency >= 500 ? "slow" : "normal";
    els.sshHealthLatency.textContent = stateName === "connected" && latency > 0 ? `${latency} ms` : "";
    els.sshHealth.title = status?.message || (stateName === "connected" ? `${status?.label || tab.remote} · connected` : "Click to retry now");
    els.sshHealth.disabled = stateName === "connecting";
  }

  function updateSSHStatus(payload) {
    const tab = state.tabs.get(payload?.tabId);
    if (!tab) return;
    if (payload.state === "closed") {
      tab.sshStatus = null;
      tab.remote = "";
      if (state.activeTabID === tab.id) renderSSHStatus(tab);
      return;
    }
    tab.sshStatus = payload;
    if (payload.label && !tab.remote && payload.state === "connected") tab.remote = payload.label;
    tab.element.dataset.sshState = payload.state || "disconnected";
    if (tab.id === state.activeTabID) renderSSHStatus(tab);
  }

  function groupedResources(items) {
    const groups = new Map();
    for (const item of items || []) {
      const group = String(item.group || "").trim() || "Ungrouped";
      if (!groups.has(group)) groups.set(group, []);
      groups.get(group).push(item);
    }
    return [...groups.entries()].sort(([left], [right]) => {
      if (left === "Ungrouped") return 1;
      if (right === "Ungrouped") return -1;
      return left.localeCompare(right);
    });
  }

  const iconDrawings = {
    code: '<path d="m5.5 4-3 4 3 4M10.5 4l3 4-3 4M9 2.8 7 13.2"/>',
    go: '<path d="M3 6.2h7.8a2.7 2.7 0 0 1 0 5.4H6.2A3.2 3.2 0 0 1 3 8.4zM1.5 7.5H5M2 9.5h3"/><circle cx="10.5" cy="8.4" r=".5"/>',
    python: '<path d="M8 2.5H5.2A2.2 2.2 0 0 0 3 4.7V7h5v2H3.6A2.1 2.1 0 0 0 1.5 11v.8M8 13.5h2.8a2.2 2.2 0 0 0 2.2-2.2V9H8V7h4.4a2.1 2.1 0 0 0 2.1-2v-.8"/><path d="M6 4.5h.01M10 11.5h.01"/>',
    docker: '<path d="M2 8.5h10.5c-.4 2.8-2.2 4.2-5.3 4.2-2.6 0-4.2-1.1-5.2-3.2zM4 6.5h2v2H4zM6.2 6.5h2v2h-2zM8.4 6.5h2v2h-2zM6.2 4.3h2v2h-2zM12 7c.7-.8 1.4-1 2.5-.8-.1 1.2-.8 2-2 2.2"/>',
    node: '<path d="M8 1.8 13.4 5v6L8 14.2 2.6 11V5zM5.5 10.8V5.2l5 5.6V5.2"/>',
    rust: '<circle cx="8" cy="8" r="3.6"/><circle cx="8" cy="8" r="1.4"/><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4"/>',
    web: '<circle cx="8" cy="8" r="5.5"/><path d="M2.7 8h10.6M8 2.5c1.6 1.5 2.3 3.3 2.3 5.5S9.6 12 8 13.5C6.4 12 5.7 10.2 5.7 8S6.4 4 8 2.5"/>',
    database: '<ellipse cx="8" cy="4" rx="4.8" ry="2"/><path d="M3.2 4v4c0 1.1 2.1 2 4.8 2s4.8-.9 4.8-2V4M3.2 8v4c0 1.1 2.1 2 4.8 2s4.8-.9 4.8-2V8"/>',
    mobile: '<rect x="4.3" y="1.5" width="7.4" height="13" rx="1.5"/><path d="M6.7 3h2.6M7.5 12.7h1"/>',
    bot: '<rect x="2.5" y="5" width="11" height="8" rx="2"/><path d="M8 2.5V5M6 8h.01M10 8h.01M5.5 11h5"/>',
    server: '<rect x="2.5" y="3" width="11" height="4" rx="1"/><rect x="2.5" y="9" width="11" height="4" rx="1"/><path d="M5 5h.01M5 11h.01M7 5h3.5M7 11h3.5"/>',
    ubuntu: '<circle cx="8" cy="8" r="3.1"/><circle cx="8" cy="2.1" r="1"/><circle cx="3" cy="11.2" r="1"/><circle cx="13" cy="11.2" r="1"/><path d="M8 3.2v1.6M4 10.6l1.4-.8M12 10.6l-1.4-.8"/>',
    debian: '<path d="M9.8 3.2c-2.5-1.1-5.5.4-5.8 3-.3 2.8 2.2 5 5 4.3 2.3-.6 3.2-3.3 1.8-5-1-1.2-3.1-1.3-4-.1-.8 1.1 0 2.6 1.3 2.8 1 .1 1.8-.6 1.7-1.4"/>',
    fedora: '<path d="M8 2.3a5.7 5.7 0 1 1-5.7 5.8V7A4.7 4.7 0 0 1 7 2.3h2.4M5 12V7.1c0-1.5 1-2.4 2.5-2.4h2M4 8h5"/>',
    arch: '<path d="m8 2 5.5 11-5.5-3-5.5 3zM5.7 9 8 5.2 10.3 9 8 8z"/>',
    alpine: '<path d="m1.8 12.8 4.4-7 2 3.2 1.5-2.3 4.5 6.1M4.3 9h2.8M9 10h3"/>',
    centos: '<path d="M3 3h4v4H3zM9 3h4v4H9zM3 9h4v4H3zM9 9h4v4H9zM7 5l2 2M9 9l-2 2"/>',
    rocky: '<path d="m2.2 12.8 5-9.6 2.1 4 1.2-2 3.3 7.6zM5 10l2.3-2 2.1 1.8 1.6-1"/>',
    opensuse: '<path d="M2.2 8.5c.8-3 3.2-4.8 6.3-4.8 2.3 0 4.3 1 5.3 2.8-1.3-.5-2.4-.2-3.2.8-1.4 1.8-3.5 2.2-5.7 1.1M6 8.8c-.3 2 1 3.6 3 3.8 1.8.2 3.5-.9 4.2-2.5"/><circle cx="11.3" cy="6.6" r=".5"/>',
    freebsd: '<path d="M4.2 5.2 2.5 2.5l3.2 1.2M11.8 5.2l1.7-2.7-3.2 1.2M3.2 8.5c0-3 2-4.7 4.8-4.7s4.8 1.7 4.8 4.7-2 5-4.8 5-4.8-2-4.8-5z"/>',
    macos: '<path d="M10.8 8.3c0-1.8 1.4-2.7 1.5-2.8-.8-1.2-2.1-1.4-2.6-1.4-1.1-.1-2.1.6-2.7.6-.6 0-1.5-.6-2.4-.6C2.6 4.2.5 6 .5 9c0 1.8.7 3.8 1.6 5 .8 1.1 1.7 2.2 2.9 2.1 1.1 0 1.6-.7 3-.7s1.8.7 3 .7c1.2 0 2-1.1 2.7-2.2.9-1.3 1.3-2.6 1.3-2.7-.1 0-2.2-.8-2.2-2.9zM9 3c.7-.8 1.1-1.9 1-3-1 .1-2 .7-2.7 1.5-.6.7-1.1 1.8-1 2.8C7.3 4.4 8.3 3.8 9 3z"/>',
    windows: '<path d="m2 3.5 5.2-.7v4.8H2zM8.3 2.7 14 2v5.6H8.3zM2 8.6h5.2v4.8L2 12.7zM8.3 8.6H14V14l-5.7-.7z"/>',
  };

  function resourceIcon(kind, item) {
    if (kind === "project" && item.iconData) {
      return `<img src="${escapeHTML(item.iconData)}" alt="">`;
    }
    const name = item.icon || (kind === "project" ? "code" : "server");
    const drawing = iconDrawings[name] || iconDrawings[kind === "project" ? "code" : "server"];
    return `<svg aria-hidden="true" viewBox="0 0 16 16">${drawing}</svg>`;
  }

  function renderResourceGroups(container, items, kind) {
    const filtering = Boolean(els.homeSearch.value.trim());
    container.innerHTML = groupedResources(items).map(([group, values]) => `
      <section class="resource-group" data-collapsed="${!filtering && state.collapsedGroups[kind].has(group)}">
        <button class="resource-group-title" type="button" data-toggle-group="${escapeHTML(kind)}" data-group-name="${escapeHTML(group)}">
          <svg class="resource-group-chevron" aria-hidden="true" viewBox="0 0 12 12"><path d="m4 2.5 4 3.5-4 3.5"/></svg>
          <svg aria-hidden="true" viewBox="0 0 16 16"><path d="M2.5 4.5h4l1.2 1.5h5.8v6.5h-11z"/></svg>
          <span>${escapeHTML(group)}</span>
          <small>${values.length}</small>
        </button>
        <div class="resource-list">
          ${values.map((item) => {
            const opening = state.openingResource === `${kind}:${item.id}`;
            const detail = kind === "project"
              ? `${item.serverId ? `${state.catalog.servers.find((server) => server.id === item.serverId)?.name || item.serverId} · ` : ""}${item.path}`
              : `${item.user}@${item.host}${item.port && item.port !== 22 ? `:${item.port}` : ""}`;
            const secret = kind === "server" && item.passwordStored ? '<span class="resource-secret">Keychain</span>' : "";
            return `<div class="resource-item" role="button" tabindex="0" data-opening="${opening}" data-open-${kind}="${escapeHTML(item.id)}">
              <span class="resource-icon" title="${escapeHTML(item.icon || (kind === "project" ? "code" : "server"))}">${resourceIcon(kind, item)}</span>
              <span class="resource-copy"><strong>${escapeHTML(item.name)}</strong><small>${escapeHTML(detail)}${secret}</small></span>
              <button class="resource-edit" type="button" data-edit-${kind}="${escapeHTML(item.id)}" aria-label="Edit ${escapeHTML(item.name)}">
                <svg aria-hidden="true" viewBox="0 0 16 16"><path d="m4 11 7-7 1 1-7 7-2 .5z"/></svg>
              </button>
            </div>`;
          }).join("")}
        </div>
      </section>`).join("");
  }

  function renderHome() {
    chooseGreeting();
    const query = els.homeSearch.value.trim().toLocaleLowerCase();
    const matches = (item) => !query || Object.values(item).some((value) => String(value ?? "").toLocaleLowerCase().includes(query));
    const allProjects = state.catalog.projects || [];
    const allServers = state.catalog.servers || [];
    const projects = allProjects.filter(matches);
    const servers = allServers.filter(matches);
    els.projectsCount.textContent = query ? `${projects.length}/${allProjects.length}` : String(projects.length);
    els.serversCount.textContent = query ? `${servers.length}/${allServers.length}` : String(servers.length);
    els.projectsEmpty.hidden = projects.length > 0;
    els.serversEmpty.hidden = servers.length > 0;
    els.projectsEmpty.querySelector("span").textContent = query ? "No matching projects" : "No projects yet";
    els.serversEmpty.querySelector("span").textContent = query ? "No matching servers" : "No servers yet";
    renderResourceGroups(els.projectGroups, projects, "project");
    renderResourceGroups(els.serverGroups, servers, "server");
    els.projectServer.innerHTML = '<option value="">This Mac</option>' + servers
      .map((server) => `<option value="${escapeHTML(server.id)}">${escapeHTML(server.name)}</option>`).join("");
  }

  function currentGreetingPeriod(date = new Date()) {
    const hour = date.getHours();
    if (hour >= 5 && hour < 12) return "morning";
    if (hour >= 12 && hour < 17) return "day";
    if (hour >= 17 && hour < 23) return "evening";
    return "night";
  }

  function chooseGreeting(force = false) {
    const period = currentGreetingPeriod();
    const key = `${period}Greetings`;
    const values = Array.isArray(state.settings[key]) && state.settings[key].length ? state.settings[key] : defaults[key];
    if (force || state.greetingPeriod !== period || !state.greetingText || !values.includes(state.greetingText)) {
      const candidates = values.length > 1 ? values.filter((value) => value !== state.greetingText) : values;
      state.greetingText = candidates[Math.floor(Math.random() * candidates.length)] || defaults[key][0];
      state.greetingPeriod = period;
    }
    els.homeGreeting.textContent = state.greetingText;
  }

  function showHomeNotice(message, persistent = false) {
    window.clearTimeout(state.homeNoticeTimer);
    els.homeNotice.textContent = message;
    els.homeNotice.hidden = false;
    if (!persistent) state.homeNoticeTimer = window.setTimeout(() => { els.homeNotice.hidden = true; }, 5000);
  }

  function showHome() {
    const current = activeTab();
    if (current) {
      current.editorDraft = els.editor.value;
      current.scrollTop = els.terminal.scrollTop;
      state.lastTabID = current.id;
    }
    state.homeActive = true;
    state.activeTabID = "";
    for (const tab of state.tabs.values()) {
      tab.pane.hidden = true;
      tab.element.setAttribute("aria-selected", "false");
      tab.element.tabIndex = -1;
    }
    clearSuggestions();
    hideInteractiveTerminal(false);
    document.body.dataset.home = "true";
    els.homeScreen.hidden = false;
    els.homeButton.setAttribute("aria-pressed", "true");
    updateBlockSelectionToolbar();
    chooseGreeting(true);
    renderHome();
  }

  function removeTab(tabID) {
    const tab = state.tabs.get(tabID);
    if (!tab) return;
    tab.element.remove();
    tab.pane.remove();
    state.tabs.delete(tabID);
  }

  function switchTab(tabID) {
    const next = state.tabs.get(tabID);
    if (!next) return;
    const current = activeTab();
    if (current) {
      current.editorDraft = els.editor.value;
      current.scrollTop = els.terminal.scrollTop;
    }
    state.activeTabID = tabID;
    state.lastTabID = tabID;
    state.homeActive = false;
    document.body.dataset.home = "false";
    els.homeScreen.hidden = true;
    els.homeButton.setAttribute("aria-pressed", "false");
    for (const tab of state.tabs.values()) {
      const selected = tab.id === tabID;
      tab.pane.hidden = !selected;
      tab.element.setAttribute("aria-selected", String(selected));
      tab.element.tabIndex = selected ? 0 : -1;
    }
    clearSuggestions();
    state.commands = new Set(next.commands || []);
    setEditor(next.editorDraft);
    els.cwd.textContent = displayPath(next);
    els.cwd.title = next.remote ? `${next.remote}:${next.cwd}` : next.cwd;
    renderGitContext(next.gitInfo);
    renderSSHStatus(next);
    refreshGitContext(tabID);
    refreshCommandInventory(tabID);
    setComposerRunning(next.running);
    requestAnimationFrame(() => { els.terminal.scrollTop = next.scrollTop; });
    refreshHistory(next);
    updateBlockSelectionToolbar(next);
    syncInteractiveTerminal(next);
    if (document.body.dataset.terminalMode !== "true") els.editor.focus();
  }

  async function newTab() {
    try {
      const meta = bridge()
        ? await bridge().NewTab("")
        : { id: `preview-${Date.now()}`, title: "~", cwd: state.home || "~", running: false };
      addTab(meta);
      switchTab(meta.id);
      refreshCommandInventory(meta.id);
    } catch (error) {
      showAppError(error);
    }
  }

  async function closeTab(tabID) {
    if (!tabID || state.closingTabs.has(tabID)) return;
    state.closingTabs.add(tabID);
    const closingTab = state.tabs.get(tabID);
    if (closingTab) closingTab.element.dataset.closing = "true";
    const wasActive = tabID === state.activeTabID;
    const order = [...state.tabs.keys()];
    const index = order.indexOf(tabID);
    try {
      if (bridge()) {
        const result = await bridge().CloseTab(tabID);
        removeTab(tabID);
        if (result.createdTab) addTab(result.createdTab);
        if (wasActive) {
          const nextID = result.activeId || [...state.tabs.keys()][0];
          if (nextID) switchTab(nextID);
          else showHome();
        }
      } else {
        removeTab(tabID);
        if (!state.tabs.size) showHome();
        else if (wasActive) switchTab(order[index + 1] || order[index - 1] || [...state.tabs.keys()][0]);
      }
    } catch (error) {
      showAppError(error);
    } finally {
      state.closingTabs.delete(tabID);
      const remaining = state.tabs.get(tabID);
      if (remaining) delete remaining.element.dataset.closing;
    }
  }

  function setResourceEditorBusy(busy) {
    state.resourceEditorBusy = busy;
    els.resourceEditorLayer.setAttribute("aria-busy", String(busy));
    els.resourceEditorLayer.querySelectorAll("button, input, select, textarea").forEach((control) => { control.disabled = busy; });
  }

  function closeResourceEditor(force = false) {
    if (els.resourceEditorLayer.hidden || (state.resourceEditorBusy && !force)) return;
    resetDeleteConfirmation();
    const sequence = ++state.resourceEditorAnimationSequence;
    els.resourceEditorLayer.dataset.closing = "true";
    const finish = () => {
      if (sequence !== state.resourceEditorAnimationSequence) return;
      els.resourceEditorLayer.hidden = true;
      delete els.resourceEditorLayer.dataset.closing;
      els.appShell.inert = false;
      const target = state.resourceEditorReturnFocus;
      state.resourceEditorReturnFocus = null;
      if (target?.isConnected) target.focus();
      else if (state.homeActive) els.homeSearch.focus();
      else els.editor.focus();
    };
    if (reducedMotion()) finish();
    else window.setTimeout(finish, 160);
  }

  function showResourceError(kind, error) {
    const element = kind === "project" ? els.projectFormError : els.serverFormError;
    element.textContent = cleanError(error);
    element.hidden = false;
  }

  function resetDeleteConfirmation() {
    window.clearTimeout(state.deleteConfirmationTimer);
    state.deleteConfirmationTimer = 0;
    for (const button of [els.deleteProject, els.deleteServer]) {
      delete button.dataset.confirming;
      button.textContent = "Delete";
    }
  }

  function confirmDeletion(button, action) {
    if (button.dataset.confirming === "true") {
      resetDeleteConfirmation();
      Promise.resolve(action()).catch(showAppError);
      return;
    }
    resetDeleteConfirmation();
    button.dataset.confirming = "true";
    button.textContent = "Confirm delete";
    state.deleteConfirmationTimer = window.setTimeout(resetDeleteConfirmation, 3500);
  }

  function openProjectEditor(project = null) {
    state.resourceEditorAnimationSequence += 1;
    state.resourceEditorReturnFocus = document.activeElement;
    setResourceEditorBusy(false);
    resetDeleteConfirmation();
    els.resourceEditorKind.textContent = "Project";
    els.resourceEditorTitle.textContent = project ? "Edit project" : "New project";
    els.projectForm.hidden = false;
    els.serverForm.hidden = true;
    els.projectID.value = project?.id || "";
    els.projectName.value = project?.name || "";
    els.projectGroup.value = project?.group || "";
    els.projectIcon.value = project?.icon || "code";
    els.projectIconPath.value = project?.iconPath || "";
    els.projectServer.value = project?.serverId || "";
    els.projectPath.value = project?.path || "";
    els.projectCommand.value = project?.startupCommand || "";
    els.projectFormError.hidden = true;
    els.deleteProject.hidden = !project;
    els.resourceEditorLayer.hidden = false;
    els.appShell.inert = true;
    delete els.resourceEditorLayer.dataset.closing;
    requestAnimationFrame(() => els.projectName.focus());
  }

  function openServerEditor(server = null) {
    state.resourceEditorAnimationSequence += 1;
    state.resourceEditorReturnFocus = document.activeElement;
    setResourceEditorBusy(false);
    resetDeleteConfirmation();
    els.resourceEditorKind.textContent = "SSH server";
    els.resourceEditorTitle.textContent = server ? "Edit server" : "New server";
    els.projectForm.hidden = true;
    els.serverForm.hidden = false;
    els.serverID.value = server?.id || "";
    els.serverName.value = server?.name || "";
    els.serverGroup.value = server?.group || "";
    els.serverHost.value = server?.host || "";
    els.serverPort.value = server?.port || 22;
    els.serverUser.value = server?.user || "";
    els.serverKey.value = server?.keyPath || "";
    els.serverPassword.value = "";
    els.serverCommand.value = server?.loginCommand || "";
    els.serverFormError.hidden = true;
    els.deleteServer.hidden = !server;
    els.clearPasswordRow.hidden = !server?.passwordStored;
    els.clearPassword.checked = false;
    els.serverPasswordState.textContent = server?.passwordStored
      ? "saved in Keychain · leave empty to keep"
      : "optional · stored in Keychain";
    els.resourceEditorLayer.hidden = false;
    els.appShell.inert = true;
    delete els.resourceEditorLayer.dataset.closing;
    requestAnimationFrame(() => els.serverName.focus());
  }

  async function saveProject(event) {
    event.preventDefault();
    if (!bridge() || state.resourceEditorBusy) return;
    setResourceEditorBusy(true);
    try {
      state.catalog = await bridge().SaveProject({
        id: els.projectID.value,
        name: els.projectName.value,
        group: els.projectGroup.value,
        icon: els.projectIcon.value,
        iconPath: els.projectIconPath.value,
        path: els.projectPath.value,
        serverId: els.projectServer.value,
        startupCommand: els.projectCommand.value,
      });
      renderHome();
      closeResourceEditor(true);
    } catch (error) {
      showResourceError("project", error);
    } finally {
      setResourceEditorBusy(false);
    }
  }

  async function saveServer(event) {
    event.preventDefault();
    if (!bridge() || state.resourceEditorBusy) return;
    setResourceEditorBusy(true);
    try {
      state.catalog = await bridge().SaveServer({
        id: els.serverID.value,
        name: els.serverName.value,
        group: els.serverGroup.value,
        host: els.serverHost.value,
        port: Number(els.serverPort.value) || 22,
        user: els.serverUser.value,
        keyPath: els.serverKey.value,
        loginCommand: els.serverCommand.value,
        password: els.serverPassword.value,
        clearPassword: els.clearPassword.checked,
      });
      renderHome();
      closeResourceEditor(true);
    } catch (error) {
      showResourceError("server", error);
    } finally {
      setResourceEditorBusy(false);
    }
  }

  async function deleteCurrentProject() {
    const id = els.projectID.value;
    if (!id || !bridge() || state.resourceEditorBusy) return;
    setResourceEditorBusy(true);
    try {
      state.catalog = await bridge().DeleteProject(id);
      renderHome();
      closeResourceEditor(true);
    } catch (error) {
      showResourceError("project", error);
    } finally {
      setResourceEditorBusy(false);
    }
  }

  async function deleteCurrentServer() {
    const id = els.serverID.value;
    if (!id || !bridge() || state.resourceEditorBusy) return;
    setResourceEditorBusy(true);
    try {
      state.catalog = await bridge().DeleteServer(id);
      renderHome();
      closeResourceEditor(true);
    } catch (error) {
      showResourceError("server", error);
    } finally {
      setResourceEditorBusy(false);
    }
  }

  async function openResource(kind, id) {
    if (!bridge() || state.openingResource) return;
    state.openingResource = `${kind}:${id}`;
    renderHome();
    try {
      const launch = kind === "project" ? await bridge().OpenProject(id) : await bridge().OpenServer(id);
      if (launch.catalog) state.catalog = launch.catalog;
      const tab = addTab(launch.tab);
      tab.fixedTitle = launch.tab.title || "";
      tab.launchQueue = Array.isArray(launch.commands) ? [...launch.commands] : [];
      switchTab(tab.id);
      if (launch.warning) showAppError(launch.warning);
      runNextLaunchCommand(tab);
    } catch (error) {
      showHomeNotice(cleanError(error));
    } finally {
      state.openingResource = "";
      if (state.homeActive) renderHome();
    }
  }

  function refreshGhost() {
    const selected = state.suggestions[state.selectedSuggestion];
    const input = els.editor.value;
    if (!selected || !selected.value.startsWith(input) || selected.value === input || els.editor.selectionEnd !== input.length) {
      els.ghost.textContent = "";
      return;
    }
    els.ghost.innerHTML = `<span class="typed">${escapeHTML(input)}</span>${escapeHTML(selected.value.slice(input.length))}`;
  }

  function renderSuggestions(localItems, preferAI = false) {
    const previousValue = state.suggestions[state.selectedSuggestion]?.value;
    if (Array.isArray(localItems)) state.localSuggestions = localItems;
    const history = state.localSuggestions.filter((item) => item.source === "history");
    const local = state.localSuggestions.filter((item) => item.source !== "history");
    const ai = state.aiSuggestion && !state.localSuggestions.some((item) => item.value === state.aiSuggestion.value)
      ? [state.aiSuggestion]
      : [];
    // Local completion paints the first frame. Once an AI continuation is
    // ready it becomes the active ghost, while deterministic alternatives
    // stay directly below it and remain reachable with the arrow keys.
    state.suggestions = ai.length
      ? [...ai, ...history, ...local].slice(0, 8)
      : [...history, ...local].slice(0, 8);
    const preservedIndex = state.suggestions.findIndex((item) => item.value === previousValue);
    state.selectedSuggestion = preferAI && ai.length ? 0 : (preservedIndex >= 0 ? preservedIndex : 0);
    if (!state.suggestions.length || !els.editor.value) {
      els.suggestions.hidden = true;
      els.suggestions.replaceChildren();
      refreshGhost();
      return;
    }
    els.suggestions.hidden = false;
    els.suggestions.innerHTML = state.suggestions.map((item, index) => `
      <button class="suggestion" role="option" data-index="${index}" aria-selected="${index === state.selectedSuggestion}">
        <span class="suggestion-main">${escapeHTML(item.label || item.value)}</span>
        <span class="suggestion-meta">${escapeHTML(item.description || "")}<span class="suggestion-source ${item.source === "ai" ? "ai" : ""}">${escapeHTML(item.source || "local")}</span></span>
      </button>`).join("");
    refreshGhost();
  }

  function clearSuggestions() {
    state.localRequestSequence += 1;
    state.aiRequestSequence += 1;
    state.localSuggestions = [];
    state.aiSuggestion = null;
    requestAIPrediction.cancel();
    renderSuggestions();
  }

  function selectSuggestion(index) {
    if (!state.suggestions.length) return;
    state.selectedSuggestion = (index + state.suggestions.length) % state.suggestions.length;
    els.suggestions.querySelectorAll(".suggestion").forEach((node, i) => {
      node.setAttribute("aria-selected", String(i === state.selectedSuggestion));
      if (i === state.selectedSuggestion) node.scrollIntoView({ block: "nearest" });
    });
    refreshGhost();
  }

  function acceptSuggestion() {
    const selected = state.suggestions[state.selectedSuggestion];
    if (!selected) return false;
    setEditor(selected.value);
    clearSuggestions();
    if (selected.source === "files" && selected.value.endsWith("/")) requestSuggestions();
    return true;
  }

  function acceptSuggestionWord() {
    const selected = state.suggestions[state.selectedSuggestion];
    const input = els.editor.value;
    if (!selected?.value.startsWith(input) || selected.value === input) return false;
    const remainder = selected.value.slice(input.length);
    const chunk = remainder.match(/^\s*\S+(?:\s+|\/)?/)?.[0] || remainder[0];
    setEditor(input + chunk);
    updateEditorState();
    return true;
  }

  function optimisticLocalSuggestions(input) {
    return state.localSuggestions.filter((item) => item.value !== input && item.value.startsWith(input));
  }

  function shouldRequestAI(input) {
    if (!state.settings.aiEnabled || !bridge()) return false;
    const trimmed = String(input || "").trim();
    if (trimmed.length < 4 || !/[\s]/.test(trimmed)) return false;
    const lastToken = trimmed.match(/\S+$/)?.[0] || "";
    return !lastToken.startsWith("-");
  }

  async function requestLocalSuggestions() {
    const tab = activeTab();
    const input = els.editor.value;
    const sequence = ++state.localRequestSequence;
    if (!tab || !input.trim() || !bridge()) {
      state.localSuggestions = [];
      renderSuggestions();
      return;
    }
    try {
      const items = await bridge().Suggest(tab.id, input);
      if (sequence === state.localRequestSequence && tab.id === state.activeTabID && input === els.editor.value) {
        renderSuggestions(items);
        if (shouldRequestAI(input)) requestAIPrediction(tab.id, input);
      }
    } catch (_) {
      if (sequence === state.localRequestSequence) {
        state.localSuggestions = [];
        renderSuggestions();
      }
    }
  }

  const requestAIPrediction = debounce(async (tabID, expectedInput) => {
    const tab = activeTab();
    const input = expectedInput;
    const sequence = ++state.aiRequestSequence;
    if (!tab || tab.id !== tabID || input !== els.editor.value || !shouldRequestAI(input)) return;
    try {
      const candidates = state.localSuggestions.slice(0, 8).map((item) => item.value);
      const items = await bridge().Predict(tab.id, input, candidates);
      if (sequence !== state.aiRequestSequence || tab.id !== state.activeTabID || input !== els.editor.value) return;
      state.aiSuggestion = Array.isArray(items) ? items[0] || null : null;
      renderSuggestions(undefined, true);
    } catch (_) {
      if (sequence === state.aiRequestSequence) {
        state.aiSuggestion = null;
        renderSuggestions();
      }
    }
  }, 320);

  function requestSuggestions() {
    const input = els.editor.value;
    state.localSuggestions = optimisticLocalSuggestions(input);
    state.aiSuggestion = null;
    state.aiRequestSequence += 1;
    requestAIPrediction.cancel();
    renderSuggestions();
    requestLocalSuggestions();
  }

  function updateEditorState() {
    const tab = activeTab();
    if (tab) tab.editorDraft = els.editor.value;
    autoSize();
    refreshSyntax();
    refreshGhost();
    requestSuggestions();
    if (tab) tab.historyIndex = -1;
  }

  function scrollToBottom(force = false) {
    const nearBottom = els.terminal.scrollHeight - els.terminal.scrollTop - els.terminal.clientHeight < 130;
    if (force || nearBottom) requestAnimationFrame(() => { els.terminal.scrollTop = els.terminal.scrollHeight; });
  }

  function createBlock(block) {
    const tab = state.tabs.get(block.tabId) || activeTab();
    if (!tab) return;
    const node = document.createElement("article");
    node.className = "block";
    node.dataset.id = block.id;
    node.dataset.state = block.state;
    node.innerHTML = `
      <div class="block-head">
        <pre class="block-command">${renderShellSyntax(block.command, state.commands, true)}</pre>
        <span class="block-trailing">
          <button class="block-select" data-select-block="${escapeHTML(block.id)}" type="button" aria-pressed="false" aria-label="Select block" title="Select block">
            <svg aria-hidden="true" viewBox="0 0 16 16"><path d="m4 8 2.5 2.5L12 5"/></svg>
          </button>
          <span class="block-status"><span class="status-icon"></span><span class="status-text">running</span></span>
          <button class="block-copy" data-copy-block="${escapeHTML(block.id)}" aria-label="Copy command and output" title="Copy command and output">
            <svg aria-hidden="true" viewBox="0 0 16 16"><rect x="5" y="5" width="8" height="8" rx="2"></rect><path d="M3 10V4.5C3 3.7 3.7 3 4.5 3H10"></path></svg>
          </button>
        </span>
      </div>
      <pre class="block-output"></pre>
      <div class="block-meta" hidden><span class="block-cwd">${escapeHTML(shortPath(block.cwd))}</span><span class="block-result"></span></div>`;
    tab.pane.appendChild(node);
    const record = {
      data: block,
      node,
      output: "",
      rawOutput: "",
      escapeTail: "",
      interactive: prefersFullscreen(block.command),
      outputRenderScheduled: false,
      outputTruncated: false,
    };
    tab.blocks.set(block.id, record);
    if (tab.id === state.activeTabID) prepareTerminalRecord(tab, record);
    if (record.interactive) {
      node.classList.add("is-interactive");
      if (tab.id === state.activeTabID) showInteractiveTerminal(tab, record, false);
    }
    if (tab.id === state.activeTabID) scrollToBottom(true);
  }

  const ansiPalette = [
    "var(--ansi-black)", "var(--ansi-red)", "var(--ansi-green)", "var(--ansi-yellow)",
    "var(--ansi-blue)", "var(--ansi-magenta)", "var(--ansi-cyan)", "var(--ansi-white)",
    "var(--ansi-bright-black)", "var(--ansi-bright-red)", "var(--ansi-bright-green)", "var(--ansi-bright-yellow)",
    "var(--ansi-bright-blue)", "var(--ansi-bright-magenta)", "var(--ansi-bright-cyan)", "var(--ansi-bright-white)",
  ];
  const ansiCubeLevels = [0, 95, 135, 175, 215, 255];

  function xtermColor(index) {
    const color = Math.max(0, Math.min(255, Number(index) || 0));
    if (color < 16) return ansiPalette[color];
    if (color < 232) {
      const offset = color - 16;
      const red = ansiCubeLevels[Math.floor(offset / 36)];
      const green = ansiCubeLevels[Math.floor((offset % 36) / 6)];
      const blue = ansiCubeLevels[offset % 6];
      return `rgb(${red} ${green} ${blue})`;
    }
    const gray = 8 + (color - 232) * 10;
    return `rgb(${gray} ${gray} ${gray})`;
  }

  function ansiRGB(red, green, blue) {
    const channel = (value) => Math.max(0, Math.min(255, Number(value) || 0));
    return `rgb(${channel(red)} ${channel(green)} ${channel(blue)})`;
  }

  function freshANSIState() {
    return {
      foreground: "", background: "", underlineColor: "", bold: false, dim: false, italic: false,
      underline: false, blink: false, inverse: false, hidden: false, strike: false,
    };
  }

  function applyExtendedANSIColor(codes, index, target, ansiState) {
    if (codes[index + 1] === 5 && codes.length > index + 2) {
      ansiState[target] = xtermColor(codes[index + 2]);
      return index + 2;
    }
    if (codes[index + 1] === 2 && codes.length > index + 4) {
      ansiState[target] = ansiRGB(codes[index + 2], codes[index + 3], codes[index + 4]);
      return index + 4;
    }
    return index;
  }

  function applySGR(parameters, ansiState) {
    const codes = [];
    for (const parameter of (parameters === "" ? ["0"] : parameters.split(";"))) {
      if (!parameter.includes(":")) {
        codes.push(Number(parameter || 0));
        continue;
      }
      const parts = parameter.split(":");
      const code = Number(parts[0] || 0);
      const mode = Number(parts[1] || 0);
      if ([38, 48, 58].includes(code) && mode === 2 && parts.length >= 5) {
        codes.push(code, mode, ...parts.slice(-3).map(Number));
      } else if ([38, 48, 58].includes(code) && mode === 5 && parts.length >= 3) {
        codes.push(code, mode, Number(parts.at(-1)));
      } else {
        codes.push(...parts.filter((value) => value !== "").map(Number));
      }
    }
    if (!codes.length) codes.push(0);
    for (let index = 0; index < codes.length; index += 1) {
      const code = Number.isFinite(codes[index]) ? codes[index] : 0;
      if (code === 0) Object.assign(ansiState, freshANSIState());
      else if (code === 1) ansiState.bold = true;
      else if (code === 2) ansiState.dim = true;
      else if (code === 3) ansiState.italic = true;
      else if (code === 4 || code === 21) ansiState.underline = true;
      else if (code === 5 || code === 6) ansiState.blink = true;
      else if (code === 7) ansiState.inverse = true;
      else if (code === 8) ansiState.hidden = true;
      else if (code === 9) ansiState.strike = true;
      else if (code === 22) { ansiState.bold = false; ansiState.dim = false; }
      else if (code === 23) ansiState.italic = false;
      else if (code === 24) ansiState.underline = false;
      else if (code === 25) ansiState.blink = false;
      else if (code === 27) ansiState.inverse = false;
      else if (code === 28) ansiState.hidden = false;
      else if (code === 29) ansiState.strike = false;
      else if (code >= 30 && code <= 37) ansiState.foreground = ansiPalette[code - 30];
      else if (code >= 90 && code <= 97) ansiState.foreground = ansiPalette[8 + code - 90];
      else if (code === 38) index = applyExtendedANSIColor(codes, index, "foreground", ansiState);
      else if (code === 39) ansiState.foreground = "";
      else if (code >= 40 && code <= 47) ansiState.background = ansiPalette[code - 40];
      else if (code >= 100 && code <= 107) ansiState.background = ansiPalette[8 + code - 100];
      else if (code === 48) index = applyExtendedANSIColor(codes, index, "background", ansiState);
      else if (code === 49) ansiState.background = "";
      else if (code === 58) index = applyExtendedANSIColor(codes, index, "underlineColor", ansiState);
      else if (code === 59) ansiState.underlineColor = "";
    }
  }

  function ansiStateStyle(ansiState) {
    let foreground = ansiState.foreground;
    let background = ansiState.background;
    if (ansiState.inverse) {
      [foreground, background] = [background || "var(--text)", foreground || "var(--canvas)"];
    }
    const styles = [];
    if (foreground) styles.push(`color:${foreground}`);
    if (background) styles.push(`background-color:${background}`);
    if (ansiState.bold) styles.push("font-weight:700");
    if (ansiState.dim) styles.push("opacity:.62");
    if (ansiState.italic) styles.push("font-style:italic");
    const decorations = [];
    if (ansiState.underline) decorations.push("underline");
    if (ansiState.strike) decorations.push("line-through");
    if (decorations.length) styles.push(`text-decoration-line:${decorations.join(" ")}`);
    if (ansiState.underlineColor) styles.push(`text-decoration-color:${ansiState.underlineColor}`);
    if (ansiState.hidden) styles.push("visibility:hidden");
    return styles.join(";");
  }

  function wrapANSI(text, ansiState) {
    if (!text) return "";
    const style = ansiStateStyle(ansiState);
    return style ? `<span class="ansi-run" style="${style}">${escapeHTML(text)}</span>` : escapeHTML(text);
  }

  function renderANSI(raw) {
    const value = String(raw || "");
    const ansiState = freshANSIState();
    let result = "";
    let textStart = 0;
    let index = 0;
    while (index < value.length) {
      if (value.charCodeAt(index) !== 27) { index += 1; continue; }
      result += wrapANSI(value.slice(textStart, index), ansiState);
      if (value[index + 1] === "[") {
        let end = index + 2;
        while (end < value.length && !(value.charCodeAt(end) >= 0x40 && value.charCodeAt(end) <= 0x7e)) end += 1;
        if (end >= value.length) break;
        if (value[end] === "m") applySGR(value.slice(index + 2, end).replace(/^\?/, ""), ansiState);
        index = end + 1;
      } else if (value[index + 1] === "]") {
        let end = index + 2;
        while (end < value.length && value.charCodeAt(end) !== 7 && !(value.charCodeAt(end) === 27 && value[end + 1] === "\\")) end += 1;
        if (end >= value.length) break;
        index = value.charCodeAt(end) === 7 ? end + 1 : end + 2;
      } else {
        index += Math.min(2, value.length - index);
      }
      textStart = index;
    }
    if (index >= value.length) result += wrapANSI(value.slice(textStart), ansiState);
    return result;
  }

  function plainTerminalText(value) {
    return String(value || "")
      .replace(/\x1b\][^\x07]*(?:\x07|\x1b\\|$)/g, "")
      .replace(/\x1b\[[0-?]*[ -\/]*[@-~]/g, "")
      .replace(/\r/g, "");
  }

  function blockClipboardText(record, includeOutput = true) {
    if (!record) return "";
    const output = includeOutput ? plainTerminalText(record.output).replace(/\n+$/, "") : "";
    return output ? `${record.data.command}\n${output}` : record.data.command;
  }

  async function writeClipboardText(text) {
    if (window.runtime?.ClipboardSetText) await window.runtime.ClipboardSetText(text);
    else await navigator.clipboard.writeText(text);
  }

  async function copyBlock(blockID, button) {
    const tab = activeTab();
    const record = tab?.blocks.get(blockID);
    if (!record) return;
    try {
      await writeClipboardText(blockClipboardText(record));
      button.dataset.copied = "true";
      button.title = "Copied";
      window.setTimeout(() => {
        button.dataset.copied = "false";
        button.title = "Copy command and output";
      }, 1100);
    } catch (_) {
      button.title = "Could not copy";
    }
  }

  function selectedBlockRecords(tab = activeTab()) {
    if (!tab) return [];
    return [...tab.blocks.values()].filter((record) => tab.selectedBlocks.has(record.data.id));
  }

  function updateBlockSelectionToolbar(tab = activeTab()) {
    const selected = tab && !state.homeActive ? selectedBlockRecords(tab) : [];
    if (tab) {
      for (const record of tab.blocks.values()) {
        const active = tab.selectedBlocks.has(record.data.id);
        record.node.classList.toggle("is-selected", active);
        const button = record.node.querySelector("[data-select-block]");
        button?.setAttribute("aria-pressed", String(active));
        if (button) button.title = active ? "Deselect block" : "Select block";
      }
    }
    els.selectionToolbar.hidden = selected.length === 0;
    els.selectionCount.textContent = `${selected.length} ${selected.length === 1 ? "block" : "blocks"}`;
  }

  function toggleBlockSelection(blockID, range = false) {
    const tab = activeTab();
    if (!tab?.blocks.has(blockID)) return;
    if (range && tab.selectionAnchor && tab.blocks.has(tab.selectionAnchor)) {
      const ids = [...tab.blocks.keys()];
      const start = ids.indexOf(tab.selectionAnchor);
      const end = ids.indexOf(blockID);
      for (let index = Math.min(start, end); index <= Math.max(start, end); index += 1) {
        tab.selectedBlocks.add(ids[index]);
      }
    } else if (tab.selectedBlocks.has(blockID)) {
      tab.selectedBlocks.delete(blockID);
    } else {
      tab.selectedBlocks.add(blockID);
    }
    tab.selectionAnchor = blockID;
    updateBlockSelectionToolbar(tab);
  }

  function clearBlockSelection(tab = activeTab()) {
    if (!tab) return;
    tab.selectedBlocks.clear();
    tab.selectionAnchor = "";
    updateBlockSelectionToolbar(tab);
  }

  async function copySelectedBlocks(includeOutput, button) {
    const records = selectedBlockRecords();
    if (!records.length) return;
    const text = records.map((record) => blockClipboardText(record, includeOutput)).join(includeOutput ? "\n\n" : "\n");
    const previous = button.textContent;
    try {
      await writeClipboardText(text);
      button.dataset.copied = "true";
      button.textContent = "Copied";
    } catch (_) {
      button.textContent = "Could not copy";
    }
    window.setTimeout(() => {
      button.dataset.copied = "false";
      button.textContent = previous;
    }, 1100);
  }

  const lsKinds = [
    ["ls-archive", new Set(["7z", "bz2", "dmg", "gz", "pkg", "rar", "tar", "tbz", "tgz", "xz", "zip", "zst"])],
    ["ls-media", new Set(["avif", "gif", "heic", "jpeg", "jpg", "m4a", "mov", "mp3", "mp4", "png", "svg", "webp", "wav"])],
    ["ls-source", new Set(["c", "cpp", "css", "go", "h", "hpp", "html", "java", "js", "jsx", "kt", "m", "md", "py", "rb", "rs", "sh", "swift", "ts", "tsx", "vue"])],
    ["ls-config", new Set(["ini", "json", "plist", "toml", "xml", "yaml", "yml"])],
    ["ls-document", new Set(["doc", "docx", "pdf", "ppt", "pptx", "rtf", "txt", "xls", "xlsx"])],
  ];

  function isLsCommand(command) {
    return /^(?:(?:command|sudo)\s+)?(?:\/[^\s]+\/)?ls(?:\s|$)/.test(String(command || "").trim());
  }

  function lsKindForName(value) {
    const name = value.replace(/[|=>@*]+$/, "");
    if (value.endsWith("/")) return "ls-directory";
    if (value.endsWith("@")) return "ls-symlink";
    if (value.endsWith("*")) return "ls-executable";
    const extension = name.includes(".") ? name.split(".").pop().toLowerCase() : "";
    return lsKinds.find(([, extensions]) => extensions.has(extension))?.[0] || "";
  }

  function highlightLsOutput(output, command) {
    if (!isLsCommand(command)) return;
    const walker = document.createTreeWalker(output, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    for (const node of nodes) {
      if (!node.nodeValue?.trim() || node.parentElement?.closest('.ansi-run, [class*="ansi-"], .ls-kind')) continue;
      const parts = node.nodeValue.split(/(\s+)/);
      if (!parts.some((part) => lsKindForName(part))) continue;
      const fragment = document.createDocumentFragment();
      for (const part of parts) {
        const kind = lsKindForName(part);
        if (!kind) {
          fragment.append(document.createTextNode(part));
          continue;
        }
        const span = document.createElement("span");
        span.className = `ls-kind ${kind}`;
        span.textContent = part;
        fragment.append(span);
      }
      node.replaceWith(fragment);
    }
  }

  function appendOutput(chunk) {
    const tab = state.tabs.get(chunk.tabId);
    const record = tab?.blocks.get(chunk.blockId);
    if (!record) return;
    const output = record.node.querySelector(".block-output");
    record.rawOutput += chunk.data;
    if (record.rawOutput.length > 4 * 1024 * 1024) record.rawOutput = record.rawOutput.slice(-4 * 1024 * 1024);
    const probe = record.escapeTail + chunk.data;
    record.escapeTail = probe.slice(-48);
    const enteredAlternateScreen = /\x1b\[\?(?:47|1047|1049)h/.test(probe);
    if (tab.id === state.activeTabID && state.terminalBlockID === record.data.id) {
      state.vtTerminal?.write(chunk.data);
    }
    if (enteredAlternateScreen && !record.interactive) {
      record.interactive = true;
      record.output = "";
      output.replaceChildren();
      output.hidden = true;
      record.node.classList.add("is-interactive");
      if (tab.id === state.activeTabID) {
        showInteractiveTerminal(tab, record, false);
      }
    }
    if (record.interactive) return;
    appendBlockText(record, chunk.data);
    scheduleBlockOutputRender(record);
    if (chunk.stream === "stderr") output.dataset.hasStderr = "true";
    if (tab.id === state.activeTabID) scrollToBottom();
  }

  function appendBlockText(record, value) {
    value = String(value || "").replace(/\r\n/g, "\n");
    if (!/[\r\b]/.test(value)) {
      record.output += value;
      limitBlockOutput(record);
      return;
    }
    for (let index = 0; index < value.length; index += 1) {
      const character = value[index];
      if (character === "\r") {
        const lineStart = record.output.lastIndexOf("\n") + 1;
        record.output = record.output.slice(0, lineStart);
        continue;
      }
      if (character === "\b") {
        const lineStart = record.output.lastIndexOf("\n") + 1;
        if (record.output.length > lineStart) record.output = record.output.slice(0, -1);
        continue;
      }
      record.output += character;
    }
    limitBlockOutput(record);
  }

  function limitBlockOutput(record) {
    if (record.output.length > maxBlockOutput) {
      record.output = outputTruncatedMarker + record.output.slice(-(maxBlockOutput - outputTruncatedMarker.length));
      record.outputTruncated = true;
    }
  }

  function renderBlockOutput(record) {
    record.outputRenderScheduled = false;
    if (record.interactive || !record.node.isConnected) return;
    const output = record.node.querySelector(".block-output");
    output.innerHTML = renderANSI(record.output);
    highlightLsOutput(output, record.data.command);
    if (record.output) output.dataset.visible = "true";
  }

  function scheduleBlockOutputRender(record) {
    if (record.outputRenderScheduled) return;
    record.outputRenderScheduled = true;
    requestAnimationFrame(() => renderBlockOutput(record));
  }

  function formatDuration(ms) {
    if (ms < 1000) return `${ms} ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)} s`;
    return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`;
  }

  function finishBlock(payload) {
    const block = payload.block || payload.Block || payload;
    const tab = state.tabs.get(block.tabId);
    const record = tab?.blocks.get(block.id);
    if (!tab || !record) return;
    record.data = block;
    if (!record.interactive) renderBlockOutput(record);
    record.node.dataset.state = block.state;
    record.node.querySelector(".status-text").textContent = block.state === "succeeded" ? "done" : block.state;
    const meta = record.node.querySelector(".block-meta");
    meta.hidden = false;
    record.node.querySelector(".block-cwd").textContent = shortPath(block.finalCwd || block.cwd);
    record.node.querySelector(".block-result").textContent = formatDuration(block.durationMs || 0);
    tab.running = false;
    tab.runningID = "";
    tab.cwd = block.finalCwd || tab.cwd;
    tab.remote = block.remote || "";
    tab.title = tab.fixedTitle || tab.remote || titleForPath(tab.cwd);
    renderTab(tab);
    refreshGitContext(tab.id);
    if (record.interactive) {
      record.node.classList.add("interactive-finished");
      record.rawOutput = "";
      if (tab.id === state.activeTabID && state.terminalBlockID === block.id) hideInteractiveTerminal(false);
    } else if (tab.id === state.activeTabID && state.terminalBlockID === block.id) {
      state.terminalBlockID = "";
    }
    if (tab.id === state.activeTabID) {
      els.cwd.textContent = displayPath(tab);
      els.cwd.title = tab.remote ? `${tab.remote}:${tab.cwd}` : tab.cwd;
      renderSSHStatus(tab);
      setComposerRunning(false);
      els.editor.focus();
      refreshHistory(tab);
      scrollToBottom(true);
    }
    refreshCommandInventory(tab.id);
    if (block.state === "succeeded" && tab.launchQueue.length) {
      window.setTimeout(() => runNextLaunchCommand(tab), 0);
    } else if (block.state !== "succeeded") {
      tab.launchQueue = [];
    }
  }

  async function executeCommand(tab, command, clearComposer = false) {
    command = String(command || "").trim();
    if (!tab || !command || tab.running || !bridge()) return;
    const isActive = tab.id === state.activeTabID;
    if (command === "clear" && isActive) {
      clearBlocks();
      if (clearComposer) setEditor("");
      tab.editorDraft = "";
      return;
    }
    if (isActive) clearSuggestions();
    try {
      const block = await bridge().Execute(tab.id, command);
      tab.running = true;
      tab.runningID = block.id;
      tab.history.push(command);
      tab.editorDraft = "";
      renderTab(tab);
      createBlock(block);
      if (isActive && clearComposer) setEditor("");
      if (isActive) setComposerRunning(true);
      const record = tab.blocks.get(block.id);
      if (isActive) await prepareInteractiveRenderer(tab, record);
      if (isActive && state.vtTerminal && state.fitAddon) {
        try {
          state.fitAddon.fit();
          await bridge().ResizeTerminal(tab.id, state.vtTerminal.cols, state.vtTerminal.rows);
        } catch (_) { /* the PTY keeps its safe default size */ }
      }
      await bridge().StartBlock(tab.id, block.id);
      return block;
    } catch (error) {
      if (tab.runningID && bridge()) await bridge().Cancel(tab.id, tab.runningID).catch(() => {});
      showLocalError(cleanError(error), tab, command);
      tab.launchQueue = [];
      return null;
    }
  }

  async function runNextLaunchCommand(tab) {
    if (!tab || tab.running || !tab.launchQueue.length) return;
    if (state.activeTabID !== tab.id) switchTab(tab.id);
    const command = tab.launchQueue.shift();
    await executeCommand(tab, command, false);
  }

  async function execute() {
    const tab = activeTab();
    const command = els.editor.value.trim();
    await executeCommand(tab, command, true);
  }

  function showLocalError(message, tab = activeTab(), command = "NTerm") {
    if (!tab) return;
    const id = `local-${Date.now()}-${++state.localErrorSequence}`;
    createBlock({ id, tabId: tab.id, command: command || "NTerm", cwd: tab.cwd, state: "failed" });
    appendOutput({ tabId: tab.id, blockId: id, stream: "stderr", data: message + "\n" });
    finishBlock({ id, tabId: tab.id, state: "failed", cwd: tab.cwd, exitCode: 1, durationMs: 0 });
  }

  function showAppError(error) {
    const message = cleanError(error);
    if (state.homeActive || !activeTab()) showHomeNotice(message);
    else showLocalError(message);
  }

  async function cancelRunning() {
    const tab = activeTab();
    if (tab?.runningID && bridge()) {
      try { await bridge().Cancel(tab.id, tab.runningID); } catch (error) { showAppError(error); }
    }
  }

  async function refreshHistory(tab = activeTab()) {
    if (!tab || !bridge()) return;
    try { tab.history = await bridge().History(tab.id); } catch (_) { /* keep local copy */ }
  }

  function navigateHistory(direction) {
    const tab = activeTab();
    if (!tab?.history.length) return;
    if (tab.historyIndex === -1) tab.draftBeforeHistory = els.editor.value;
    tab.historyIndex = Math.max(-1, Math.min(tab.history.length - 1, tab.historyIndex + direction));
    const value = tab.historyIndex === -1
      ? tab.draftBeforeHistory
      : tab.history[tab.history.length - 1 - tab.historyIndex];
    setEditor(value);
    requestSuggestions();
  }

  function clearBlocks() {
    const tab = activeTab();
    if (!tab) return;
    tab.selectedBlocks.clear();
    tab.selectionAnchor = "";
    const running = tab.runningID ? tab.blocks.get(tab.runningID) : null;
    tab.blocks.clear();
    tab.pane.replaceChildren();
    if (running) {
      tab.blocks.set(running.data.id, running);
      tab.pane.appendChild(running.node);
    }
    updateBlockSelectionToolbar(tab);
    els.editor.focus();
  }

  function registerEvents() {
    if (!window.runtime?.EventsOn) return;
    window.runtime.EventsOn("block:output", appendOutput);
    window.runtime.EventsOn("block:done", finishBlock);
    window.runtime.EventsOn("ssh:status", updateSSHStatus);
  }

  els.sshHealth.addEventListener("click", async () => {
    const tab = activeTab();
    if (!tab?.sshStatus || tab.sshStatus.state === "connected" || !bridge()) return;
    tab.sshStatus = { ...tab.sshStatus, state: "connecting", message: "Reconnecting now…" };
    renderSSHStatus(tab);
    try { await bridge().ReconnectSSH(tab.id); } catch (error) {
      tab.sshStatus = { ...tab.sshStatus, state: "disconnected", message: cleanError(error) };
      renderSSHStatus(tab);
    }
  });

  function fillSettingsForm(settings) {
    const radio = els.settingsForm.querySelector(`input[name="theme"][value="${settings.theme || "system"}"]`);
    if (radio) radio.checked = true;
    els.fontFamily.value = settings.fontFamily || defaults.fontFamily;
    els.fontSize.value = settings.fontSize || defaults.fontSize;
    els.defaultPath.value = settings.defaultPath || defaults.defaultPath;
    els.shellPath.value = settings.shell || "";
    els.aiEnabled.checked = settings.aiEnabled !== false;
    els.reduceTransparency.checked = Boolean(settings.reduceTransparency);
    els.showBlockMetadata.checked = settings.showBlockMetadata !== false;
    els.openHomeOnLaunch.checked = settings.openHomeOnLaunch !== false;
    els.runProjectCommands.checked = settings.runProjectCommands !== false;
    els.sshHelperEnabled.checked = settings.sshHelperEnabled !== false;
    els.morningGreetings.value = (settings.morningGreetings || defaults.morningGreetings).join("\n");
    els.dayGreetings.value = (settings.dayGreetings || defaults.dayGreetings).join("\n");
    els.eveningGreetings.value = (settings.eveningGreetings || defaults.eveningGreetings).join("\n");
    els.nightGreetings.value = (settings.nightGreetings || defaults.nightGreetings).join("\n");
    els.configPath.value = state.configPath || "";
    updateAIControls();
  }

  function readSettingsForm() {
    const lines = (element) => element.value.split("\n").map((value) => value.trim()).filter(Boolean);
    return {
      theme: els.settingsForm.querySelector('input[name="theme"]:checked')?.value || "system",
      fontFamily: els.fontFamily.value,
      fontSize: Number(els.fontSize.value),
      defaultPath: els.defaultPath.value.trim(),
      shell: els.shellPath.value.trim(),
      aiEnabled: els.aiEnabled.checked,
      aiModel: defaults.aiModel,
      reduceTransparency: els.reduceTransparency.checked,
      showBlockMetadata: els.showBlockMetadata.checked,
      openHomeOnLaunch: els.openHomeOnLaunch.checked,
      runProjectCommands: els.runProjectCommands.checked,
      sshHelperEnabled: els.sshHelperEnabled.checked,
      morningGreetings: lines(els.morningGreetings),
      dayGreetings: lines(els.dayGreetings),
      eveningGreetings: lines(els.eveningGreetings),
      nightGreetings: lines(els.nightGreetings),
    };
  }

  function openSettings() {
    state.settingsAnimationSequence += 1;
    state.settingsReturnFocus = document.activeElement;
    setSettingsBusy(false);
    delete els.settingsLayer.dataset.closing;
    state.settingsSnapshot = { ...state.settings };
    fillSettingsForm(state.settings);
    els.settingsError.hidden = true;
    els.settingsLayer.hidden = false;
    els.appShell.inert = true;
    for (const element of [els.settingsBackdrop, els.settingsPanel]) {
      element.getAnimations().forEach((animation) => animation.cancel());
    }
    if (!reducedMotion()) {
      els.settingsBackdrop.animate([{ opacity: 0 }, { opacity: 1 }], {
        duration: 180, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both",
      });
      els.settingsPanel.animate([
        { opacity: 0, transform: "translateY(8px) scale(0.98)" },
        { opacity: 1, transform: "translateY(0) scale(1)" },
      ], { duration: 220, easing: "cubic-bezier(0.22, 1, 0.36, 1)", fill: "both" });
    }
    refreshAIStatus();
    els.settingsClose.focus();
  }

  function updateAIControls() {
    const enabled = els.aiEnabled.checked;
    if (!enabled) {
      els.aiStatus.dataset.state = "disabled";
      els.aiStatusMessage.textContent = "Smart autocomplete is off";
    }
  }

  async function refreshAIStatus() {
    updateAIControls();
    if (!els.aiEnabled.checked) return;
    els.aiStatus.dataset.state = "checking";
    els.aiStatusMessage.textContent = "Checking local model…";
    if (!bridge()) {
      els.aiStatus.dataset.state = "unavailable";
      els.aiStatusMessage.textContent = "Run the native app to check the built-in model";
      return;
    }
    try {
      const status = await bridge().AutocompleteStatus();
      els.aiStatus.dataset.state = status.state || "unavailable";
      els.aiStatusMessage.textContent = status.message || "Built-in model is unavailable";
    } catch (_) {
      els.aiStatus.dataset.state = "unavailable";
      els.aiStatusMessage.textContent = "Built-in model is unavailable";
    }
  }

  const scheduleAIStatus = debounce(refreshAIStatus, 180);

  function setSettingsBusy(busy) {
    state.settingsBusy = busy;
    els.settingsPanel.setAttribute("aria-busy", String(busy));
    els.settingsPanel.querySelectorAll("button, input, select, textarea").forEach((control) => { control.disabled = busy; });
  }

  function closeSettings(restore = true, force = false) {
    if (state.settingsBusy && !force) return;
    if (restore && state.settingsSnapshot) applyVisualSettings(state.settingsSnapshot);
    state.settingsSnapshot = null;
    if (els.settingsLayer.hidden || els.settingsLayer.dataset.closing === "true") return;
    const sequence = ++state.settingsAnimationSequence;
    const finish = () => {
      if (sequence !== state.settingsAnimationSequence) return;
      els.settingsLayer.hidden = true;
      delete els.settingsLayer.dataset.closing;
      els.appShell.inert = false;
      const target = state.settingsReturnFocus;
      state.settingsReturnFocus = null;
      if (target?.isConnected) {
        target.focus();
      } else if (state.homeActive) {
        chooseGreeting(true);
        renderHome();
        els.homeSearch.focus();
      } else {
        els.editor.focus();
      }
    };
    if (reducedMotion()) {
      finish();
      return;
    }
    els.settingsLayer.dataset.closing = "true";
    const backdropAnimation = els.settingsBackdrop.animate([{ opacity: 1 }, { opacity: 0 }], {
      duration: 140, easing: "ease-in", fill: "both",
    });
    const panelAnimation = els.settingsPanel.animate([
      { opacity: 1, transform: "translateY(0) scale(1)" },
      { opacity: 0, transform: "translateY(5px) scale(0.99)" },
    ], { duration: 140, easing: "ease-in", fill: "both" });
    Promise.allSettled([backdropAnimation.finished, panelAnimation.finished]).then(finish);
  }

  function showSettingsError(message) {
    if (els.settingsLayer.hidden) openSettings();
    els.settingsError.textContent = message;
    els.settingsError.hidden = false;
  }

  async function saveSettings(event) {
    event.preventDefault();
    if (state.settingsBusy) return;
    setSettingsBusy(true);
    const value = readSettingsForm();
    try {
      const saved = bridge() ? await bridge().SaveSettings(value) : value;
      state.settings = { ...saved };
      applyVisualSettings(state.settings);
      if (!state.settings.aiEnabled) clearSuggestions();
      els.settingsError.hidden = true;
      closeSettings(false, true);
    } catch (error) {
      showSettingsError(cleanError(error));
    } finally {
      setSettingsBusy(false);
    }
  }

  async function init() {
    registerEvents();
    if (bridge()) {
      try {
        const initial = await bridge().InitialState();
        state.settings = { ...defaults, ...initial.settings };
        state.commands = new Set(Array.isArray(initial.commands) ? initial.commands : []);
        state.catalog = initial.catalog || { projects: [], servers: [] };
        state.configPath = initial.configPath || "";
        applyVisualSettings(state.settings);
        renderHome();
        for (const meta of initial.tabs || []) addTab(meta);
        const first = initial.activeTabId || initial.tabs?.[0]?.id;
        const firstTab = state.tabs.get(first);
        state.home = firstTab?.cwd.match(/^\/Users\/[^/]+|^\/home\/[^/]+/)?.[0] || "";
        if (initial.openHome) showHome();
        else if (first) switchTab(first);
        if (initial.startupError) {
          showHome();
          showHomeNotice(`Configuration could not be loaded: ${initial.startupError}`, true);
        }
        refreshCommandInventory(first);
      } catch (error) {
        showLocalError(cleanError(error));
      }
    } else {
      applyVisualSettings(defaults);
      state.home = "~";
      state.catalog = { projects: [], servers: [] };
      showHome();
    }
    if (!state.homeActive) els.editor.focus();
  }

  els.editor.addEventListener("input", updateEditorState);
  els.editor.addEventListener("scroll", syncEditorLayers);
  els.editor.addEventListener("click", refreshGhost);
  els.editor.addEventListener("keyup", refreshGhost);

  // Line-oriented interactive programs stay inside their block rather than
  // switching to the full-screen renderer. Capture their keyboard input at
  // the document boundary so prompts work regardless of which visible element
  // currently has focus.
  document.addEventListener("keydown", (event) => {
    if (!capturesRunningInput()) return;
    if (event.ctrlKey && event.key.toLowerCase() === "c" && !window.getSelection()?.isCollapsed) return;
    const sequence = terminalSequenceForKey(event);
    if (sequence === null) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendTerminalInput(activeTab(), sequence);
  }, true);

  document.addEventListener("beforeinput", (event) => {
    if (!capturesRunningInput() || event.isComposing) return;
    const sequences = {
      insertLineBreak: "\r", insertParagraph: "\r", deleteContentBackward: "\x7f", deleteContentForward: "\x1b[3~",
    };
    const value = sequences[event.inputType] ?? (event.inputType === "insertText" ? event.data : null);
    if (!value) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendTerminalInput(activeTab(), value);
  }, true);

  document.addEventListener("compositionstart", () => {
    if (capturesRunningInput()) state.runningCompositionDraft = els.editor.value;
  }, true);

  document.addEventListener("compositionend", (event) => {
    if (state.runningCompositionDraft === null) return;
    const draft = state.runningCompositionDraft;
    state.runningCompositionDraft = null;
    if (event.data) sendTerminalInput(activeTab(), event.data);
    setEditor(draft);
    const tab = activeTab();
    if (tab) tab.editorDraft = draft;
  }, true);

  document.addEventListener("paste", (event) => {
    if (!capturesRunningInput()) return;
    const value = event.clipboardData?.getData("text/plain") || "";
    if (!value) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendTerminalInput(activeTab(), value);
  }, true);

  els.editor.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault(); execute(); return;
    }
    if (event.key === "Tab") {
      event.preventDefault();
      if (!acceptSuggestion()) document.execCommand("insertText", false, "  ");
      return;
    }
    if (event.key === "ArrowRight" && (event.altKey || event.ctrlKey) && els.editor.selectionStart === els.editor.value.length && acceptSuggestionWord()) {
      event.preventDefault(); return;
    }
    if (event.key === "ArrowRight" && els.editor.selectionStart === els.editor.value.length && acceptSuggestion()) {
      event.preventDefault(); return;
    }
    if (event.key === "ArrowUp" && !event.shiftKey) {
      event.preventDefault();
      clearSuggestions();
      navigateHistory(1);
      return;
    }
    if (event.key === "ArrowDown" && !event.shiftKey) {
      event.preventDefault();
      clearSuggestions();
      navigateHistory(-1);
      return;
    }
    if (event.key === "Escape") clearSuggestions();
  });

  els.suggestions.addEventListener("mousedown", (event) => {
    const button = event.target.closest(".suggestion");
    if (!button) return;
    event.preventDefault();
    selectSuggestion(Number(button.dataset.index));
    acceptSuggestion();
    els.editor.focus();
  });

  els.tabList.addEventListener("click", (event) => {
    const close = event.target.closest("[data-close-tab]");
    if (close) {
      event.stopPropagation();
      closeTab(close.dataset.closeTab);
      return;
    }
    const tab = event.target.closest(".tab-button");
    if (tab) switchTab(tab.dataset.tabId);
  });

  els.tabList.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      const tab = event.target.closest(".tab-button");
      if (tab) { event.preventDefault(); switchTab(tab.dataset.tabId); }
    }
  });

  els.blocks.addEventListener("click", (event) => {
    const select = event.target.closest("[data-select-block]");
    if (select) {
      event.preventDefault();
      toggleBlockSelection(select.dataset.selectBlock, event.shiftKey);
      return;
    }
    const button = event.target.closest("[data-copy-block]");
    if (button) {
      copyBlock(button.dataset.copyBlock, button);
      return;
    }
    const block = event.target.closest(".block");
    const tab = activeTab();
    const rangeSelection = event.shiftKey && tab?.selectedBlocks.size > 0 && !window.getSelection()?.toString();
    if (block && (event.metaKey || event.ctrlKey || rangeSelection) && !event.target.closest("button, a")) {
      event.preventDefault();
      toggleBlockSelection(block.dataset.id, rangeSelection);
    }
  });

  els.copySelectedCommands.addEventListener("click", () => copySelectedBlocks(false, els.copySelectedCommands));
  els.copySelectedBlocks.addEventListener("click", () => copySelectedBlocks(true, els.copySelectedBlocks));
  els.clearBlockSelection.addEventListener("click", () => clearBlockSelection());

  els.homeButton.addEventListener("click", showHome);
  els.homeSearch.addEventListener("input", renderHome);
  els.addProject.addEventListener("click", () => openProjectEditor());
  els.addServer.addEventListener("click", () => openServerEditor());
  els.homeScreen.addEventListener("click", (event) => {
    const toggle = event.target.closest("[data-toggle-group]");
    if (toggle) {
      const groups = state.collapsedGroups[toggle.dataset.toggleGroup];
      const name = toggle.dataset.groupName;
      if (groups.has(name)) groups.delete(name);
      else groups.add(name);
      renderHome();
      return;
    }
    const create = event.target.closest("[data-create]");
    if (create) {
      create.dataset.create === "project" ? openProjectEditor() : openServerEditor();
      return;
    }
    const editProject = event.target.closest("[data-edit-project]");
    if (editProject) {
      event.stopPropagation();
      openProjectEditor(state.catalog.projects.find((project) => project.id === editProject.dataset.editProject));
      return;
    }
    const editServer = event.target.closest("[data-edit-server]");
    if (editServer) {
      event.stopPropagation();
      openServerEditor(state.catalog.servers.find((server) => server.id === editServer.dataset.editServer));
      return;
    }
    const project = event.target.closest("[data-open-project]");
    if (project) { openResource("project", project.dataset.openProject); return; }
    const server = event.target.closest("[data-open-server]");
    if (server) openResource("server", server.dataset.openServer);
  });
  els.homeScreen.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    if (event.target.closest("[data-edit-project], [data-edit-server]")) return;
    const project = event.target.closest("[data-open-project]");
    const server = event.target.closest("[data-open-server]");
    if (project || server) {
      event.preventDefault();
      openResource(project ? "project" : "server", project?.dataset.openProject || server.dataset.openServer);
    }
  });

  els.resourceEditorBackdrop.addEventListener("click", () => closeResourceEditor());
  els.resourceEditorClose.addEventListener("click", () => closeResourceEditor());
  els.resourceEditorLayer.querySelectorAll("[data-close-editor]").forEach((button) => button.addEventListener("click", () => closeResourceEditor()));
  els.projectForm.addEventListener("submit", saveProject);
  els.serverForm.addEventListener("submit", saveServer);
  els.deleteProject.addEventListener("click", () => confirmDeletion(els.deleteProject, deleteCurrentProject));
  els.deleteServer.addEventListener("click", () => confirmDeletion(els.deleteServer, deleteCurrentServer));

  els.newTab.addEventListener("click", newTab);
  document.querySelector(".tabbar").addEventListener("dblclick", (event) => {
    if (!event.target.closest("button, .tab-button") && window.runtime) window.runtime.WindowToggleMaximise();
  });
  els.settings.addEventListener("click", openSettings);
  els.settingsClose.addEventListener("click", () => closeSettings());
  els.settingsBackdrop.addEventListener("click", () => closeSettings());
  els.settingsForm.addEventListener("submit", saveSettings);
  els.settingsForm.addEventListener("input", () => applyVisualSettings(readSettingsForm()));
  els.settingsForm.addEventListener("change", () => applyVisualSettings(readSettingsForm()));
  els.aiEnabled.addEventListener("change", () => {
    updateAIControls();
    scheduleAIStatus();
  });
  els.settingsReset.addEventListener("click", () => {
    fillSettingsForm(defaults);
    applyVisualSettings(defaults);
    refreshAIStatus();
    els.settingsError.hidden = true;
  });
  els.reloadConfig.addEventListener("click", async () => {
    if (!bridge() || state.settingsBusy) return;
    setSettingsBusy(true);
    try {
      const initial = await bridge().ReloadConfiguration();
      state.settings = { ...defaults, ...initial.settings };
      state.catalog = initial.catalog || { projects: [], servers: [] };
      state.configPath = initial.configPath || state.configPath;
      applyVisualSettings(state.settings);
      fillSettingsForm(state.settings);
      renderHome();
      els.settingsError.hidden = true;
    } catch (error) {
      showSettingsError(cleanError(error));
    } finally {
      setSettingsBusy(false);
    }
  });

  function trapModalFocus(event, container) {
    if (event.key !== "Tab") return;
    const focusable = [...container.querySelectorAll(
      'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    )].filter((element) => !element.hidden && element.offsetParent !== null);
    if (!focusable.length) {
      event.preventDefault();
      container.focus();
      return;
    }
    const first = focusable[0];
    const last = focusable.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  document.addEventListener("keydown", (event) => {
    const command = event.metaKey || event.ctrlKey;
    if (!els.resourceEditorLayer.hidden) {
      if (event.key === "Escape") { event.preventDefault(); closeResourceEditor(); }
      trapModalFocus(event, els.resourceEditorLayer);
      return;
    }
    if (!els.settingsLayer.hidden) {
      if (event.key === "Escape") { event.preventDefault(); closeSettings(); }
      if (command && event.key === ",") { event.preventDefault(); closeSettings(); }
      trapModalFocus(event, els.settingsPanel);
      return;
    }
    if (event.key === "Escape" && activeTab()?.selectedBlocks.size) {
      event.preventDefault(); clearBlockSelection(); return;
    }
    if (document.body.dataset.terminalMode === "true" && !event.metaKey) return;
    if (event.ctrlKey && event.key.toLowerCase() === "c" && activeTab()?.runningID && window.getSelection()?.isCollapsed) {
      event.preventDefault(); cancelRunning(); return;
    }
    if (command && event.key.toLowerCase() === "c" && activeTab()?.selectedBlocks.size) {
      const editorHasSelection = document.activeElement === els.editor && els.editor.selectionStart !== els.editor.selectionEnd;
      const pageHasSelection = !window.getSelection()?.isCollapsed;
      if (!editorHasSelection && !pageHasSelection) {
        event.preventDefault(); copySelectedBlocks(true, els.copySelectedBlocks); return;
      }
    }
    if (event.ctrlKey && event.key.toLowerCase() === "l") {
      event.preventDefault(); clearBlocks(); return;
    }
    if (command && event.key.toLowerCase() === "k") {
      event.preventDefault(); els.editor.focus(); els.editor.select(); return;
    }
    if (command && event.key.toLowerCase() === "t") {
      event.preventDefault(); newTab(); return;
    }
    if (command && event.shiftKey && event.key.toLowerCase() === "h") {
      event.preventDefault(); showHome(); return;
    }
    if (command && event.key.toLowerCase() === "f" && state.homeActive) {
      event.preventDefault(); els.homeSearch.focus(); els.homeSearch.select(); return;
    }
    if (command && event.key.toLowerCase() === "w") {
      event.preventDefault(); if (state.activeTabID) closeTab(state.activeTabID); return;
    }
    if (command && event.key === ",") {
      event.preventDefault(); openSettings(); return;
    }
    if (command && /^[1-9]$/.test(event.key)) {
      const tabID = [...state.tabs.keys()][Number(event.key) - 1];
      if (tabID) { event.preventDefault(); switchTab(tabID); }
    }
  });

  const handleViewportResize = () => {
    if (document.body.dataset.terminalMode === "true") scheduleInteractiveResize();
  };
  window.addEventListener("resize", handleViewportResize);
  window.visualViewport?.addEventListener("resize", handleViewportResize);

  init();
})();
