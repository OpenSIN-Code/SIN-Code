'use client';

import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { Sidebar } from '@/components/Sidebar';
import { Dashboard } from '@/components/Dashboard';
import { CodeHub } from '@/components/CodeHub';
import { SecurityPanel } from '@/components/SecurityPanel';
import { TuiLauncher } from '@/components/TuiLauncher';
import { Settings } from '@/components/Settings';
import { StatusBar } from '@/components/StatusBar';

type View = 'dashboard' | 'code' | 'security' | 'tui' | 'settings';

export default function Home() {
  const [view, setView] = useState<View>('dashboard');
  const [sinVersion, setSinVersion] = useState<string>('...');
  const [sinAvailable, setSinAvailable] = useState<boolean>(false);
  const [appVersion] = useState<string>('1.0.0');

  useEffect(() => {
    invoke<string>('get_version')
      .then(setSinVersion)
      .catch(() => setSinVersion('error'));

    invoke<boolean>('check_sin_cli')
      .then(setSinAvailable)
      .catch(() => setSinAvailable(false));
  }, []);

  return (
    <div className="flex h-screen bg-sin-base text-sin-text">
      <Sidebar currentView={view} onViewChange={setView} />
      <main className="flex-1 flex flex-col overflow-hidden">
        <div className="flex-1 overflow-auto p-sin-6">
          {view === 'dashboard' && <Dashboard sinAvailable={sinAvailable} onNavigate={setView} />}
          {view === 'code' && <CodeHub />}
          {view === 'security' && <SecurityPanel />}
          {view === 'tui' && <TuiLauncher />}
          {view === 'settings' && <Settings />}
        </div>
        <StatusBar sinVersion={sinVersion} sinAvailable={sinAvailable} appVersion={appVersion} />
      </main>
    </div>
  );
}