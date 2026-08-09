(() => {
  "use strict";

  const els = {
    terminal: document.getElementById("terminal"),
    tuiShell: document.getElementById("tui-shell"),
    tuiTerminal: document.getElementById("tui-terminal"),
    blocks: document.getElementById("blocks"),
    editor: document.getElementById("command-editor"),
    syntax: document.getElementById("syntax-highlight"),
    ghost: document.getElementById("ghost"),
    composer: document.getElementById("composer"),
    suggestions: document.getElementById("suggestions"),
    cwd: document.getElementById("cwd"),
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
    vtTerminal: null,
    fitAddon: null,
    terminalBlockID: "",
    terminalResizeTimer: 0,
    terminalReplaying: false,
  };

  const bridge = () => window.go?.main?.App;
  const activeTab = () => state.tabs.get(state.activeTabID);
  const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
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
      ? { background: "#efeff1", foreground: "#242426", cursor: "#242426", selectionBackground: "#00000020" }
      : { background: "#141415", foreground: "#ededee", cursor: "#ededee", selectionBackground: "#ffffff28" };
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
    state.vtTerminal.onData((data) => {
      const tab = activeTab();
      if (!tab?.runningID || !bridge() || state.terminalReplaying || state.terminalBlockID !== tab.runningID) return;
      bridge().TerminalInput(tab.id, data).catch(() => {});
    });
    new ResizeObserver(resizeInteractiveTerminal).observe(els.tuiShell);
    requestAnimationFrame(resizeInteractiveTerminal);
  }

  function showInteractiveTerminal(tab, record, replay = true) {
    if (!state.vtTerminal || !tab || !record) return;
    const changed = state.terminalBlockID !== record.data.id;
    state.terminalBlockID = record.data.id;
    document.body.dataset.terminalMode = "true";
    els.tuiShell.setAttribute("aria-hidden", "false");
    if (changed || replay) {
      state.terminalReplaying = true;
      state.vtTerminal.reset();
      if (record.rawOutput) state.vtTerminal.write(record.rawOutput, () => { state.terminalReplaying = false; });
      else state.terminalReplaying = false;
    }
    requestAnimationFrame(() => {
      resizeInteractiveTerminal();
      state.vtTerminal.focus();
    });
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

  function syntaxToken(value, type) {
    return `<span class="syntax-${type}">${escapeHTML(value)}</span>`;
  }

  function shellWordType(word, expectsCommand, commands) {
    if (/^[A-Za-z_][A-Za-z0-9_]*=/.test(word)) return "variable";
    const decoded = word.replace(/\\(.)/g, "$1");
    const commandName = decoded.includes("/") ? decoded.split("/").pop() : decoded;
    if (expectsCommand && commands.has(commandName)) return "command";
    if (/^--?[A-Za-z0-9]/.test(word)) return "option";
    if (/^(?:~|\.{1,2})(?:\/|$)|^\/|\//.test(word)) return "path";
    if (/\$(?:[A-Za-z_][A-Za-z0-9_]*|\{)/.test(word)) return "variable";
    return "argument";
  }

  function renderShellSyntax(source, commands = state.commands) {
    const value = String(source || "");
    let result = "";
    let index = 0;
    let expectsCommand = true;

    while (index < value.length) {
      const start = index;
      const char = value[index];

      if (/\s/.test(char)) {
        while (index < value.length && /\s/.test(value[index])) index += 1;
        const whitespace = value.slice(start, index);
        result += escapeHTML(whitespace);
        if (whitespace.includes("\n")) expectsCommand = true;
        continue;
      }

      if (char === "#" && (index === 0 || /\s/.test(value[index - 1]))) {
        const end = value.indexOf("\n", index);
        const stop = end === -1 ? value.length : end;
        result += syntaxToken(value.slice(index, stop), "comment");
        index = stop;
        continue;
      }

      if (char === "\"" || char === "'") {
        const quote = char;
        index += 1;
        while (index < value.length) {
          if (value[index] === "\\" && quote === "\"" && index + 1 < value.length) {
            index += 2;
            continue;
          }
          if (value[index++] === quote) break;
        }
        result += syntaxToken(value.slice(start, index), "string");
        expectsCommand = false;
        continue;
      }

      if (/[|&;<>()]/.test(char)) {
        index += 1;
        if (index < value.length && value[index] === char && /[|&<>]/.test(char)) index += 1;
        const operator = value.slice(start, index);
        result += syntaxToken(operator, "operator");
        if (/^(?:\||\|\||&&|;|&|\(|\))$/.test(operator)) expectsCommand = true;
        continue;
      }

      while (index < value.length && !/[\s|&;<>()"']/.test(value[index])) {
        if (value[index] === "\\" && index + 1 < value.length) index += 2;
        else index += 1;
      }
      const word = value.slice(start, index);
      const type = shellWordType(word, expectsCommand, commands);
      result += syntaxToken(word, type);
      if (type === "command") expectsCommand = false;
      else if (type !== "variable" || !expectsCommand) expectsCommand = false;
    }
    return result;
  }

  function syncEditorLayers() {
    els.syntax.scrollTop = els.editor.scrollTop;
    els.syntax.scrollLeft = els.editor.scrollLeft;
    els.ghost.scrollTop = els.editor.scrollTop;
    els.ghost.scrollLeft = els.editor.scrollLeft;
  }

  function refreshSyntax() {
    els.syntax.innerHTML = renderShellSyntax(els.editor.value);
    syncEditorLayers();
  }

  async function refreshCommandInventory(tabID) {
    if (!bridge() || !tabID) return;
    try {
      const commands = await bridge().CommandNames(tabID);
      if (!Array.isArray(commands) || !commands.length) return;
      state.commands = new Set(commands);
      refreshSyntax();
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
    return String(error).replace(/^Error:\s*/, "").replace(/^.*?: /, "");
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
    for (const tab of state.tabs.values()) {
      const selected = tab.id === tabID;
      tab.pane.hidden = !selected;
      tab.element.setAttribute("aria-selected", String(selected));
      tab.element.tabIndex = selected ? 0 : -1;
    }
    clearSuggestions();
    setEditor(next.editorDraft);
    els.cwd.textContent = displayPath(next);
    els.cwd.title = next.remote ? `${next.remote}:${next.cwd}` : next.cwd;
    renderGitContext(next.gitInfo);
    refreshGitContext(tabID);
    refreshCommandInventory(tabID);
    els.composer.classList.toggle("is-running", next.running);
    requestAnimationFrame(() => { els.terminal.scrollTop = next.scrollTop; });
    refreshHistory(next);
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
      showSettingsError(cleanError(error));
    }
  }

  async function closeTab(tabID) {
    const wasActive = tabID === state.activeTabID;
    const order = [...state.tabs.keys()];
    const index = order.indexOf(tabID);
    try {
      if (bridge()) {
        const result = await bridge().CloseTab(tabID);
        removeTab(tabID);
        if (result.createdTab) addTab(result.createdTab);
        if (wasActive) switchTab(result.activeId || [...state.tabs.keys()][0]);
      } else {
        removeTab(tabID);
        if (!state.tabs.size) await newTab();
        else if (wasActive) switchTab(order[index + 1] || order[index - 1] || [...state.tabs.keys()][0]);
      }
    } catch (error) {
      showLocalError(cleanError(error));
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

  function renderSuggestions(localItems) {
    const previousValue = state.suggestions[state.selectedSuggestion]?.value;
    if (Array.isArray(localItems)) state.localSuggestions = localItems;
    const history = state.localSuggestions.filter((item) => item.source === "history");
    const local = state.localSuggestions.filter((item) => item.source !== "history");
    const ai = state.aiSuggestion && !state.localSuggestions.some((item) => item.value === state.aiSuggestion.value)
      ? [state.aiSuggestion]
      : [];
    state.suggestions = [...history, ...ai, ...local].slice(0, 8);
    const preservedIndex = state.suggestions.findIndex((item) => item.value === previousValue);
    state.selectedSuggestion = preservedIndex >= 0 ? preservedIndex : 0;
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
    requestLocalSuggestions.cancel();
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

  const requestLocalSuggestions = debounce(async () => {
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
      if (sequence === state.localRequestSequence && tab.id === state.activeTabID && input === els.editor.value) renderSuggestions(items);
    } catch (_) {
      if (sequence === state.localRequestSequence) {
        state.localSuggestions = [];
        renderSuggestions();
      }
    }
  }, 65);

  const requestAIPrediction = debounce(async () => {
    const tab = activeTab();
    const input = els.editor.value;
    const sequence = ++state.aiRequestSequence;
    if (!tab || input.trim().length < 3 || !state.settings.aiEnabled || !bridge()) return;
    try {
      const items = await bridge().Predict(tab.id, input);
      if (sequence !== state.aiRequestSequence || tab.id !== state.activeTabID || input !== els.editor.value) return;
      state.aiSuggestion = Array.isArray(items) ? items[0] || null : null;
      renderSuggestions();
    } catch (_) {
      if (sequence === state.aiRequestSequence) {
        state.aiSuggestion = null;
        renderSuggestions();
      }
    }
  }, 460);

  function requestSuggestions() {
    state.localSuggestions = [];
    state.aiSuggestion = null;
    renderSuggestions();
    requestLocalSuggestions();
    requestAIPrediction();
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
        <pre class="block-command">${renderShellSyntax(block.command)}</pre>
        <span class="block-trailing">
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
    };
    tab.blocks.set(block.id, record);
    if (tab.id === state.activeTabID) prepareTerminalRecord(tab, record);
    if (record.interactive) {
      node.classList.add("is-interactive");
      if (tab.id === state.activeTabID) showInteractiveTerminal(tab, record, false);
    }
    if (tab.id === state.activeTabID) scrollToBottom(true);
  }

  const ansiClasses = {
    1: "ansi-bold", 2: "ansi-dim", 30: "", 31: "ansi-red", 32: "ansi-green",
    33: "ansi-yellow", 34: "ansi-blue", 35: "ansi-magenta", 36: "ansi-cyan", 37: "ansi-white",
    90: "ansi-dim", 91: "ansi-red", 92: "ansi-green", 93: "ansi-yellow", 94: "ansi-blue",
    95: "ansi-magenta", 96: "ansi-cyan", 97: "ansi-white",
  };

  function renderANSI(raw) {
    let result = "";
    let position = 0;
    let classes = [];
    const sgr = /\x1b\[([0-9;]*)m/g;
    for (const match of raw.matchAll(sgr)) {
      result += wrapANSI(escapeHTML(raw.slice(position, match.index)), classes);
      const codes = match[1] ? match[1].split(";").map(Number) : [0];
      for (const code of codes) {
        if (code === 0 || code === 39 || code === 22) classes = [];
        else if (ansiClasses[code] !== undefined && ansiClasses[code]) {
          classes = classes.filter((value) => !value.startsWith("ansi-") || ["ansi-bold", "ansi-dim"].includes(value));
          classes.push(ansiClasses[code]);
        }
      }
      position = match.index + match[0].length;
    }
    result += wrapANSI(escapeHTML(raw.slice(position)), classes);
    return result.replace(/\x1b\][^\x07]*(?:\x07|$)/g, "").replace(/\x1b\[[0-?]*[ -\/]*[@-~]/g, "");
  }

  function wrapANSI(text, classes) {
    return classes.length && text ? `<span class="${classes.join(" ")}">${text}</span>` : text;
  }

  function plainTerminalText(value) {
    return String(value || "")
      .replace(/\x1b\][^\x07]*(?:\x07|\x1b\\|$)/g, "")
      .replace(/\x1b\[[0-?]*[ -\/]*[@-~]/g, "")
      .replace(/\r/g, "");
  }

  async function copyBlock(blockID, button) {
    const tab = activeTab();
    const record = tab?.blocks.get(blockID);
    if (!record) return;
    const output = plainTerminalText(record.output).replace(/\n+$/, "");
    const text = output ? `${record.data.command}\n${output}` : record.data.command;
    try {
      if (window.runtime?.ClipboardSetText) await window.runtime.ClipboardSetText(text);
      else await navigator.clipboard.writeText(text);
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
      if (!node.nodeValue?.trim() || node.parentElement?.closest('[class*="ansi-"], .ls-kind')) continue;
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
    if (record.interactive) {
      return;
    }
    record.output += chunk.data.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
    output.innerHTML = renderANSI(record.output);
    highlightLsOutput(output, record.data.command);
    if (record.output && output.dataset.visible !== "true") output.dataset.visible = "true";
    if (chunk.stream === "stderr") output.dataset.hasStderr = "true";
    if (tab.id === state.activeTabID) scrollToBottom();
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
    tab.title = tab.remote || titleForPath(tab.cwd);
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
      els.composer.classList.remove("is-running");
      els.editor.focus();
      refreshHistory(tab);
      scrollToBottom(true);
    }
    refreshCommandInventory(tab.id);
  }

  async function execute() {
    const tab = activeTab();
    const command = els.editor.value.trim();
    if (!tab || !command || tab.running || !bridge()) return;
    if (command === "clear") {
      clearBlocks();
      setEditor("");
      tab.editorDraft = "";
      return;
    }
    clearSuggestions();
    try {
      const block = await bridge().Execute(tab.id, command);
      tab.running = true;
      tab.runningID = block.id;
      tab.history.push(command);
      tab.editorDraft = "";
      renderTab(tab);
      createBlock(block);
      setEditor("");
      els.composer.classList.add("is-running");
    } catch (error) {
      showLocalError(cleanError(error));
    }
  }

  function showLocalError(message) {
    const tab = activeTab();
    if (!tab) return;
    const id = `local-${Date.now()}`;
    createBlock({ id, tabId: tab.id, command: els.editor.value || "NTerm", cwd: tab.cwd, state: "failed" });
    appendOutput({ tabId: tab.id, blockId: id, stream: "stderr", data: message + "\n" });
    finishBlock({ id, tabId: tab.id, state: "failed", cwd: tab.cwd, exitCode: 1, durationMs: 0 });
  }

  async function cancelRunning() {
    const tab = activeTab();
    if (tab?.runningID && bridge()) await bridge().Cancel(tab.id, tab.runningID);
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
    const running = tab.runningID ? tab.blocks.get(tab.runningID) : null;
    tab.blocks.clear();
    tab.pane.replaceChildren();
    if (running) {
      tab.blocks.set(running.data.id, running);
      tab.pane.appendChild(running.node);
    }
    els.editor.focus();
  }

  function registerEvents() {
    if (!window.runtime?.EventsOn) return;
    window.runtime.EventsOn("block:output", appendOutput);
    window.runtime.EventsOn("block:done", finishBlock);
  }

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
    updateAIControls();
  }

  function readSettingsForm() {
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
    };
  }

  function openSettings() {
    state.settingsAnimationSequence += 1;
    delete els.settingsLayer.dataset.closing;
    state.settingsSnapshot = { ...state.settings };
    fillSettingsForm(state.settings);
    els.settingsError.hidden = true;
    els.settingsLayer.hidden = false;
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

  function closeSettings(restore = true) {
    if (restore && state.settingsSnapshot) applyVisualSettings(state.settingsSnapshot);
    state.settingsSnapshot = null;
    if (els.settingsLayer.hidden || els.settingsLayer.dataset.closing === "true") return;
    const sequence = ++state.settingsAnimationSequence;
    const finish = () => {
      if (sequence !== state.settingsAnimationSequence) return;
      els.settingsLayer.hidden = true;
      delete els.settingsLayer.dataset.closing;
      els.editor.focus();
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
    const value = readSettingsForm();
    try {
      const saved = bridge() ? await bridge().SaveSettings(value) : value;
      state.settings = { ...saved };
      applyVisualSettings(state.settings);
      if (!state.settings.aiEnabled) clearSuggestions();
      els.settingsError.hidden = true;
      closeSettings(false);
    } catch (error) {
      showSettingsError(cleanError(error));
    }
  }

  async function init() {
    registerEvents();
    if (bridge()) {
      try {
        const initial = await bridge().InitialState();
        state.settings = { ...defaults, ...initial.settings };
        state.commands = new Set(Array.isArray(initial.commands) ? initial.commands : []);
        applyVisualSettings(state.settings);
        initializeInteractiveTerminal();
        for (const meta of initial.tabs || []) addTab(meta);
        const first = initial.activeTabId || initial.tabs?.[0]?.id;
        const firstTab = state.tabs.get(first);
        state.home = firstTab?.cwd.match(/^\/Users\/[^/]+|^\/home\/[^/]+/)?.[0] || "";
        if (first) switchTab(first);
        refreshCommandInventory(first);
      } catch (error) {
        showLocalError(cleanError(error));
      }
    } else {
      applyVisualSettings(defaults);
      initializeInteractiveTerminal();
      state.home = "~";
      const preview = addTab({ id: "preview", title: "~", cwd: "~", running: false });
      switchTab(preview.id);
      els.cwd.textContent = "Run with “wails dev” to connect the shell";
    }
    els.editor.focus();
  }

  els.editor.addEventListener("input", updateEditorState);
  els.editor.addEventListener("scroll", syncEditorLayers);
  els.editor.addEventListener("click", refreshGhost);
  els.editor.addEventListener("keyup", refreshGhost);
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
      if (!els.suggestions.hidden) selectSuggestion(state.selectedSuggestion - 1);
      else navigateHistory(1);
      return;
    }
    if (event.key === "ArrowDown" && !event.shiftKey) {
      event.preventDefault();
      if (!els.suggestions.hidden) selectSuggestion(state.selectedSuggestion + 1);
      else navigateHistory(-1);
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
    const button = event.target.closest("[data-copy-block]");
    if (button) copyBlock(button.dataset.copyBlock, button);
  });

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

  document.addEventListener("keydown", (event) => {
    const command = event.metaKey || event.ctrlKey;
    if (!els.settingsLayer.hidden) {
      if (event.key === "Escape") { event.preventDefault(); closeSettings(); }
      if (command && event.key === ",") { event.preventDefault(); closeSettings(); }
      return;
    }
    if (document.body.dataset.terminalMode === "true" && !event.metaKey) return;
    if (event.ctrlKey && event.key.toLowerCase() === "c" && activeTab()?.runningID && window.getSelection()?.isCollapsed) {
      event.preventDefault(); cancelRunning(); return;
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

  init();
})();
