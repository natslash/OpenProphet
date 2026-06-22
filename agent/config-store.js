// Persistent configuration store for agents, strategies, and settings
// Uses a JSON file for simplicity - no extra DB dependencies
import fs from 'fs/promises';
import path from 'path';
import { fileURLToPath } from 'url';
import crypto from 'crypto';
import { execSync } from 'child_process';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const CONFIG_PATH = path.join(__dirname, '..', 'data', 'agent-config.json');

const DEFAULT_HEARTBEAT = {
  pre_market: 900,
  market_open: 120,
  midday: 600,
  market_close: 120,
  after_hours: 1800,
  closed: 3600,
};

export const HEARTBEAT_PROFILES = {
  active: {
    label: 'Active Trading',
    description: 'High-frequency monitoring during market hours',
    phases: { pre_market: 300, market_open: 60, midday: 300, market_close: 60, after_hours: 600, closed: 1800 },
  },
  passive: {
    label: 'Passive Monitoring',
    description: 'Low-frequency check-ins, hands-off approach',
    phases: { pre_market: 1800, market_open: 600, midday: 900, market_close: 600, after_hours: 3600, closed: 7200 },
  },
  long_horizon: {
    label: 'Long Horizon',
    description: 'Weekly/monthly style check-ins for position management',
    phases: { pre_market: 7200, market_open: 3600, midday: 3600, market_close: 3600, after_hours: 7200, closed: 14400 },
  },
  earnings_season: {
    label: 'Earnings Season',
    description: 'Heightened vigilance during earnings periods',
    phases: { pre_market: 180, market_open: 30, midday: 120, market_close: 30, after_hours: 300, closed: 1800 },
  },
  overnight: {
    label: 'Overnight Hold',
    description: 'Set and forget with minimal overnight checks',
    phases: { pre_market: 900, market_open: 120, midday: 300, market_close: 120, after_hours: 7200, closed: 10800 },
  },
  scalp: {
    label: 'Scalp Mode',
    description: 'Rapid-fire execution for day trading',
    phases: { pre_market: 60, market_open: 15, midday: 30, market_close: 15, after_hours: 120, closed: 600 },
  },
};

export const PHASE_TIME_RANGES = {
  pre_market: { label: 'Pre-Market', start: 240, end: 570 },
  market_open: { label: 'Market Open', start: 570, end: 630 },
  midday: { label: 'Midday', start: 630, end: 900 },
  market_close: { label: 'Market Close', start: 900, end: 960 },
  after_hours: { label: 'After Hours', start: 960, end: 1200 },
  closed: { label: 'Markets Closed', start: null, end: null },
};

const DEFAULT_PERMISSIONS = {
  allowLiveTrading: true,
  maxPositionPct: 15,
  maxDeployedPct: 80,
  maxDailyLoss: 5,
  maxOpenPositions: 10,
  maxOrderValue: 0,
  allowedTools: [],
  blockedTools: [],
  allowOptions: true,
  allowStocks: true,
  allow0DTE: false,
  requireConfirmation: false,
  maxToolRoundsPerBeat: 25,
};

const DEFAULT_PLUGINS = {
  slack: {
    enabled: false,
    webhookUrl: '',
    channel: '',
    notifyOn: {
      tradeExecuted: true,
      agentStartStop: true,
      errors: true,
      dailySummary: true,
      positionOpened: true,
      positionClosed: true,
      heartbeat: false,
    },
  },
};

function defaultAgents() {
  return [
    {
      id: 'default',
      name: 'Prophet',
      description: 'Aggressive discretionary options trader with scalping overlay',
      systemPromptTemplate: 'default',
      strategyId: 'default',
      model: 'anthropic/claude-sonnet-4-6',
      heartbeatOverrides: {},
      customSystemPrompt: '',
      createdAt: new Date().toISOString(),
    },
    {
      id: 'conservative',
      name: 'Guardian',
      description: 'Conservative swing trader focused on capital preservation',
      systemPromptTemplate: 'custom',
      customSystemPrompt: `You are Guardian, a conservative AI trading agent. You prioritize capital preservation above all else.

## Rules
- Only take high-conviction setups with clear risk/reward > 3:1
- Maximum 5% of portfolio per position
- Maximum 30% deployed at any time (70%+ cash always)
- Only swing trades: 30-90 DTE, delta 0.40-0.60
- No scalping, no 0DTE, no earnings plays
- Stop loss at -10%, take profit at +30%
- Maximum 5 positions at once`,
      strategyId: null,
      model: 'anthropic/claude-sonnet-4-6',
      heartbeatOverrides: {
        pre_market: 1800,
        market_open: 300,
        midday: 900,
        market_close: 300,
        after_hours: 3600,
      },
      createdAt: new Date().toISOString(),
    },
  ];
}

function defaultStrategies() {
  return [
    {
      id: 'default',
      name: 'Aggressive Options',
      description: 'Multi-timeframe options with scalping overlay',
      rulesFile: 'TRADING_RULES.md',
      customRules: null,
      createdAt: new Date().toISOString(),
    },
  ];
}

function defaultModels() {
  try {
    const out = execSync('opencode models 2>&1', { encoding: 'utf-8', timeout: 10000 });
    const lines = out.trim().split('\n').filter(l => l && l.includes('/'));
    const models = [];
    const seen = new Set();
    
    for (const line of lines) {
      const id = line.trim();
      if (!id || seen.has(id)) continue;
      seen.add(id);
      
      let name = id;
      let description = '';
      
      if (id.startsWith('anthropic/')) {
        const model = id.replace('anthropic/', '');
        if (model.includes('opus')) {
          name = `Claude Opus ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Opus model';
        } else if (model.includes('sonnet')) {
          name = `Claude Sonnet ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Sonnet model';
        } else if (model.includes('haiku')) {
          name = `Claude Haiku ${model.replace(/[^\d.]/g, '')}`;
          description = 'Anthropic Haiku model';
        }
      } else if (id.startsWith('openai/')) {
        name = id.replace('openai/', '').replace(/-/g, ' ').replace(/\\b\\w/g, c => c.toUpperCase());
        description = 'OpenAI model';
      } else if (id.startsWith('google/')) {
        name = 'Gemini ' + id.replace('google/', '').replace(/-/g, ' ');
        description = 'Google model';
      } else if (id.startsWith('openrouter/')) {
        name = id.replace('openrouter/', '').replace(/:/g, ' ').replace(/-/g, ' ');
        description = 'OpenRouter model';
      } else if (id.startsWith('opencode/')) {
        name = id.replace('opencode/', '').replace(/-/g, ' ').replace(/\\b\\w/g, c => c.toUpperCase());
        description = 'OpenCode provider model';
      } else {
        name = id.split('/').pop().replace(/-/g, ' ').replace(/\\b\\w/g, c => c.toUpperCase());
        description = 'Available model';
      }
      
      models.push({ id, name, description });
    }
    
    if (models.length > 0) {
      console.log(`[config-store] Loaded ${models.length} models from opencode`);
      return models;
    }
  } catch (err) {
    console.log('[config-store] Could not load models from opencode, using defaults:', err.message);
  }
  
  return [
    { id: 'anthropic/claude-sonnet-4-6', name: 'Claude Sonnet 4.6', description: 'Best speed + intelligence, $3/$15 per MTok' },
    { id: 'anthropic/claude-opus-4-8', name: 'Claude Opus 4.8', description: 'Next generation Opus, $15/$75 per MTok' },
    { id: 'google/gemini-3-1-pro', name: 'Gemini 3.1 Pro', description: 'Google Advanced Pro Model' },
    { id: 'anthropic/claude-opus-4-6', name: 'Claude Opus 4.6', description: 'Most intelligent, best for agents, $5/$25 per MTok' },
    { id: 'anthropic/claude-haiku-4-5', name: 'Claude Haiku 4.5', description: 'Fastest, near-frontier, $1/$5 per MTok' },
    { id: 'anthropic/claude-sonnet-4-5', name: 'Claude Sonnet 4.5 (Legacy)', description: 'Previous gen Sonnet, $3/$15 per MTok' },
    { id: 'anthropic/claude-opus-4-5', name: 'Claude Opus 4.5 (Legacy)', description: 'Previous gen Opus, $5/$25 per MTok' },
    { id: 'anthropic/claude-sonnet-4-0', name: 'Claude Sonnet 4 (Legacy)', description: 'Original Sonnet 4, $3/$15 per MTok' },
    { id: 'anthropic/claude-opus-4-0', name: 'Claude Opus 4 (Legacy)', description: 'Original Opus 4, $15/$75 per MTok' },
  ];
}

function createDefaultConfig() {
  return {
    schemaVersion: 3,
    activeAgentId: 'default',
    activeModel: 'anthropic/claude-sonnet-4-6',
    heartbeat: { ...DEFAULT_HEARTBEAT },
    permissions: { ...DEFAULT_PERMISSIONS },
    plugins: mergePlugins(),
    agents: defaultAgents(),
    strategies: defaultStrategies(),
    models: defaultModels(),
  };
}

function mergePlugins(plugins = {}) {
  return {
    ...DEFAULT_PLUGINS,
    ...plugins,
    slack: {
      ...DEFAULT_PLUGINS.slack,
      ...(plugins.slack || {}),
      notifyOn: {
        ...DEFAULT_PLUGINS.slack.notifyOn,
        ...(plugins.slack?.notifyOn || {}),
      },
    },
  };
}

function normalizeConfig(raw = {}) {
  const defaults = createDefaultConfig();
  
  // If the user's config is older schema (v2) with sandboxes/accounts, migrate it on the fly:
  let migrated = { ...raw };
  if (raw.schemaVersion !== 3 && raw.sandboxes) {
    const defaultSbx = Object.values(raw.sandboxes)[0];
    if (defaultSbx) {
      migrated.activeAgentId = defaultSbx.agent?.activeAgentId || raw.activeAgentId || 'default';
      migrated.activeModel = defaultSbx.agent?.model || raw.activeModel || 'anthropic/claude-sonnet-4-6';
      migrated.heartbeat = { ...DEFAULT_HEARTBEAT, ...(defaultSbx.heartbeat || {}) };
      migrated.permissions = { ...DEFAULT_PERMISSIONS, ...(defaultSbx.permissions || {}) };
      migrated.plugins = mergePlugins(defaultSbx.plugins || {});
    }
  }

  const config = {
    ...defaults,
    ...migrated,
    schemaVersion: 3,
    heartbeat: { ...DEFAULT_HEARTBEAT, ...(migrated.heartbeat || {}) },
    permissions: { ...DEFAULT_PERMISSIONS, ...(migrated.permissions || {}) },
    plugins: mergePlugins(migrated.plugins || {}),
    agents: migrated.agents || defaults.agents,
    strategies: migrated.strategies || defaults.strategies,
    models: migrated.models || defaults.models,
  };
  
  // Remove vestigial fields
  delete config.sandboxes;
  delete config.accounts;
  delete config.manager;
  delete config.activeAccountId;
  delete config.activeSandboxId;

  return config;
}

let _config = null;
let _writeLock = Promise.resolve();

export async function loadConfig() {
  try {
    const raw = await fs.readFile(CONFIG_PATH, 'utf-8');
    _config = normalizeConfig(JSON.parse(raw));
  } catch (err) {
    if (err.code !== 'ENOENT') console.error('Warning: Failed to parse config file:', err.message);
    _config = createDefaultConfig();
  }

  await saveConfig();
  return _config;
}

export async function saveConfig() {
  _writeLock = _writeLock.then(async () => {
    await fs.mkdir(path.dirname(CONFIG_PATH), { recursive: true });
    await fs.writeFile(CONFIG_PATH, JSON.stringify(_config, null, 2));
  }).catch(err => console.error('Config save error:', err.message));
  return _writeLock;
}

export function getConfig() {
  if (!_config) throw new Error('Config not loaded. Call loadConfig() first.');
  return _config;
}

// ── Agents ─────────────────────────────────────────────────────────

export async function addAgent(agent) {
  const id = crypto.randomUUID().slice(0, 8);
  const newAgent = {
    id,
    name: agent.name || 'New Agent',
    description: agent.description || '',
    systemPromptTemplate: agent.systemPromptTemplate || 'custom',
    customSystemPrompt: agent.customSystemPrompt || '',
    strategyId: agent.strategyId || null,
    model: agent.model || _config.activeModel,
    heartbeatOverrides: agent.heartbeatOverrides || {},
    createdAt: new Date().toISOString(),
  };
  _config.agents.push(newAgent);
  await saveConfig();
  return newAgent;
}

export async function updateAgent(id, updates) {
  const idx = _config.agents.findIndex(a => a.id === id);
  if (idx === -1) throw new Error('Agent not found');
  _config.agents[idx] = { ..._config.agents[idx], ...updates, updatedAt: new Date().toISOString() };

  // If this is the active agent, propagate its model change to activeModel
  if (_config.activeAgentId === id && updates.model) {
    _config.activeModel = updates.model;
  }

  await saveConfig();
  return _config.agents[idx];
}

export async function removeAgent(id) {
  if (id === 'default') throw new Error('Cannot remove default agent');
  _config.agents = _config.agents.filter(a => a.id !== id);
  if (_config.activeAgentId === id) {
    _config.activeAgentId = 'default';
  }
  await saveConfig();
}

export async function setActiveAgent(id) {
  const agent = _config.agents.find(a => a.id === id);
  if (!agent) throw new Error('Agent not found');
  _config.activeAgentId = id;
  _config.activeModel = agent.model || _config.activeModel;
  await saveConfig();
}

export function getActiveAgent() {
  return getResolvedAgent() || _config.agents[0];
}

export function getAgentById(id) {
  return _config.agents.find(a => a.id === id) || null;
}

export function getResolvedAgent() {
  const baseAgent = getAgentById(_config.activeAgentId) || null;
  const resolved = {
    ...(baseAgent || {}),
    id: _config.activeAgentId,
    model: _config.activeModel || baseAgent?.model || 'anthropic/claude-sonnet-4-6',
    heartbeatOverrides: {
      ...(baseAgent?.heartbeatOverrides || {})
    },
  };
  return resolved;
}

// ── Strategies ─────────────────────────────────────────────────────

export async function addStrategy(strategy) {
  const id = crypto.randomUUID().slice(0, 8);
  const newStrategy = {
    id,
    name: strategy.name || 'New Strategy',
    description: strategy.description || '',
    rulesFile: null,
    customRules: strategy.customRules || '',
    createdAt: new Date().toISOString(),
  };
  _config.strategies.push(newStrategy);
  await saveConfig();
  return newStrategy;
}

export async function updateStrategy(id, updates) {
  const idx = _config.strategies.findIndex(s => s.id === id);
  if (idx === -1) throw new Error('Strategy not found');
  _config.strategies[idx] = { ..._config.strategies[idx], ...updates, updatedAt: new Date().toISOString() };
  await saveConfig();
  return _config.strategies[idx];
}

export async function removeStrategy(id) {
  if (id === 'default') throw new Error('Cannot remove default strategy');
  _config.strategies = _config.strategies.filter(s => s.id !== id);
  await saveConfig();
}

export function getStrategyById(id) {
  return _config.strategies.find(s => s.id === id) || null;
}

// ── Model ──────────────────────────────────────────────────────────

export async function setActiveModel(modelId) {
  _config.activeModel = modelId;
  await saveConfig();
}

// ── Heartbeat ──────────────────────────────────────────────────────

export async function updateHeartbeat(phaseIntervals) {
  _config.heartbeat = { ..._config.heartbeat, ...phaseIntervals };
  await saveConfig();
}

export function getHeartbeatForPhase(phase) {
  return _config.heartbeat?.[phase] || DEFAULT_HEARTBEAT[phase] || 600;
}

export function getHeartbeatProfiles() {
  return HEARTBEAT_PROFILES;
}

export function getPhaseTimeRanges() {
  return PHASE_TIME_RANGES;
}

export async function applyHeartbeatProfile(profileKey) {
  const profile = HEARTBEAT_PROFILES[profileKey];
  if (!profile) throw new Error(`Unknown heartbeat profile: ${profileKey}`);
  await updateHeartbeat(profile.phases);
}

export async function updatePhaseTimeRange(phase, range) {
  if (!PHASE_TIME_RANGES[phase]) throw new Error(`Unknown phase: ${phase}`);
  if (range.start !== undefined) PHASE_TIME_RANGES[phase].start = range.start;
  if (range.end !== undefined) PHASE_TIME_RANGES[phase].end = range.end;
}

// ── Permissions ───────────────────────────────────────────────────

export async function updatePermissions(perms) {
  _config.permissions = { ..._config.permissions, ...perms };
  await saveConfig();
}

export function getPermissions() {
  return _config.permissions || DEFAULT_PERMISSIONS;
}

// ── Plugins ────────────────────────────────────────────────────────

export async function updatePlugin(pluginName, pluginConfig) {
  _config.plugins = {
    ...(_config.plugins || {}),
    [pluginName]: { ...((_config.plugins || {})[pluginName] || {}), ...pluginConfig },
  };
  await saveConfig();
}

export function getPlugin(pluginName) {
  return _config.plugins?.[pluginName] || null;
}
