# Third-party notices

The self-contained macOS build includes these local AI components:

- **llama.cpp / llama-server**, MIT License, pinned to the b9840-compatible
  runtime used by NTerm. Source: https://github.com/ggml-org/llama.cpp
- **Qwen2.5-Coder-0.5B-Instruct GGUF**, Apache License 2.0, Q4_K_M
  quantization. Source: https://huggingface.co/Qwen/Qwen2.5-Coder-0.5B-Instruct-GGUF

The components run only as child processes of NTerm, bind to the loopback
interface, and are stopped when NTerm exits or after the configured idle period.

The terminal engine also includes:

- **xterm.js 6.0.0** and **@xterm/addon-fit 0.11.0**, MIT License. The
  minified renderer, stylesheet and license texts are vendored into the app so
  users do not install npm packages. Source: https://github.com/xtermjs/xterm.js
- **creack/pty 1.1.24**, MIT License. It is linked into the Go executable and
  provides Unix pseudo-terminal allocation and resize support. Source:
  https://github.com/creack/pty
