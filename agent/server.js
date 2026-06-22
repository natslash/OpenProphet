#!/usr/bin/env node

// Prophet Agent Web Server - SSE streaming dashboard + agent control
import 'dotenv/config';

if (process.env.ADMIN_TOKEN !== undefined) {
  console.error("FATAL: ADMIN_TOKEN is set in process.env. This breaks the 'human-only' security invariant.");
  console.error("Please remove it from .env and place it exclusively in .env.backend.");
  process.exit(1);
}

import express from 'express';
import cors from 'cors';

import axios from 'axios';
import * as http from 'http';
import * as https from 'https';
import fs from 'fs/promises';
import { existsSync } from 'fs';
import path from 'path';
import { spawn, execSync } from 'child_process';
import { fileURLToPath } from 'url';
import { EventEmitter } from 'events';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
import {
  loadConfig, getConfig, saveConfig,
  addAgent, updateAgent, removeAgent, setActiveAgent, getActiveAgent, getAgentById, getResolvedAgent,
  addStrategy, updateStrategy, removeStrategy,
  setActiveModel, getStrategyById,
  updateHeartbeat, getHeartbeatForPhase,
  updatePermissions, getPermissions,
  updatePlugin, getPlugin,
  getHeartbeatProfiles, getPhaseTimeRanges, applyHeartbeatProfile, updatePhaseTimeRange,
} from './config-store.js';

const PROJECT_ROOT = path.join(__dirname, '..');
const PORT = process.env.AGENT_PORT || 3737;
const TRADING_BOT_PORT = process.env.TRADING_BOT_PORT || '4534';
const TRADING_BOT_URL = process.env.TRADING_BOT_URL || `http://localhost:${TRADING_BOT_PORT}`;


// Pooled HTTP agent for Go backend calls — reuses TCP connections
const goHttpAgent = new http.Agent({ keepAlive: true, maxSockets: 10, keepAliveMsecs: 30000 });
const goAxios = axios.create({ baseURL: TRADING_BOT_URL, httpAgent: goHttpAgent, timeout: 5000 });

const app = express();
app.use(express.json({ limit: '1mb' }));

// ── Auth Middleware ────────────────────────────────────────────────
// Token-based auth. Set AGENT_AUTH_TOKEN env var to enable.
// Without it, server is open (for local dev). With it, all API routes require the token.
const AUTH_TOKEN = process.env.AGENT_AUTH_TOKEN || '';
function authMiddleware(req, res, next) {
  if (!AUTH_TOKEN) return next(); // no token configured = open access
  // Allow health check unauthenticated
  if (req.path === '/api/health') return next();
  // Check Authorization header or query param
  const header = req.headers.authorization;
  const token = header?.startsWith('Bearer ') ? header.slice(7) : req.query.token;
  if (token === AUTH_TOKEN) return next();
  res.status(401).json({ error: 'Unauthorized. Set Authorization: Bearer <token> header.' });
}
app.use('/api', authMiddleware);

// ── Go Backend Manager ─────────────────────────────────────────────
// Manages the Go trading bot lifecycle, supports restarting with different Alpaca keys
let goProc = null;
let goReady = false;

async function startGoBackend() {
  await stopGoBackend();

  const binaryPath = path.join(PROJECT_ROOT, 'prophet_bot');
  try {
    if (!existsSync(binaryPath)) {
      console.log('  Building Go binary...');
      execSync('go build -o prophet_bot ./cmd/bot', { cwd: PROJECT_ROOT, timeout: 60000 });
    }
  } catch (err) {
    console.error('  Failed to build Go binary:', err.message);
    return false;
  }

  const env = {
    ...process.env,
    PORT: TRADING_BOT_PORT,
    DATABASE_PATH: path.join(PROJECT_ROOT, 'data', 'prophet_trader.db'),
    ACTIVITY_LOG_DIR: path.join(PROJECT_ROOT, 'data', 'activity_logs'),
  };

  await fs.mkdir(path.dirname(env.DATABASE_PATH), { recursive: true });

  console.log(`  Starting Go backend...`);

  goProc = spawn(binaryPath, [], {
    cwd: PROJECT_ROOT,
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  goProc.stdout.on('data', (d) => {
    const msg = d.toString().trim();
    if (msg) {
      console.log(`  [go] ${msg}`);
      broadcast('agent_log', {
        message: msg,
        level: 'info',
        timestamp: new Date().toISOString()
      });
    }
  });
  goProc.stderr.on('data', (d) => {
    const msg = d.toString().trim();
    if (msg) {
      console.log(`  [go-err] ${msg}`);
      broadcast('agent_log', {
        message: msg,
        level: msg.toLowerCase().includes('error') ? 'error' : (msg.toLowerCase().includes('warn') ? 'warning' : 'info'),
        timestamp: new Date().toISOString()
      });
    }
  });
  goProc.on('exit', (code, signal) => {
    console.log(`  Go backend exited (code: ${code}, signal: ${signal})`);
    goReady = false;
    goProc = null;
    // Auto-restart on unexpected crash (not manual stop)
    if (code !== 0 && code !== null && signal !== 'SIGTERM' && signal !== 'SIGKILL') {
      console.log('  Go backend crashed — auto-restarting in 5s...');
      broadcast('agent_log', {
        message: 'Trading backend crashed — auto-restarting in 5s...',
        level: 'error',
        timestamp: new Date().toISOString(),
      });
      setTimeout(() => {
        startGoBackend();
      }, 5000);
    }
  });

  // Wait for health check
  goReady = false;
  for (let i = 0; i < 20; i++) {
    await new Promise(r => setTimeout(r, 500));
    try {
      await goAxios.get('/health', { timeout: 2000 });
      goReady = true;
      console.log(`  Go backend ready on port ${TRADING_BOT_PORT} `);
      broadcast('agent_log', {
        message: `Trading backend started for account "${account.name}" (${account.paper ? 'paper' : 'live'})`,
        level: 'success',
        timestamp: new Date().toISOString(),
      });
      return true;
    } catch {}
  }

  console.error('  Go backend failed to start within 10s');
  broadcast('agent_log', {
    message: 'Trading backend failed to start. Check logs.',
    level: 'error',
    timestamp: new Date().toISOString(),
  });
  return false;
}

app.post('/api/backend/restart', (req, res) => {
  if (goProc) {
    console.log('  Manual restart requested: killing Go backend...');
    goProc.removeAllListeners('exit');
    goProc.on('exit', () => {
      goReady = false;
      goProc = null;
      startGoBackend();
    });
    goProc.kill('SIGTERM');
  } else {
    startGoBackend();
  }
  res.json({ ok: true });
});

async function stopGoBackend() {
  if (goProc) {
    const pid = goProc.pid;
    goProc.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 1500));
    // Check if still alive
    try { process.kill(pid, 0); goProc.kill('SIGKILL'); } catch {}
    goProc = null;
    goReady = false;
    await new Promise(r => setTimeout(r, 500));
  }
  // Kill any orphaned Go backend on the port (but NOT our own Node process)
  const myPid = process.pid;
  try {
    const pids = execSync(`lsof -t -i :${TRADING_BOT_PORT} -sTCP:LISTEN 2>/dev/null`, { encoding: 'utf-8' }).trim();
    if (pids) {
      for (const pid of pids.split('\n')) {
        const p = parseInt(pid);
        if (p && p !== myPid) {
          try { process.kill(p, 'SIGTERM'); } catch {}
        }
      }
      await new Promise(r => setTimeout(r, 500));
    }
  } catch {}
}

// ── Load Config ────────────────────────────────────────────────────
await loadConfig();
// Migration skipped


// ── Agent Instance ─────────────────────────────────────────────────


const sseClients = new Set();

function getGoClient() {
  return goAxios;
}

function broadcast(event, data) {
  if (sseClients.size === 0) return; // skip serialization when no clients connected
  const msg = `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
  for (const client of sseClients) {
    client.write(msg);
  }
}

const EVENTS = [
  'status', 'agent_log', 'agent_text', 'beat_start', 'beat_end',
  'tool_call', 'tool_result', 'heartbeat_change', 'schedule', 'trade',
];


// ── Slack Notification Dispatcher ──────────────────────────────────
async function notifySlack(text) {
  try {
    const slack = getPlugin('slack');
    if (!slack?.enabled || !slack?.webhookUrl) return;
    await axios.post(slack.webhookUrl, {
      text,
      channel: slack.channel || undefined,
    }, { timeout: 5000 });
  } catch (err) {
    console.error('Slack notification failed:', err.message);
  }
}

function slackEnabled(event) {
  const slack = getPlugin('slack');
  return slack?.enabled && slack?.webhookUrl && slack?.notifyOn?.[event];
}



// ── SSE Endpoint ───────────────────────────────────────────────────
app.get('/api/events', async (req, res) => {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    'Connection': 'keep-alive',
    'Access-Control-Allow-Origin': '*',
  });
  
  try {
    const goPort = TRADING_BOT_PORT;
    const response = await fetch(`http://localhost:${goPort}/api/v1/agent/status`);
    const stateData = await response.json().catch(() => ({ running: false }));
    res.write(`event: state\ndata: ${JSON.stringify({ running: stateData.running })}\n\n`);
    res.write(`event: status\ndata: ${JSON.stringify({ status: stateData.running ? 'active' : 'stopped' })}\n\n`);
  } catch(e) {
    res.write(`event: state\ndata: ${JSON.stringify({ running: false, error: e.message })}\n\n`);
  }
  
  res.write(`event: config\ndata: ${JSON.stringify(safeConfig())}\n\n`);
  sseClients.add(res);
  req.on('close', () => sseClients.delete(res));
});

// ── Agent Control ──────────────────────────────────────────────────
app.post('/api/agent/start', async (req, res) => {
  try {
    const goPort = TRADING_BOT_PORT;
    const response = await fetch(`http://localhost:${goPort}/api/v1/agent/start`, { method: 'POST' });
    if (!response.ok) {
      const errData = await response.json().catch(() => ({}));
      throw new Error(errData.error || `Go backend returned ${response.status}`);
    }
    res.json({ ok: true, status: 'started' });
  } catch (err) { res.status(500).json({ error: err.message }); }
});

app.post('/api/agent/stop', async (req, res) => {
  try {
    const goPort = TRADING_BOT_PORT;
    const response = await fetch(`http://localhost:${goPort}/api/v1/agent/stop`, { method: 'POST' });
    if (!response.ok) {
      const errData = await response.json().catch(() => ({}));
      throw new Error(errData.error || `Go backend returned ${response.status}`);
    }
    res.json({ ok: true, status: 'stopped' });
  } catch (err) { res.status(500).json({ error: err.message }); }
});


// ── Chat / Terminal ──
app.post('/api/agent/message', async (req, res) => {
  try {
    const { message } = req.body;
    if (!message?.trim()) return res.status(400).json({ error: 'Message is required' });

    // Check for commands
    const trimmed = message.trim();
    const config = getConfig();
    const lowerTrimmed = trimmed.toLowerCase();
    
    // /help - show available commands
    if (lowerTrimmed === '/help' || lowerTrimmed === '/?') {
      const helpText = `Available commands:

/start - Start the agent heartbeat
/stop - Stop the agent heartbeat
/newagent - Create a new agent
/editagent <id> - Edit an existing agent
/agents - List all agents
/portfolios - Show portfolio and positions

Models: ${(config.models || []).length} available
Providers: ${[...new Set((config.models || []).map(m => m.id.split('/')[0]))].join(', ')}

Any other text will be sent to the agent.`;
      return res.json({ ok: true, text: helpText });
    }
    
    if (lowerTrimmed === '/stop') {
      try {
        await stopGoBackend();
        return res.json({ ok: true, text: 'Agent stopped manually.' });
      } catch (err) {
        return res.json({ ok: true, text: 'Failed to stop agent: ' + err.message });
      }
    }

    if (lowerTrimmed === '/start') {
      try {
        await startGoBackend();
        return res.json({ ok: true, text: 'Agent started.' });
      } catch (err) {
        return res.json({ ok: true, text: 'Failed to start agent: ' + err.message });
      }
    }

    // /newagent - open agent builder
    if (lowerTrimmed === '/newagent' || lowerTrimmed.startsWith('/newagent ')) {
      const models = config.models || [];
      const strategies = config.strategies || [];
      broadcast('agent_builder', {
        mode: 'create',
        models,
        strategies,
      });
      return res.json({ ok: true, builder: true });
    }
    
    // /editagent - open agent editor
    const editMatch = trimmed.match(/^\/editagent\s+(\S+)/);
    if (editMatch) {
      const agentId = editMatch[1];
      const agent = getAgentById(agentId);
      if (!agent) return res.status(404).json({ error: 'Agent not found' });
      const models = config.models || [];
      const strategies = config.strategies || [];
      broadcast('agent_builder', {
        mode: 'edit',
        agent,
        models,
        strategies,
      });
      return res.json({ ok: true, builder: true });
    }
    
    // /agents - list agents
    if (lowerTrimmed === '/agents') {
      const agents = config.agents || [];
      let msg = 'Available agents:\n';
      for (const a of agents) {
        msg += `\n- ${a.name} (${a.id})\n  Model: ${a.model || 'default'}\n  Strategy: ${a.strategyId || 'none'}\n`;
      }
      msg += '\nUse /editagent <id> to edit an agent';
      return res.json({ ok: true, text: msg });
    }

    if (lowerTrimmed === '/portfolio' || lowerTrimmed === '/portfolios') {
      try {
        const goPort = TRADING_BOT_PORT;
        const accRes = await fetch(`http://localhost:${goPort}/api/v1/account`);
        const accData = await accRes.json();
        const posRes = await fetch(`http://localhost:${goPort}/api/v1/positions`);
        const posData = await posRes.json();
        
        let msg = `📊 **Portfolio Status** (Account: ${accData.ID || 'Unknown'})\n`;
        msg += `💰 Net Liquidation: €${(accData.PortfolioValue || 0).toLocaleString(undefined, {minimumFractionDigits: 2})}\n`;
        msg += `💵 Available Cash: €${(accData.Cash || 0).toLocaleString(undefined, {minimumFractionDigits: 2})}\n`;
        msg += `🚀 Buying Power: €${(accData.BuyingPower || 0).toLocaleString(undefined, {minimumFractionDigits: 2})}\n\n`;
        
        msg += `📈 **Open Positions**:\n`;
        if (!posData || posData.length === 0) {
           msg += 'No open positions.\n';
        } else {
           for (const p of posData) {
             msg += `- ${p.Symbol}: ${p.Qty} shares @ €${p.AvgEntryPrice ? p.AvgEntryPrice.toFixed(2) : '0.00'}\n`;
           }
        }
        return res.json({ ok: true, text: msg });
      } catch (e) {
        return res.json({ ok: true, text: 'Failed to fetch portfolio data: ' + e.message });
      }
    }

    try {
      const goPort = TRADING_BOT_PORT;
      const goRes = await fetch(`http://localhost:${goPort}/api/v1/agent/message`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: trimmed })
      });
      if (!goRes.ok) {
        const d = await goRes.json().catch(() => ({}));
        throw new Error(d.error || 'Failed to send instruction to autonomous agent.');
      }
      return res.json({ ok: true, text: 'Instruction sent to autonomous agent.' });
    } catch (err) {
      return res.status(400).json({ error: err.message });
    }
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// ── Order Confirmation ─────────────────────────────────────────────
// When requireConfirmation is enabled, the MCP server checks /api/permissions
// and returns an error asking the agent to wait. The operator must approve via UI.
// This is enforced at the MCP permission layer (enforcePermissions function).
// The UI can show a confirmation prompt — for now, requireConfirmation
// makes the MCP server reject orders with a "requires confirmation" error.
// The agent will see this error and should report it to the operator.

app.post('/api/agent/heartbeat', async (req, res) => {
  const { seconds, reason } = req.body;
  if (!seconds || seconds < 30 || seconds > 3600) return res.status(400).json({ error: 'seconds must be 30-3600' });
  try {
    const goPort = TRADING_BOT_PORT;
    await fetch(`http://localhost:${goPort}/api/v1/agent/heartbeat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ seconds, reason })
    });
    res.json({ ok: true, seconds });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// ── Safe Config (strip secrets) ────────────────────────────────────
function safeConfig() {
  const cfg = { ...getConfig() };
  // Strip secret keys from accounts
  cfg.accounts = (cfg.accounts || []).map(a => ({ ...a, secretKey: a.secretKey ? '****' + a.secretKey.slice(-4) : '****' }));
  return cfg;
}

// ── Env & Config CRUD ────────────────────────────────────────────────────
app.get('/api/env', async (req, res) => {
  const env = await readEnv();
  res.json({
    LLM_POLLING_ENABLED: env.LLM_POLLING_ENABLED === 'true',
    LLM_POLLING_INTERVAL_SECS: parseInt(env.LLM_POLLING_INTERVAL_SECS || '3600', 10),
    LLM_PROVIDER: env.LLM_PROVIDER || 'anthropic',
    LLM_MODEL: env.LLM_MODEL || '',
    BEAT_ENABLED: env.BEAT_ENABLED === 'true',
    TRADING_ENABLED: env.TRADING_ENABLED === 'true'
  });
});

app.post('/api/env', async (req, res) => {
  const updates = req.body;
  const envUpdates = {};
  if (updates.LLM_POLLING_ENABLED !== undefined) envUpdates.LLM_POLLING_ENABLED = updates.LLM_POLLING_ENABLED ? 'true' : 'false';
  if (updates.LLM_POLLING_INTERVAL_SECS !== undefined) envUpdates.LLM_POLLING_INTERVAL_SECS = updates.LLM_POLLING_INTERVAL_SECS.toString();
  if (updates.LLM_PROVIDER !== undefined) envUpdates.LLM_PROVIDER = updates.LLM_PROVIDER;
  if (updates.LLM_MODEL !== undefined) envUpdates.LLM_MODEL = updates.LLM_MODEL;
  if (updates.BEAT_ENABLED !== undefined) envUpdates.BEAT_ENABLED = updates.BEAT_ENABLED ? 'true' : 'false';
  if (updates.TRADING_ENABLED !== undefined) envUpdates.TRADING_ENABLED = updates.TRADING_ENABLED ? 'true' : 'false';
  
  await writeEnv(envUpdates);
  for (const [k, v] of Object.entries(envUpdates)) {
    process.env[k] = v;
  }
  res.json({ ok: true });
});

app.get('/api/config', (req, res) => {
  res.json(safeConfig());
});

async function buildSystemPrompt(agentConfig, context = {}) {
  if (!agentConfig) return 'No agent configured.';
  let prompt = agentConfig.prompt || '';
  
  if (agentConfig.strategies && agentConfig.strategies.length > 0) {
    prompt += '\n\n## Strategy Rules\n';
    for (const strategyId of agentConfig.strategies) {
      const strategy = context.getStrategyById ? context.getStrategyById(strategyId) : null;
      if (strategy && strategy.rules) {
        prompt += `\n### ${strategy.name}\n${strategy.rules}\n`;
      }
    }
  }
  
  prompt += "\n\nCRITICAL CONTEXT:\n- Timezone: CET (Central European Time)\n- Base Currency: EUR (€)\nEnsure all price values, portfolio calculations, and temporal reasoning naturally default to Euros and CET without requiring manual prompting.";

  return prompt;
}

// System prompt preview
app.get('/api/agent/prompt-preview', async (req, res) => {
  try {
    const agentConfig = getResolvedAgent();
    const prompt = await buildSystemPrompt(agentConfig, { getStrategyById });
    res.json({ prompt, agentName: agentConfig.name });
  } catch (err) { res.status(500).json({ error: err.message }); }
});


// Agents
app.get('/api/agents', (req, res) => {
  const config = getConfig();
  res.json({ agents: config.agents, activeId: config.activeAgentId });
});

app.post('/api/agents', async (req, res) => {
  try {
    const agent = await addAgent(req.body);
    broadcast('config', safeConfig());
    res.json({ ok: true, agent });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.put('/api/agents/:id', async (req, res) => {
  try {
    const agent = await updateAgent(req.params.id, req.body);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true, agent });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.delete('/api/agents/:id', async (req, res) => {
  try {
    await removeAgent(req.params.id);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.post('/api/agents/:id/activate', async (req, res) => {
  try {
    await setActiveAgent(req.params.id);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// Strategies
app.get('/api/strategies', (req, res) => {
  const config = getConfig();
  res.json({ strategies: config.strategies });
});

app.post('/api/strategies', async (req, res) => {
  try {
    const strategy = await addStrategy(req.body);
    broadcast('config', safeConfig());
    res.json({ ok: true, strategy });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.put('/api/strategies/:id', async (req, res) => {
  try {
    const strategy = await updateStrategy(req.params.id, req.body);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true, strategy });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.delete('/api/strategies/:id', async (req, res) => {
  try {
    await removeStrategy(req.params.id);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// Model selection
app.get('/api/models', (req, res) => {
  const config = getConfig();
  const allModels = config.models || [];
  const provider = req.query.provider;
  const models = provider ? allModels.filter(m => m.id.startsWith(provider + '/')) : allModels;
  const allProviders = [...new Set(allModels.map(m => m.id.split('/')[0]))];
  const filteredProviders = provider ? [provider] : allProviders;
  res.json({ models, activeModel: config.activeModel, providers: filteredProviders, allProviders });
});

app.post('/api/models/activate', async (req, res) => {
  try {
    await setActiveModel(req.body.model);
    await startGoBackend();
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.post('/api/models/refresh', async (req, res) => {
  try {
    const out = execSync('opencode models 2>&1', { encoding: 'utf-8', timeout: 10000 });
    const lines = out.trim().split('\n').filter(l => l && l.includes('/'));
    const models = [];
    const seen = new Set();
    
    for (const line of lines) {
      const id = line.trim();
      if (!id || seen.has(id)) continue;
      
      // Filter models: No GPT, keep Gemini and select Claude models
      if (id.startsWith('openai/') || id.startsWith('openrouter/') || id.startsWith('opencode/')) continue;
      
      // Keep only up to 3 Claude variants (Opus, Sonnet, Haiku) and Gemini
      const isAllowed = [
        'anthropic/claude-3-opus',
        'anthropic/claude-3-5-sonnet',
        'anthropic/claude-3-haiku',
        'google/gemini-1.5-pro',
        'google/gemini-2.0-flash',
        'google/gemini-pro'
      ].some(allowed => id.startsWith(allowed));
      
      if (!isAllowed) continue;
      seen.add(id);
      
      let name = id;
      let description = '';
      
      if (id.startsWith('anthropic/')) {
        const model = id.replace('anthropic/', '');
        if (model.includes('opus')) {
          name = `Claude Opus ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Opus model (Powerful)';
        } else if (model.includes('sonnet')) {
          name = `Claude Sonnet ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Sonnet model (Balanced)';
        } else if (model.includes('haiku')) {
          name = `Claude Haiku ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Haiku model (Fast)';
        }
      } else if (id.startsWith('google/')) {
        name = 'Gemini ' + id.replace('google/', '').replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        description = 'Google Gemini model (Excellent for trading logic)';
      } else {
        name = id.split('/').pop().replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        description = 'Available model';
      }
      
      models.push({ id, name, description });
    }
    
    const config = getConfig();
    config.models = models;
    await saveConfig();
    broadcast('config', safeConfig());
    res.json({ ok: true, count: models.length });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// ── Heartbeat Config ───────────────────────────────────────────────
app.get('/api/heartbeat', (req, res) => {
  const config = getConfig();
  res.json(config.heartbeat || {});
});

app.put('/api/heartbeat', async (req, res) => {
  try {
    const heartbeatBody = req.body || {};
    await updateHeartbeat(heartbeatBody);
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.get('/api/heartbeat/profiles', (req, res) => {
  res.json({ profiles: getHeartbeatProfiles() });
});

app.post('/api/heartbeat/apply-profile', async (req, res) => {
  try {
    const { profile } = req.body || {};
    await applyHeartbeatProfile(profile);
    broadcast('config', safeConfig());
    res.json({ ok: true, profile });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.get('/api/heartbeat/phases', (req, res) => {
  res.json({ phases: getPhaseTimeRanges() });
});

app.put('/api/heartbeat/phases', async (req, res) => {
  try {
    const { phase, start, end } = req.body || {};
    if (!phase) throw new Error('Phase is required');
    await updatePhaseTimeRange(phase, { start, end });
    broadcast('config', safeConfig());
    res.json({ ok: true, phases: getPhaseTimeRanges() });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// ── Permissions / Guardrails ───────────────────────────────────────
app.get('/api/permissions', (req, res) => {
  res.json(getPermissions());
});

app.put('/api/permissions', async (req, res) => {
  try {
    const permBody = req.body || {};
    await updatePermissions(permBody);
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

// ── Plugins ────────────────────────────────────────────────────────
app.get('/api/plugins', (req, res) => {
  const config = getConfig();
  res.json(config.plugins || {});
});

app.get('/api/plugins/:name', (req, res) => {
  const plugin = getPlugin(req.params.name);
  res.json(plugin || {});
});

app.put('/api/plugins/:name', async (req, res) => {
  try {
    const pluginBody = req.body || {};
    await updatePlugin(req.params.name, pluginBody);
    broadcast('config', safeConfig());
    res.json({ ok: true });
  } catch (err) { res.status(400).json({ error: err.message }); }
});

app.post('/api/plugins/slack/test', async (req, res) => {
  try {
    const slack = getPlugin('slack');
    if (!slack?.webhookUrl) return res.status(400).json({ error: 'No Slack webhook URL configured' });
    const { default: axios } = await import('axios');
    await axios.post(slack.webhookUrl, {
      text: ':robot_face: *Prophet Agent* - Test notification\nSlack integration is working!',
      channel: slack.channel || undefined,
    }, { timeout: 5000 });
    res.json({ ok: true });
  } catch (err) { res.status(500).json({ error: 'Failed to send test message: ' + err.message }); }
});

// ── Portfolio Proxy ────────────────────────────────────────────────
app.get('/api/portfolio/account', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const { data } = await client.get('/api/v1/account');
    res.json(data);
  } catch { res.status(502).json({ error: 'Trading bot unavailable' }); }
});

app.get('/api/portfolio/positions', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const { data } = await client.get('/api/v1/positions');
    res.json(data);
  } catch { res.status(502).json({ error: 'Trading bot unavailable' }); }
});

app.get('/api/portfolio/orders', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const { data } = await client.get('/api/v1/orders');
    res.json(data);
  } catch { res.status(502).json({ error: 'Trading bot unavailable' }); }
});

// ── Intents Proxy ──────────────────────────────────────────────────
app.get('/api/intents', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const { data } = await client.get('/api/v1/beat/intents');
    res.json(data);
  } catch { res.status(502).json({ error: 'Trading bot unavailable' }); }
});

app.post('/api/intents/authorize/:id', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const authHeader = req.headers.authorization || '';
    const { data } = await client.post(`/api/v1/beat/authorize/${req.params.id}`, {}, {
      headers: { 'Authorization': authHeader }
    });
    res.json(data);
  } catch (err) {
    res.status(err.response?.status || 502).json(err.response?.data || { error: 'Failed to authorize intent' });
  }
});

app.post('/api/intents/reject/:id', async (req, res) => {
  try {
    const client = getGoClient();
    if (!client) return res.status(404).json({ error: 'Sandbox trading backend unavailable' });
    const authHeader = req.headers.authorization || '';
    const { data } = await client.post(`/api/v1/beat/reject/${req.params.id}`, {}, {
      headers: { 'Authorization': authHeader }
    });
    res.json(data);
  } catch (err) {
    res.status(err.response?.status || 502).json(err.response?.data || { error: 'Failed to reject intent' });
  }
});

// ── Auth (OpenCode) ────────────────────────────────────────────────
app.get('/api/auth/status', (req, res) => {
  // API key in env is the fastest check
  if (process.env.ANTHROPIC_API_KEY) {
    return res.json({
      loggedIn: true,
      authMethod: 'api_key',
      provider: 'opencode',
      raw: 'ANTHROPIC_API_KEY set in environment',
    });
  }
  try {
    const out = execSync('opencode auth list 2>&1', { timeout: 5000, encoding: 'utf-8' });
    // Parse the table output - look for "Anthropic" with "oauth" or any credential
    const hasAnthropicAuth = out.includes('Anthropic') && (out.includes('oauth') || out.includes('api-key'));
    res.json({
      loggedIn: hasAnthropicAuth,
      authMethod: hasAnthropicAuth ? 'opencode_oauth' : 'none',
      provider: 'opencode',
      raw: out.replace(/\x1b\[[0-9;]*m/g, '').trim(), // strip ANSI codes
    });
  } catch (err) {
    const output = (err.stdout || err.stderr || err.message || '').replace(/\x1b\[[0-9;]*m/g, '');
    res.json({ loggedIn: false, provider: 'opencode', raw: output.substring(0, 200) });
  }
});

app.post('/api/auth/login', (req, res) => {
  // Spawn opencode auth login and capture the URL
  const proc = spawn('opencode', ['auth', 'login'], {
    stdio: ['pipe', 'pipe', 'pipe'],
    env: { ...process.env, BROWSER: 'echo' }, // prevent auto-opening browser
  });

  let output = '';
  let urlSent = false;

  const sendUrl = (data) => {
    output += data.toString();
    // Look for any OAuth/auth URL
    const match = output.match(/(https:\/\/[^\s]+authorize[^\s]*)/);
    if (match && !urlSent) {
      urlSent = true;
      res.json({ ok: true, url: match[1] });
      proc.on('exit', (code) => {
        broadcast('agent_log', {
          message: code === 0 ? 'OpenCode authenticated successfully!' : 'Auth flow ended (code: ' + code + ')',
          level: code === 0 ? 'success' : 'warning',
          timestamp: new Date().toISOString(),
        });
      });
    }
  };

  proc.stdout.on('data', sendUrl);
  proc.stderr.on('data', sendUrl);

  // Also handle interactive prompts - pipe newline to accept defaults
  setTimeout(() => {
    try { proc.stdin.write('\n'); } catch {}
  }, 2000);

  // Timeout - if no URL found in 15s, return error
  setTimeout(() => {
    if (!urlSent) {
      proc.kill();
      res.status(500).json({ error: 'Timed out waiting for auth URL', output: output.substring(0, 500) });
    }
  }, 15000);
});

app.post('/api/auth/logout', (req, res) => {
  try {
    execSync('opencode auth logout 2>&1', { timeout: 10000, encoding: 'utf-8' });
    broadcast('agent_log', {
      message: 'OpenCode logged out.',
      level: 'info',
      timestamp: new Date().toISOString(),
    });
    res.json({ ok: true });
  } catch (err) {
    const output = err.stdout || err.stderr || err.message || '';
    res.status(500).json({ error: 'Logout failed: ' + output.substring(0, 200) });
  }
});

// ── Health ──────────────────────────────────────────────────────────
app.get('/api/health', async (req, res) => {
  let botHealthy = false;
  let botRunning = false;
  try {
    await goAxios.get('/health', { timeout: 3000 });
    botHealthy = true;
    const statRes = await goAxios.get('/api/v1/agent/status', { timeout: 3000 });
    botRunning = statRes.data?.running || false;
  } catch {}
  const account = getActiveAccount();
  res.json({
    agent: 'healthy',
    trading_bot: botHealthy ? 'healthy' : 'unavailable',
    trading_bot_managed: goProc !== null,
    activeAccount: account ? { name: account.name, paper: account.paper } : null,
    uptime: process.uptime(),
    state: { running: botRunning },
  });
});

// Serve static files (after API routes)
app.use(express.static(path.join(__dirname, 'public'), {
  setHeaders: (res, path) => {
    if (path.endsWith('.html')) {
      res.setHeader('Cache-Control', 'no-cache, no-store, must-revalidate');
      res.setHeader('Pragma', 'no-cache');
      res.setHeader('Expires', '0');
    }
  }
}));

// SPA fallback - serve index.html for non-API routes
app.use((req, res, next) => {
  if (!req.path.startsWith('/api/') && req.method === 'GET') {
    res.sendFile(path.join(__dirname, 'public', 'index.html'));
  } else {
    next();
  }
});

// ── Start Server ───────────────────────────────────────────────────

// Start Go backend
await startGoBackend();

// Graceful shutdown
process.on('SIGTERM', async () => {
  console.log('\n  Shutting down...');
  await stopGoBackend();
  process.exit(0);
});
process.on('SIGINT', async () => {
  console.log('\n  Shutting down...');
  await stopGoBackend();
  process.exit(0);
});

app.listen(PORT, '0.0.0.0', () => {
  console.log(`\n  Prophet Agent Dashboard: http://localhost:${PORT}`);
  console.log(`  Network:                http://0.0.0.0:${PORT}`);
  console.log(`  Trading Bot Backend:    ${TRADING_BOT_URL}\n`);
});

// Proxy logs from Go.
// The Go backend is (re)started on account switch, the restart button, crash
// auto-restart, and the daily IB Gateway restart. Each restart is a *new*
// process with a *new* /agent/stream, so this proxy must reconnect whenever the
// current stream ends or errors — otherwise agent_text and every other agent
// event silently stop reaching the dashboard after the first restart.
let _goLogsConnected = false;
function reconnectGoLogs() {
  if (!_goLogsConnected) return; // a (re)connect attempt is already scheduled/running
  _goLogsConnected = false;
  setTimeout(proxyGoLogs, 2000);
}

async function proxyGoLogs() {
  const goPort = TRADING_BOT_PORT;
  if (_goLogsConnected) return; // avoid overlapping streams
  _goLogsConnected = true;
  try {
    // Use axios with a streaming response (Node Readable). The previous code
    // did `import('node-fetch')`, but node-fetch is not a dependency, so the
    // import threw on every attempt and this proxy never connected — which is
    // why agent_text never reached the chat pane (only agent_log, via Go
    // stdout, did). axios is already a dependency and gives us a Node stream
    // with the .on('data'/'end'/'error') API used below.
    const res = await axios.get(`http://localhost:${goPort}/api/v1/agent/stream`, {
      responseType: 'stream',
    });
    const body = res.data;
    if (!body) {
      reconnectGoLogs();
      return;
    }
    let buffer = '';
    body.on('data', (chunk) => {
      buffer += chunk.toString();
      let newlineIndex;
      while ((newlineIndex = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, newlineIndex);
        buffer = buffer.slice(newlineIndex + 1);

        let dataStr = "";
        if (line.startsWith('data: ')) {
          dataStr = line.slice(6);
        } else if (line.startsWith('data:')) {
          dataStr = line.slice(5);
        } else if (line.trim().length > 0 && !line.startsWith('event:')) {
          dataStr = line.trim();
        }

        if (dataStr) {
          try {
            const d = JSON.parse(dataStr);
            if (d.event && d.data) {
              // Forward event directly
              broadcast(d.event, { ...d.data });
            } else {
              broadcast('log', dataStr);
            }
          } catch (e) {
            broadcast('log', dataStr);
          }
        }
      }
    });
    // Reconnect when the Go process restarts (stream ends/errors).
    body.on('end', reconnectGoLogs);
    body.on('close', reconnectGoLogs);
    body.on('error', reconnectGoLogs);
  } catch (err) {
    reconnectGoLogs();
  }
}
proxyGoLogs();
