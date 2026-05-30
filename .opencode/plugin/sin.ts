/**
 * SIN-Code Bundle — opencode plugin
 *
 * Turns the AGENTS.md doctrine into an *enforced* protocol:
 *   - after every file edit  -> run semantic_diff + architectural_debt
 *   - before a session ends   -> require a GREEN Oracle verification
 *   - on a tripped ADW breaker -> hard-stop the agent
 *
 * Docs: https://opencode.ai/docs/plugins
 *
 * The plugin talks to the SIN MCP tools that opencode already loaded via
 * `opencode.json` (mcp.sin). It does not shell out to `sin` itself; instead it
 * reads/writes a small session ledger under `.sin/session/` so the gate state
 * survives across tool calls.
 */

import type { Plugin } from "@opencode-ai/plugin"
import { mkdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"

// --------------------------------------------------------------------------- //
// Config (overridable via env)
// --------------------------------------------------------------------------- //
const SIN_DIR = ".sin"
const SESSION_DIR = join(SIN_DIR, "session")
const LEDGER = join(SESSION_DIR, "gate.json")

const RISK_BLOCK_LEVEL = (process.env.SIN_RISK_BLOCK ?? "high").toLowerCase()
const DEBT_BREAKER = Number(process.env.SIN_DEBT_BREAKER ?? "85") // 0-100
const ENFORCE = (process.env.SIN_ENFORCE ?? "1") !== "0"

type RiskLevel = "low" | "medium" | "high"

interface Ledger {
  /** files edited but not yet verified green */
  dirty: string[]
  /** last Oracle verdict: "pass" | "fail" | "unknown" */
  oracle: "pass" | "fail" | "unknown"
  /** last architectural debt score 0-100 */
  debt: number
  /** highest risk seen since last green verification */
  risk: RiskLevel
  /** human-readable reasons accumulated for the current gate */
  notes: string[]
  updatedAt: string
}

const EMPTY_LEDGER: Ledger = {
  dirty: [],
  oracle: "unknown",
  debt: 0,
  risk: "low",
  notes: [],
  updatedAt: new Date(0).toISOString(),
}

// --------------------------------------------------------------------------- //
// Ledger persistence
// --------------------------------------------------------------------------- //
async function readLedger(): Promise<Ledger> {
  try {
    const raw = await readFile(LEDGER, "utf8")
    return { ...EMPTY_LEDGER, ...(JSON.parse(raw) as Partial<Ledger>) }
  } catch {
    return { ...EMPTY_LEDGER }
  }
}

async function writeLedger(ledger: Ledger): Promise<void> {
  ledger.updatedAt = new Date().toISOString()
  await mkdir(SESSION_DIR, { recursive: true })
  await writeFile(LEDGER, JSON.stringify(ledger, null, 2), "utf8")
}

const RISK_ORDER: Record<RiskLevel, number> = { low: 0, medium: 1, high: 2 }
function maxRisk(a: RiskLevel, b: RiskLevel): RiskLevel {
  return RISK_ORDER[a] >= RISK_ORDER[b] ? a : b
}

// --------------------------------------------------------------------------- //
// Helpers to call the SIN MCP tools through the opencode client
// --------------------------------------------------------------------------- //
async function callSin(
  client: any,
  tool: string,
  args: Record<string, unknown>,
): Promise<any> {
  try {
    return await client.tool.call({ server: "sin", tool, arguments: args })
  } catch (err) {
    // Subsystem may be unavailable (graceful degradation). Never crash the agent.
    return { ok: false, error: String(err) }
  }
}

function parseRisk(result: any): RiskLevel {
  const r = String(result?.risk ?? result?.risk_level ?? "low").toLowerCase()
  if (r === "high" || r === "critical") return "high"
  if (r === "medium" || r === "moderate") return "medium"
  return "low"
}

function parseDebt(result: any): number {
  const d = Number(result?.score ?? result?.debt ?? result?.complexity ?? 0)
  return Number.isFinite(d) ? d : 0
}

function parseOracle(result: any): "pass" | "fail" | "unknown" {
  const v = String(result?.verdict ?? result?.status ?? "").toLowerCase()
  if (v === "pass" || v === "passed" || v === "green" || result?.ok === true)
    return "pass"
  if (v === "fail" || v === "failed" || v === "red" || result?.ok === false)
    return "fail"
  return "unknown"
}

// --------------------------------------------------------------------------- //
// Plugin
// --------------------------------------------------------------------------- //
export const SinPlugin: Plugin = async ({ client, $ }) => {
  return {
    /**
     * After any file edit: assess the change semantically and update debt.
     * This is the "review" + "guard debt" steps of the SIN loop, automated.
     */
    "file.edited": async ({ file }) => {
      if (!file) return
      const ledger = await readLedger()

      // 1) semantic diff against git HEAD for this file
      const diff = await callSin(client, "semantic_diff", {
        file_a: `git:HEAD:${file}`,
        file_b: file,
      })
      const risk = parseRisk(diff)
      ledger.risk = maxRisk(ledger.risk, risk)

      // 2) architectural debt snapshot
      const debt = await callSin(client, "architectural_debt", {})
      ledger.debt = parseDebt(debt)

      // any edit invalidates the previous green verification
      ledger.oracle = "unknown"
      if (!ledger.dirty.includes(file)) ledger.dirty.push(file)

      const note = `edited ${file} (risk=${risk}, debt=${ledger.debt})`
      ledger.notes.push(note)
      await writeLedger(ledger)

      // 3) ADW breaker: hard stop
      if (ENFORCE && ledger.debt >= DEBT_BREAKER) {
        throw new Error(
          `[SIN] ADW breaker tripped: debt ${ledger.debt} >= ${DEBT_BREAKER}. ` +
            `Stop adding code and refactor. Re-run architectural_debt after refactor.`,
        )
      }

      // 4) risk gate: warn loudly (does not stop the edit, stops "done")
      if (RISK_ORDER[risk] >= RISK_ORDER[RISK_BLOCK_LEVEL as RiskLevel]) {
        await client.session.log?.({
          level: "warn",
          message:
            `[SIN] High-risk change in ${file}. Justify it and run ` +
            `verify_tests before reporting done.`,
        })
      }
    },

    /**
     * Before a tool runs: if the agent tries to "finish" while the gate is not
     * green, intercept and force a verification first.
     */
    "tool.execute.before": async ({ tool }, output) => {
      if (!ENFORCE) return
      const name = (tool ?? "").toLowerCase()
      const isFinishSignal =
        name.includes("done") ||
        name.includes("finish") ||
        name.includes("complete")
      if (!isFinishSignal) return

      const ledger = await readLedger()
      if (ledger.dirty.length === 0) return

      if (ledger.oracle !== "pass") {
        throw new Error(
          `[SIN] Cannot report done: Oracle verification is "${ledger.oracle}". ` +
            `Files awaiting green verification: ${ledger.dirty.join(", ")}. ` +
            `Run the SIN "verify_tests" tool until it returns pass.`,
        )
      }
      // gate is green -> reset ledger for next task
      await writeLedger({ ...EMPTY_LEDGER })
    },

    /**
     * After a verification tool runs: record the Oracle verdict so the finish
     * gate can open. We watch for verify_tests / prove / verify_change results.
     */
    "tool.execute.after": async ({ tool }, output) => {
      const name = (tool ?? "").toLowerCase()
      const isVerify =
        name.includes("verify") || name.includes("prove") || name.includes("oracle")
      if (!isVerify) return

      const ledger = await readLedger()
      const verdict = parseOracle(output?.result ?? output)
      ledger.oracle = verdict
      if (verdict === "pass") {
        ledger.dirty = []
        ledger.risk = "low"
        ledger.notes.push("oracle: PASS")
      } else if (verdict === "fail") {
        ledger.notes.push("oracle: FAIL")
      }
      await writeLedger(ledger)
    },

    /**
     * Session idle: gentle reminder if there is unverified work on the table.
     */
    "session.idle": async () => {
      const ledger = await readLedger()
      if (ledger.dirty.length > 0 && ledger.oracle !== "pass") {
        await client.session.log?.({
          level: "info",
          message:
            `[SIN] ${ledger.dirty.length} file(s) edited without a green ` +
            `verification. Run verify_tests before finishing.`,
        })
      }
    },
  }
}

export default SinPlugin
