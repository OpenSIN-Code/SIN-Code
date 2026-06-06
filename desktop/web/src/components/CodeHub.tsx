'use client';

import { useState } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { PlayIcon, CheckIcon, XIcon } from './icons';

interface CommandResult {
  success: boolean;
  stdout: string;
  stderr: string;
  duration_ms: number;
}

const TOOLS = [
  { key: 'ibd', label: 'IBD', desc: 'Intent-Based Diffing', args: ['ibd'] },
  { key: 'poc', label: 'PoC', desc: 'Proof-of-Correctness', args: ['poc'] },
  { key: 'adw', label: 'ADW', desc: 'Architectural Debt Watchdog', args: ['adw'] },
  { key: 'oracle', label: 'Oracle', desc: 'Verification Oracle', args: ['oracle'] },
  { key: 'sckg', label: 'SCKG', desc: 'Knowledge Graph', args: ['sckg', 'run'] },
  { key: 'codocs', label: 'CoDocs', desc: 'Documentation validator', args: ['codocs', 'check', '.'] },
];

export function CodeHub() {
  const [selectedTool, setSelectedTool] = useState<string | null>(null);
  const [result, setResult] = useState<CommandResult | null>(null);
  const [running, setRunning] = useState(false);

  const runTool = async (args: string[]) => {
    setRunning(true);
    setResult(null);
    const start = Date.now();
    try {
      const stdout = await invoke<string>('run_sin_command', { args });
      setResult({
        success: true,
        stdout,
        stderr: '',
        duration_ms: Date.now() - start,
      });
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : String(err);
      setResult({
        success: false,
        stdout: '',
        stderr: errorMsg,
        duration_ms: Date.now() - start,
      });
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="max-w-5xl mx-auto">
      <header className="mb-sin-6">
        <h2 className="text-sin-3xl font-bold">Code Hub</h2>
        <p className="text-sin-textMuted mt-sin-1">
          Run code analysis tools via the `sin` CLI
        </p>
      </header>

      <section className="mb-sin-4">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-sin-3">
          {TOOLS.map((tool) => (
            <button
              key={tool.key}
              onClick={() => setSelectedTool(tool.key)}
              className={`sin-card text-left transition-all duration-sin-fast ease-sin-out ${
                selectedTool === tool.key
                  ? 'border-sin-primary'
                  : 'hover:border-sin-accent'
              }`}
            >
              <h4 className="text-sin-base font-semibold text-sin-primary">
                {tool.label}
              </h4>
              <p className="text-sin-sm text-sin-textMuted mt-sin-1">{tool.desc}</p>
            </button>
          ))}
        </div>
      </section>

      {selectedTool && (
        <section className="sin-card">
          <h3 className="text-sin-lg font-semibold mb-sin-3">
            Run: sin {TOOLS.find((t) => t.key === selectedTool)?.label}
          </h3>
          <div className="flex gap-sin-2 mb-sin-3">
            <button
              onClick={() => {
                const tool = TOOLS.find((t) => t.key === selectedTool);
                if (tool) runTool(tool.args);
              }}
              disabled={running}
              className="sin-button-primary flex items-center gap-sin-2 disabled:opacity-50"
            >
              <PlayIcon className="w-4 h-4" />
              {running ? 'Running...' : 'Execute'}
            </button>
            <button
              onClick={() => {
                setSelectedTool(null);
                setResult(null);
              }}
              className="sin-button-secondary"
            >
              Clear
            </button>
          </div>

          {result && (
            <div className="mt-sin-3">
              <div className="flex items-center gap-sin-2 mb-sin-2">
                {result.success ? (
                  <span className="sin-badge sin-badge-success flex items-center gap-sin-1">
                    <CheckIcon className="w-3 h-3" /> Success ({result.duration_ms}ms)
                  </span>
                ) : (
                  <span className="sin-badge sin-badge-danger flex items-center gap-sin-1">
                    <XIcon className="w-3 h-3" /> Failed ({result.duration_ms}ms)
                  </span>
                )}
              </div>
              <pre className="bg-sin-base p-sin-3 rounded-sin-sm text-sin-xs font-mono overflow-x-auto max-h-96 overflow-y-auto">
                {result.stdout || result.stderr || '(no output)'}
              </pre>
            </div>
          )}
        </section>
      )}
    </div>
  );
}