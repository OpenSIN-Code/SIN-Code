'use client';

interface StatusBarProps {
  sinVersion: string;
  sinAvailable: boolean;
  appVersion: string;
}

export function StatusBar({ sinVersion, sinAvailable, appVersion }: StatusBarProps) {
  return (
    <footer className="h-8 bg-sin-surface border-t border-sin-border px-sin-4 flex items-center justify-between text-sin-xs text-sin-textMuted">
      <div className="flex items-center gap-sin-3">
        <span className="flex items-center gap-sin-1">
          <div
            className={`w-2 h-2 rounded-full ${
              sinAvailable ? 'bg-sin-success' : 'bg-sin-danger'
            }`}
          />
          sin: {sinVersion}
        </span>
      </div>
      <div className="flex items-center gap-sin-3">
        <span>Desktop v{appVersion}</span>
        <span>•</span>
        <span>Tauri 2.x + Next.js 14</span>
      </div>
    </footer>
  );
}