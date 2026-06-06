'use client';

import { TerminalIcon } from './icons';

export function TuiLauncher() {
  return (
    <div className="max-w-5xl mx-auto">
      <header className="mb-sin-6">
        <h2 className="text-sin-3xl font-bold">TUI Launcher</h2>
        <p className="text-sin-textMuted mt-sin-1">
          Launch the SIN-Code Bubbletea terminal UI
        </p>
      </header>

      <section className="sin-card">
        <div className="flex items-start gap-sin-4">
          <TerminalIcon className="w-12 h-12 text-sin-primary" />
          <div className="flex-1">
            <h3 className="text-sin-lg font-semibold mb-sin-2">
              sin-tui (Bubbletea)
            </h3>
            <p className="text-sin-sm text-sin-textMuted mb-sin-3">
              The TUI is a separate Go binary that provides a full interactive
              menu over every <code className="text-sin-accent">sin</code> subcommand.
              It must be invoked from a real terminal because it uses alt-screen +
              mouse.
            </p>
            <p className="text-sin-sm text-sin-textMuted mb-sin-3">
              To use the TUI:
            </p>
            <ol className="list-decimal list-inside text-sin-sm text-sin-textMuted space-y-sin-1">
              <li>Open your system terminal (Terminal.app, iTerm2, etc.)</li>
              <li>
                Run: <code className="text-sin-accent">sin tui</code>
              </li>
              <li>
                Or directly: <code className="text-sin-accent">~/.local/bin/sin-tui</code>
              </li>
            </ol>
          </div>
        </div>
      </section>

      <section className="mt-sin-4 sin-card">
        <h3 className="text-sin-lg font-semibold mb-sin-3">TUI Features</h3>
        <ul className="text-sin-sm text-sin-textMuted space-y-sin-1">
          <li>40+ commands across 7 groups (Code, Go Tools, Python Tools, Security, Skills, MCP, System)</li>
          <li>Live search filter (<code className="text-sin-accent">/</code>)</li>
          <li>Help modal (<code className="text-sin-accent">?</code>)</li>
          <li>Theme switcher (<code className="text-sin-accent">t</code>)</li>
          <li>Command history (<code className="text-sin-accent">↑</code>/<code className="text-sin-accent">↓</code>)</li>
          <li>Copy-to-clipboard (<code className="text-sin-accent">y</code>)</li>
          <li>Streamed subprocess output</li>
        </ul>
      </section>
    </div>
  );
}