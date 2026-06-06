'use client';

import { useState } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { PlayIcon, CheckIcon, XIcon } from './icons';

interface ScanResult {
  tool: string;
  success: boolean;
  output: string;
  duration_ms: number;
}

const SECURITY_TOOLS = [
  { key: 'secrets', label: 'Secrets Scan', desc: 'Hardcoded secret detection', frameworks: ['OWASP', 'CIS'] },
  { key: 'sast', label: 'SAST', desc: 'Static Application Security Testing', frameworks: ['OWASP', 'NIST'] },
  { key: 'sca', label: 'SCA', desc: 'Software Composition Analysis', frameworks: ['NIST', 'SOC2'] },
  { key: 'sbom', label: 'SBOM', desc: 'Software Bill of Materials (SPDX + CycloneDX)', frameworks: ['ISO27001', 'NIST'] },
  { key: 'container', label: 'Container', desc: 'Container image security', frameworks: ['CIS', 'PCI'] },
  { key: 'iac', label: 'IaC', desc: 'Infrastructure as Code (Terraform, K8s)', frameworks: ['CIS', 'SOC2'] },
  { key: 'license', label: 'License', desc: 'License compliance check', frameworks: ['ISO27001', 'SOC2'] },
  { key: 'dast', label: 'DAST', desc: 'Dynamic Application Security Testing', frameworks: ['OWASP', 'PCI'] },
];

export function SecurityPanel() {
  const [results, setResults] = useState<Record<string, ScanResult>>({});
  const [running, setRunning] = useState<string | null>(null);
  const [target, setTarget] = useState('.');

  const runScan = async (tool: string) => {
    setRunning(tool);
    const start = Date.now();
    try {
      const stdout = await invoke<string>('run_sin_command', {
        args: ['security', tool, target],
      });
      setResults((prev) => ({
        ...prev,
        [tool]: {
          tool,
          success: true,
          output: stdout,
          duration_ms: Date.now() - start,
        },
      }));
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : String(err);
      setResults((prev) => ({
        ...prev,
        [tool]: {
          tool,
          success: false,
          output: errorMsg,
          duration_ms: Date.now() - start,
        },
      }));
    } finally {
      setRunning(null);
    }
  };

  const runAll = async () => {
    for (const tool of SECURITY_TOOLS) {
      await runScan(tool.key);
    }
  };

  return (
    <div className="max-w-5xl mx-auto">
      <header className="mb-sin-6">
        <h2 className="text-sin-3xl font-bold">Security</h2>
        <p className="text-sin-textMuted mt-sin-1">
          8-tool security bundle, 8 compliance frameworks
        </p>
      </header>

      <section className="sin-card mb-sin-4">
        <div className="flex items-end gap-sin-3">
          <div className="flex-1">
            <label className="block text-sin-sm font-medium mb-sin-1">
              Target path
            </label>
            <input
              type="text"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              className="sin-input w-full"
            />
          </div>
          <button
            onClick={runAll}
            disabled={running !== null}
            className="sin-button-primary flex items-center gap-sin-2 disabled:opacity-50"
          >
            <PlayIcon className="w-4 h-4" />
            Run All 8
          </button>
        </div>
      </section>

      <section className="grid grid-cols-1 md:grid-cols-2 gap-sin-3">
        {SECURITY_TOOLS.map((tool) => {
          const result = results[tool.key];
          const isRunning = running === tool.key;
          return (
            <div key={tool.key} className="sin-card">
              <div className="flex items-start justify-between mb-sin-2">
                <div>
                  <h4 className="text-sin-base font-semibold">{tool.label}</h4>
                  <p className="text-sin-sm text-sin-textMuted mt-sin-1">{tool.desc}</p>
                  <div className="flex gap-sin-1 mt-sin-2">
                    {tool.frameworks.map((f) => (
                      <span key={f} className="sin-badge sin-badge-info">
                        {f}
                      </span>
                    ))}
                  </div>
                </div>
                <button
                  onClick={() => runScan(tool.key)}
                  disabled={isRunning}
                  className="sin-button-primary text-sin-xs px-sin-2 py-sin-1 disabled:opacity-50"
                >
                  {isRunning ? '...' : 'Run'}
                </button>
              </div>
              {result && (
                <div className="mt-sin-2 pt-sin-2 border-t border-sin-border">
                  <div className="flex items-center gap-sin-2 mb-sin-1">
                    {result.success ? (
                      <span className="sin-badge sin-badge-success">
                        OK ({result.duration_ms}ms)
                      </span>
                    ) : (
                      <span className="sin-badge sin-badge-danger">
                        FAIL ({result.duration_ms}ms)
                      </span>
                    )}
                  </div>
                  <pre className="bg-sin-base p-sin-2 rounded-sin-sm text-sin-xs font-mono overflow-x-auto max-h-40 overflow-y-auto">
                    {result.output.slice(0, 500) || '(no output)'}
                    {result.output.length > 500 && '...'}
                  </pre>
                </div>
              )}
            </div>
          );
        })}
      </section>
    </div>
  );
}