import { execFileAsync } from 'child-process-promise';
import { exec, execFile } from 'node:child_process';
import { promisify } from 'node:util';

const execAsync = promisify(exec);

export type SimoneMcpAgentAction =
  | { action: 'agent.help' }
  | { action: 'sin.simone.mcp.health' }
  | { action: 'sin.simone.mcp.symbol.search'; query: string }
  | { action: 'sin.simone.mcp.structural.edit'; editPayload: string }
  | { action: 'sin.simone.mcp.memory.query'; query: string };

export async function executeSimoneMcpAgentAction(action: SimoneMcpAgentAction): Promise<unknown> {
  switch (action.action) {
    case 'agent.help':
      return {
        ok: true,
        agent: 'sin-simone-mcp',
        mandate: 'Ultra-Duo Team Coder with LSP-powered semantic code analysis, async A2A runtime, and cloud semantic memory.',
        actions: ['sin.simone.mcp.health', 'sin.simone.mcp.symbol.search', 'sin.simone.mcp.structural.edit', 'sin.simone.mcp.memory.query'],
      };

    case 'sin.simone.mcp.health':
      return {
        ok: true,
        agent: 'sin-simone-mcp',
        primaryModel: 'opencode/qwen3.6-plus-free',
        fallbackModel: 'opencode/nemotron-3-super-free',
        team: 'Team - Coding',
        status: 'Simone MCP Online — LSP Semantic Engine Ready',
      };

    case 'sin.simone.mcp.symbol.search':
      return await executeOpenCode(
        `Use LSP-powered semantic analysis to search for symbols matching: ${action.query}. Return symbol locations, types, and references.`
      );

    case 'sin.simone.mcp.structural.edit':
      return await executeOpenCode(
        `Perform a structural code edit using LSP-grade symbol resolution: ${action.editPayload}`
      );

    case 'sin.simone.mcp.memory.query':
      return await executeOpenCode(
        `Query the cloud semantic memory for code context matching: ${action.query}`
      );
  }
}

async function executeOpenCode(prompt: string, dir?: string) {
  const options = dir ? { cwd: dir } : {};

  try {
    const { stdout, stderr } = await execFileAsync('opencode', ['run', prompt, '--model', 'opencode/qwen3.6-plus-free'], options);
    return {
      ok: true,
      expertAnalysis: stdout,
      warnings: stderr || undefined,
    };
  } catch (error: any) {
    throw new Error(`Simone MCP Execution Failed: ${error.message}`);
  }
}

// OpenCode LLM call helper
async function callLLM(prompt: string): Promise<string> {
  const result = await execFileAsync('opencode', ['run', prompt, '--format', 'json']);
  return result.stdout;
}
