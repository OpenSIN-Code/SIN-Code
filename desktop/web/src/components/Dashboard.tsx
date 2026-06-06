'use client';

import { CodeIcon, ShieldIcon, TerminalIcon } from './icons';

interface DashboardProps {
  sinAvailable: boolean;
  onNavigate: (view: 'dashboard' | 'code' | 'security' | 'tui' | 'settings') => void;
}

interface QuickAction {
  id: 'code' | 'security' | 'tui';
  title: string;
  description: string;
  icon: React.ReactNode;
}

const QUICK_ACTIONS: QuickAction[] = [
  {
    id: 'code',
    title: 'Run Code Analysis',
    description: 'IBD, PoC, ADW, Oracle — semantic code tools',
    icon: <CodeIcon className="w-8 h-8" />,
  },
  {
    id: 'security',
    title: 'Run Security Scan',
    description: '8 tools: secrets, SAST, SCA, SBOM, container, IaC, license, DAST',
    icon: <ShieldIcon className="w-8 h-8" />,
  },
  {
    id: 'tui',
    title: 'Launch TUI',
    description: 'Interactive Bubbletea terminal UI',
    icon: <TerminalIcon className="w-8 h-8" />,
  },
];

export function Dashboard({ sinAvailable, onNavigate }: DashboardProps) {
  return (
    <div className="max-w-5xl mx-auto">
      <header className="mb-sin-6">
        <h2 className="text-sin-3xl font-bold text-sin-text">Dashboard</h2>
        <p className="text-sin-textMuted mt-sin-1">
          SIN-Code unified agent engineering stack
        </p>
      </header>

      <section className="sin-card mb-sin-4">
        <div className="flex items-center gap-sin-3">
          <div
            className={`w-3 h-3 rounded-full ${
              sinAvailable ? 'bg-sin-success' : 'bg-sin-danger'
            }`}
          />
          <div>
            <h3 className="text-sin-base font-medium">
              {sinAvailable ? 'sin CLI connected' : 'sin CLI not found'}
            </h3>
            <p className="text-sin-xs text-sin-textMuted">
              {sinAvailable
                ? 'All commands will be executed via the local `sin` binary.'
                : 'Install sin-code-bundle: pipx install sin-code-bundle'}
            </p>
          </div>
        </div>
      </section>

      <section className="mb-sin-4">
        <h3 className="text-sin-lg font-semibold mb-sin-3">Quick Actions</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-sin-3">
          {QUICK_ACTIONS.map((action) => (
            <button
              key={action.id}
              onClick={() => onNavigate(action.id)}
              className="sin-card text-left transition-all duration-sin-fast ease-sin-out hover:border-sin-primary"
            >
              <div className="text-sin-primary mb-sin-2">{action.icon}</div>
              <h4 className="text-sin-base font-medium mb-sin-1">{action.title}</h4>
              <p className="text-sin-sm text-sin-textMuted">{action.description}</p>
            </button>
          ))}
        </div>
      </section>

      <section>
        <h3 className="text-sin-lg font-semibold mb-sin-3">Architecture</h3>
        <div className="sin-card">
          <pre className="text-sin-xs text-sin-textMuted font-mono overflow-x-auto">
{`SIN-Code Desktop (Tauri v2)
├── Rust backend (src-tauri/)
│   ├── tauri.conf.json       # App config + plugins
│   ├── Cargo.toml            # Dependencies
│   └── src/main.rs           # Commands + tray + window
└── Next.js frontend (web/)
    ├── src/app/              # Routes (App Router)
    ├── src/components/       # UI components
    └── theme/tokens.css      # Shared design tokens

Design system shared with TUI:
  Go:    internal/tui/theme/tokens.go
  Web:   web/theme/tokens.css (via @import)
  Same color story across all UIs.`}
          </pre>
        </div>
      </section>
    </div>
  );
}