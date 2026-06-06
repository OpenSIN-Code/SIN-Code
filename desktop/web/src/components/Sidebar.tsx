'use client';

import { DashboardIcon, CodeIcon, ShieldIcon, TerminalIcon, SettingsIcon } from './icons';

export type View = 'dashboard' | 'code' | 'security' | 'tui' | 'settings';

interface SidebarProps {
  currentView: View;
  onViewChange: (view: View) => void;
}

const NAV_ITEMS: { id: View; label: string; icon: React.ReactNode }[] = [
  { id: 'dashboard', label: 'Dashboard', icon: <DashboardIcon /> },
  { id: 'code', label: 'Code Hub', icon: <CodeIcon /> },
  { id: 'security', label: 'Security', icon: <ShieldIcon /> },
  { id: 'tui', label: 'TUI', icon: <TerminalIcon /> },
  { id: 'settings', label: 'Settings', icon: <SettingsIcon /> },
];

export function Sidebar({ currentView, onViewChange }: SidebarProps) {
  return (
    <aside className="w-56 bg-sin-surface border-r border-sin-border flex flex-col">
      <div className="p-sin-4 border-b border-sin-border">
        <h1 className="text-sin-lg font-semibold text-sin-primary">SIN-Code</h1>
        <p className="text-sin-xs text-sin-textMuted">Desktop GUI</p>
      </div>
      <nav className="flex-1 p-sin-2">
        {NAV_ITEMS.map((item) => (
          <button
            key={item.id}
            onClick={() => onViewChange(item.id)}
            className={`w-full flex items-center gap-sin-3 px-sin-3 py-sin-2 rounded-sin-sm mb-sin-1 transition-colors duration-sin-fast ease-sin-out ${
              currentView === item.id
                ? 'bg-sin-primary/20 text-sin-primary'
                : 'text-sin-textMuted hover:bg-sin-base hover:text-sin-text'
            }`}
          >
            <span className="w-5 h-5">{item.icon}</span>
            <span className="text-sin-sm font-medium">{item.label}</span>
          </button>
        ))}
      </nav>
      <div className="p-sin-3 border-t border-sin-border text-sin-xs text-sin-textMuted">
        v1.0.0
      </div>
    </aside>
  );
}