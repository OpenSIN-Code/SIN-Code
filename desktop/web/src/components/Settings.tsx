'use client';

import { useState } from 'react';

export function Settings() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const [autoCheck, setAutoCheck] = useState(true);

  return (
    <div className="max-w-5xl mx-auto">
      <header className="mb-sin-6">
        <h2 className="text-sin-3xl font-bold">Settings</h2>
        <p className="text-sin-textMuted mt-sin-1">
          Desktop app preferences
        </p>
      </header>

      <section className="sin-card mb-sin-4">
        <h3 className="text-sin-lg font-semibold mb-sin-3">Appearance</h3>
        <div className="space-y-sin-3">
          <div>
            <label className="block text-sin-sm font-medium mb-sin-1">Theme</label>
            <select
              value={theme}
              onChange={(e) => setTheme(e.target.value as 'dark' | 'light')}
              className="sin-input"
            >
              <option value="dark">Dark (default)</option>
              <option value="light">Light</option>
            </select>
          </div>
        </div>
      </section>

      <section className="sin-card mb-sin-4">
        <h3 className="text-sin-lg font-semibold mb-sin-3">Updates</h3>
        <label className="flex items-center gap-sin-2">
          <input
            type="checkbox"
            checked={autoCheck}
            onChange={(e) => setAutoCheck(e.target.checked)}
            className="w-4 h-4"
          />
          <span className="text-sin-sm">Check for updates on startup</span>
        </label>
      </section>

      <section className="sin-card">
        <h3 className="text-sin-lg font-semibold mb-sin-3">About</h3>
        <dl className="text-sin-sm space-y-sin-1">
          <div className="flex justify-between">
            <dt className="text-sin-textMuted">Version</dt>
            <dd>1.0.0</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-sin-textMuted">Tauri</dt>
            <dd>2.x</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-sin-textMuted">Frontend</dt>
            <dd>Next.js 14 (static export)</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-sin-textMuted">Design tokens</dt>
            <dd>Shared with TUI (Go/Lipgloss)</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}