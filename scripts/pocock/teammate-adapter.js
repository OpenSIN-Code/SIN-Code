/** OpenSIN TeammateTool API Adapter
 * 
 * Standardizes peer-to-peer agent communication for Code-Swarm multi-agent orchestration.
 * Implements native Claude-Code-Swarm operations: spawnTeam, write, broadcast, approvePlan, etc.
 * 
 * Creates decentralized JSON-based inboxes under ~/.claude/tasks/{team-name}/
 * 
 * Usage:
 *   node teammate-adapter.js spawn <team-name> [description]
 *   node teammate-adapter.js write <team-name> <agent-id> <message>
 *   node teammate-adapter.js broadcast <team-name> <sender> <message>
 *   node teammate-adapter.js inbox <team-name> <agent-id>
 *   node teammate-adapter.js status <team-name>
 *   node teammate-adapter.js approve <team-name> <agent-id>
 *   node teammate-adapter.js shutdown <team-name>
 * 
 * Docs: teammate-adapter.doc.md
 */

const { mkdir, writeFile, readFile, readdir, rm } = require('fs/promises');
const { join, dirname } = require('path');
const { homedir } = require('os');

const CLAUDE_BASE_DIR = join(homedir(), '.claude');

class TeammateAdapter {
  constructor(teamName) {
    this.teamName = teamName;
    this.teamPath = join(CLAUDE_BASE_DIR, 'teams', teamName);
    this.tasksPath = join(CLAUDE_BASE_DIR, 'tasks', teamName);
    this.inboxesPath = join(this.teamPath, 'inboxes');
    this.globalPath = join(this.teamPath, 'global');
  }

  // ── 1. spawnTeam ───────────────────────────────────────────────────────
  async spawnTeam(description) {
    await mkdir(this.teamPath, { recursive: true });
    await mkdir(this.tasksPath, { recursive: true });
    await mkdir(this.inboxesPath, { recursive: true });
    await mkdir(this.globalPath, { recursive: true });

    const config = {
      team_name: this.teamName,
      description: description || "OpenSIN Autonomous Software Engineering Team",
      spawned_at: new Date().toISOString(),
      status: "active",
      members: [],
      task_counter: 0,
      message_counter: 0
    };

    await writeFile(
      join(this.teamPath, 'config.json'),
      JSON.stringify(config, null, 2)
    );

    await writeFile(
      join(this.tasksPath, 'tasks_queue.json'),
      JSON.stringify([], null, 2)
    );

    await writeFile(
      join(this.globalPath, 'broadcasts.json'),
      JSON.stringify([], null, 2)
    );

    console.log(`🐝 Team '${this.teamName}' erfolgreich gestartet.`);
    console.log(`📂 Team-Verzeichnis: ${this.teamPath}`);
    console.log(`📂 Task-Verzeichnis: ${this.tasksPath}`);
    console.log(`📂 Inbox-Verzeichnis: ${this.inboxesPath}`);
    return config;
  }

  // ── 2. write (P2P Message) ───────────────────────────────────────────
  async write(targetAgentId, messageValue, senderId = "team-lead") {
    const inboxDir = join(this.inboxesPath, targetAgentId);
    await mkdir(inboxDir, { recursive: true });

    const config = await this._readConfig();
    const messageId = `msg-${Date.now()}`;
    const message = {
      id: messageId,
      from: senderId,
      timestamp: new Date().toISOString(),
      body: messageValue,
      type: "direct",
      read: false,
      sequence: config.message_counter + 1
    };

    await writeFile(
      join(inboxDir, `${messageId}.json`),
      JSON.stringify(message, null, 2)
    );

    // Update global counter
    config.message_counter = message.sequence;
    await this._writeConfig(config);

    console.log(`📥 [P2P] Nachricht an '${targetAgentId}' zugestellt.`);
    console.log(`   ID: ${messageId}`);
    console.log(`   Von: ${senderId}`);
    return message;
  }

  // ── 3. broadcast (Multicast) ──────────────────────────────────────────
  async broadcast(senderName, broadcastValue) {
    console.warn(`💸 Sende Broadcast an alle Teammates (kostenintensiv!).`);
    
    const config = await this._readConfig();
    const messageId = `bcast-${Date.now()}`;
    const payload = {
      id: messageId,
      type: "broadcast",
      sender: senderName,
      timestamp: new Date().toISOString(),
      body: broadcastValue,
      sequence: config.message_counter + 1
    };

    // Write to global broadcasts
    const broadcastsPath = join(this.globalPath, 'broadcasts.json');
    let broadcasts = [];
    try {
      const data = await readFile(broadcastsPath, 'utf8');
      broadcasts = JSON.parse(data);
    } catch (e) {
      // File doesn't exist yet
    }
    broadcasts.push(payload);
    await writeFile(broadcastsPath, JSON.stringify(broadcasts, null, 2));

    // Write to each inbox
    try {
      const inboxes = await readdir(this.inboxesPath);
      for (const inbox of inboxes) {
        if (inbox === senderName) continue;
        await this.write(inbox, broadcastValue, senderName);
      }
    } catch (e) {
      // No inboxes yet
    }

    // Update counter
    config.message_counter = payload.sequence;
    await this._writeConfig(config);

    console.log(`📢 Broadcast gesendet: ${messageId}`);
    return payload;
  }

  // ── 4. inbox (Read messages) ──────────────────────────────────────────
  async inbox(agentId, markRead = true) {
    const inboxDir = join(this.inboxesPath, agentId);
    
    try {
      const files = await readdir(inboxDir);
      const messages = [];
      
      for (const file of files) {
        if (!file.endsWith('.json')) continue;
        const data = await readFile(join(inboxDir, file), 'utf8');
        const message = JSON.parse(data);
        messages.push(message);
        
        if (markRead && !message.read) {
          message.read = true;
          await writeFile(join(inboxDir, file), JSON.stringify(message, null, 2));
        }
      }

      messages.sort((a, b) => a.sequence - b.sequence);
      
      console.log(`📬 Inbox von '${agentId}': ${messages.length} Nachrichten`);
      for (const msg of messages) {
        const status = msg.read ? '✓' : '◯';
        console.log(`   ${status} [${msg.from}] ${msg.type}: ${msg.body.substring(0, 50)}...`);
      }

      return messages;
    } catch (e) {
      console.log(`📬 Inbox von '${agentId}': Leer`);
      return [];
    }
  }

  // ── 5. approvePlan ─────────────────────────────────────────────────────
  async approvePlan(agentId, planId, approved = true) {
    const inboxDir = join(this.inboxesPath, agentId);
    await mkdir(inboxDir, { recursive: true });

    const message = {
      id: `approval-${Date.now()}`,
      from: "team-lead",
      timestamp: new Date().toISOString(),
      type: "approval",
      plan_id: planId,
      approved: approved,
      body: approved ? "Plan genehmigt." : "Plan abgelehnt."
    };

    await writeFile(
      join(inboxDir, `${message.id}.json`),
      JSON.stringify(message, null, 2)
    );

    console.log(`${approved ? '✅' : '❌'} Plan '${planId}' für '${agentId}' ${approved ? 'genehmigt' : 'abgelehnt'}.`);
    return message;
  }

  // ── 6. approveShutdown ────────────────────────────────────────────────
  async approveShutdown(agentId, reason = "Task completed") {
    const inboxDir = join(this.inboxesPath, agentId);
    await mkdir(inboxDir, { recursive: true });

    const message = {
      id: `shutdown-${Date.now()}`,
      from: "team-lead",
      timestamp: new Date().toISOString(),
      type: "shutdown",
      reason: reason,
      body: `Shutdown genehmigt: ${reason}`
    };

    await writeFile(
      join(inboxDir, `${message.id}.json`),
      JSON.stringify(message, null, 2)
    );

    console.log(`🛑 Shutdown für '${agentId}' genehmigt: ${reason}`);
    return message;
  }

  // ── 7. status ────────────────────────────────────────────────────────
  async status() {
    try {
      const config = await this._readConfig();
      const inboxes = await readdir(this.inboxesPath).catch(() => []);
      
      console.log(`\n📊 Team Status: ${this.teamName}`);
      console.log(`   Status: ${config.status}`);
      console.log(`   Gestartet: ${config.spawned_at}`);
      console.log(`   Beschreibung: ${config.description}`);
      console.log(`   Agents: ${inboxes.length}`);
      console.log(`   Nachrichten: ${config.message_counter}`);
      
      for (const inbox of inboxes) {
        const messages = await this.inbox(inbox, false);
        const unread = messages.filter(m => !m.read).length;
        console.log(`   📬 ${inbox}: ${messages.length} msgs (${unread} ungelesen)`);
      }
      
      return config;
    } catch (e) {
      console.error(`❌ Team '${this.teamName}' nicht gefunden.`);
      throw e;
    }
  }

  // ── 8. requestJoin ─────────────────────────────────────────────────────
  async requestJoin(agentId, capabilities = []) {
    const inboxDir = join(this.inboxesPath, agentId);
    await mkdir(inboxDir, { recursive: true });

    const config = await this._readConfig();
    if (!config.members.includes(agentId)) {
      config.members.push(agentId);
      await this._writeConfig(config);
    }

    const message = {
      id: `join-${Date.now()}`,
      from: agentId,
      timestamp: new Date().toISOString(),
      type: "join_request",
      capabilities: capabilities,
      body: `Agent ${agentId} möchte beitreten.`
    };

    await writeFile(
      join(inboxDir, `${message.id}.json`),
      JSON.stringify(message, null, 2)
    );

    console.log(`🙋 Agent '${agentId}' möchte Team beitreten.`);
    console.log(`   Capabilities: ${capabilities.join(', ')}`);
    return message;
  }

  // ── 9. addTask ────────────────────────────────────────────────────────
  async addTask(taskName, description, assignee = null, dependencies = []) {
    const config = await this._readConfig();
    const taskId = `task-${Date.now()}`;
    const task = {
      id: taskId,
      name: taskName,
      description: description,
      assignee: assignee,
      dependencies: dependencies,
      status: "pending",
      created_at: new Date().toISOString(),
      sequence: config.task_counter + 1
    };

    // Read existing tasks
    const tasksPath = join(this.tasksPath, 'tasks_queue.json');
    let tasks = [];
    try {
      const data = await readFile(tasksPath, 'utf8');
      tasks = JSON.parse(data);
    } catch (e) {
      // File doesn't exist
    }
    tasks.push(task);
    await writeFile(tasksPath, JSON.stringify(tasks, null, 2));

    // Update counter
    config.task_counter = task.sequence;
    await this._writeConfig(config);

    console.log(`📋 Task hinzugefügt: ${taskName}`);
    console.log(`   ID: ${taskId}`);
    console.log(`   Assignee: ${assignee || 'unassigned'}`);
    return task;
  }

  // ── 10. shutdownTeam ──────────────────────────────────────────────────
  async shutdownTeam(reason = "Shutdown requested") {
    const config = await this._readConfig();
    config.status = "shutdown";
    config.shutdown_at = new Date().toISOString();
    config.shutdown_reason = reason;
    await this._writeConfig(config);

    console.log(`🛑 Team '${this.teamName}' heruntergefahren: ${reason}`);
    return config;
  }

  // ── Helper: read/write config ───────────────────────────────────────────
  async _readConfig() {
    const data = await readFile(join(this.teamPath, 'config.json'), 'utf8');
    return JSON.parse(data);
  }

  async _writeConfig(config) {
    await writeFile(
      join(this.teamPath, 'config.json'),
      JSON.stringify(config, null, 2)
    );
  }
}

// ── CLI Interface ─────────────────────────────────────────────────────────
async function main() {
  const args = process.argv.slice(2);
  const operation = args[0];
  const team = args[1] || 'default-swarm';

  const adapter = new TeammateAdapter(team);

  switch (operation) {
    case 'spawn':
      const description = args.slice(2).join(' ') || "OpenSIN Autonomous Team";
      await adapter.spawnTeam(description);
      break;

    case 'write':
      if (args.length < 4) {
        console.error('Usage: node teammate-adapter.js write <team> <agent-id> <message>');
        process.exit(1);
      }
      await adapter.write(args[2], args[3], args[4] || 'team-lead');
      break;

    case 'broadcast':
      if (args.length < 4) {
        console.error('Usage: node teammate-adapter.js broadcast <team> <sender> <message>');
        process.exit(1);
      }
      await adapter.broadcast(args[2], args[3]);
      break;

    case 'inbox':
      if (args.length < 3) {
        console.error('Usage: node teammate-adapter.js inbox <team> <agent-id>');
        process.exit(1);
      }
      await adapter.inbox(args[2]);
      break;

    case 'status':
      await adapter.status();
      break;

    case 'approve':
      if (args.length < 4) {
        console.error('Usage: node teammate-adapter.js approve <team> <agent-id> <plan-id>');
        process.exit(1);
      }
      await adapter.approvePlan(args[2], args[3]);
      break;

    case 'reject':
      if (args.length < 4) {
        console.error('Usage: node teammate-adapter.js reject <team> <agent-id> <plan-id>');
        process.exit(1);
      }
      await adapter.approvePlan(args[2], args[3], false);
      break;

    case 'shutdown':
      if (args.length < 3) {
        console.error('Usage: node teammate-adapter.js shutdown <team> [agent-id] [reason]');
        process.exit(1);
      }
      if (args[2]) {
        await adapter.approveShutdown(args[2], args[3] || 'Shutdown requested');
      } else {
        await adapter.shutdownTeam(args[2] || 'Shutdown requested');
      }
      break;

    case 'join':
      if (args.length < 3) {
        console.error('Usage: node teammate-adapter.js join <team> <agent-id> [capabilities...]');
        process.exit(1);
      }
      const capabilities = args.slice(3);
      await adapter.requestJoin(args[2], capabilities);
      break;

    case 'task':
      if (args.length < 4) {
        console.error('Usage: node teammate-adapter.js task <team> <task-name> <description> [assignee] [dependencies...]');
        process.exit(1);
      }
      const deps = args.slice(5);
      await adapter.addTask(args[2], args[3], args[4] || null, deps);
      break;

    default:
      console.log(`
OpenSIN Teammate Adapter - Multi-Agent Swarm Communication

Usage: node teammate-adapter.js <command> <team> [args...]

Commands:
  spawn <team> [description]        - Initialize team
  write <team> <agent> <msg>        - Send P2P message
  broadcast <team> <sender> <msg>   - Broadcast to all
  inbox <team> <agent>              - Read inbox
  status <team>                     - Show team status
  approve <team> <agent> <plan>     - Approve plan
  reject <team> <agent> <plan>      - Reject plan
  shutdown <team> [agent] [reason]  - Shutdown agent/team
  join <team> <agent> [caps...]     - Request to join
  task <team> <name> <desc> [assign] - Add task

Examples:
  node teammate-adapter.js spawn "my-team" "API Gateway Team"
  node teammate-adapter.js write "my-team" "agent-1" "Start implementation"
  node teammate-adapter.js broadcast "my-team" "team-lead" "All hands meeting"
  node teammate-adapter.js status "my-team"
  node teammate-adapter.js task "my-team" "Implement auth" "Add JWT" "agent-1"
      `);
  }
}

main().catch(err => {
  console.error('❌ Error:', err.message);
  process.exit(1);
});
