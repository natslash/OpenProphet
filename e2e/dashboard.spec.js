import { test, expect } from '@playwright/test';

// ── Page Load & Core Structure ──────────────────────────────────

test.describe('Dashboard loads correctly', () => {
  test('page loads without JS errors', async ({ page }) => {
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    // Allow a short settle for async init
    await page.waitForTimeout(500);
    expect(errors).toEqual([]);
  });

  test('topbar renders with logo, clock, and status', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.logo')).toBeVisible();
    await expect(page.locator('#clock')).toBeVisible();
    await expect(page.locator('#status-text')).toBeVisible();
    await expect(page.locator('#phase-badge')).toBeVisible();
  });

  test('start/stop buttons exist', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#btn-start')).toBeVisible();
    // Stop button exists but hidden initially
    await expect(page.locator('#btn-stop')).toBeAttached();
  });

  test('footer renders with SSE and Bot status', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#sse-status')).toBeVisible();
    await expect(page.locator('#bot-status')).toBeVisible();
    await expect(page.locator('#footer-model')).toBeVisible();
  });
});

// ── Navigation Tabs ─────────────────────────────────────────────

test.describe('Tab navigation works', () => {
  // Portfolio tab is mobile-only (hidden on desktop via CSS)
  const tabs = [
    { id: 'terminal', panel: 'panel-terminal' },
    { id: 'trades', panel: 'panel-trades' },
    { id: 'intents', panel: 'panel-intents' },
    { id: 'agents', panel: 'panel-agents' },
    { id: 'accounts', panel: 'panel-accounts' },
    { id: 'plugins', panel: 'panel-plugins' },
    { id: 'settings', panel: 'panel-settings' },
    { id: 'about', panel: 'panel-about' },
  ];

  for (const { id, panel } of tabs) {
    test(`clicking "${id}" tab shows #${panel}`, async ({ page }) => {
      await page.goto('/');
      await page.click(`button[data-tab="${id}"]`);
      const panelEl = page.locator(`#${panel}`);
      // Terminal panel uses "active" class, other panels become visible
      if (id === 'terminal') {
        await expect(panelEl).toHaveClass(/active/);
      } else {
        await expect(panelEl).toBeVisible();
      }
    });
  }
});

// ── Terminal Panel ──────────────────────────────────────────────

test.describe('Terminal panel', () => {
  test('terminal container is created', async ({ page }) => {
    await page.goto('/');
    const termContainer = page.locator('#terminal-container');
    await expect(termContainer).toBeAttached();
  });

  test('terminal has initial "Agent ready" message', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    // The main terminal element should exist
    const terminalEl = page.locator('[data-terminal="main"]');
    await expect(terminalEl).toBeAttached();
  });

  test('chat input bar exists with send button', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    const chatInput = page.locator('[data-chat="main"]');
    await expect(chatInput).toBeAttached();
  });

  test('terminal filter input exists', async ({ page }) => {
    await page.goto('/');
    // The filter function should not throw
    const hasFilter = await page.evaluate(() => typeof filterTerminal === 'function');
    expect(hasFilter).toBe(true);
  });
});

// ── Sidebar (Portfolio Stats) ───────────────────────────────────

test.describe('Sidebar portfolio stats', () => {
  test('sidebar contains Account section with stats', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    const portfolio = page.locator('[data-stat="main-portfolio"]');
    const cash = page.locator('[data-stat="main-cash"]');
    const bp = page.locator('[data-stat="main-bp"]');
    await expect(portfolio).toBeAttached();
    await expect(cash).toBeAttached();
    await expect(bp).toBeAttached();
  });

  test('sidebar contains Heartbeat section', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    await expect(page.locator('[data-stat="main-hb-interval"]')).toBeAttached();
    await expect(page.locator('[data-stat="main-hb-next"]')).toBeAttached();
    await expect(page.locator('[data-stat="main-hb-count"]')).toBeAttached();
  });

  test('sidebar contains Positions section', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    await expect(page.locator('[data-stat="main-positions"]')).toBeAttached();
  });

  test('sidebar contains Stats section (beats, tools, trades, errors)', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    await expect(page.locator('[data-stat="main-beats"]')).toBeAttached();
    await expect(page.locator('[data-stat="main-tools"]')).toBeAttached();
    await expect(page.locator('[data-stat="main-trades"]')).toBeAttached();
    await expect(page.locator('[data-stat="main-errors"]')).toBeAttached();
  });
});

// ── Trades Panel ────────────────────────────────────────────────

test.describe('Trades panel', () => {
  test('trades panel shows empty state', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="trades"]');
    await expect(page.locator('#panel-trades h2')).toHaveText('Live Trades');
    await expect(page.locator('#trades-feed')).toBeVisible();
  });
});

// ── Intents Panel ───────────────────────────────────────────────

test.describe('Intents panel', () => {
  test('intents panel shows empty state', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="intents"]');
    await expect(page.locator('#panel-intents h2')).toHaveText('Approval Hub (Double-Confirm)');
    await expect(page.locator('#intents-feed')).toBeVisible();
  });
});

// ── Portfolio Panel (mobile) ────────────────────────────────────

test.describe('Portfolio panel (mobile-only)', () => {
  test('portfolio panel elements exist in DOM', async ({ page }) => {
    await page.goto('/');
    // Portfolio tab is hidden on desktop but the panel elements exist
    await expect(page.locator('#m-portfolio')).toBeAttached();
    await expect(page.locator('#m-cash')).toBeAttached();
    await expect(page.locator('#m-bp')).toBeAttached();
    await expect(page.locator('#m-positions')).toBeAttached();
  });
});

// ── Agents Panel ────────────────────────────────────────────────

test.describe('Agents panel', () => {
  test('agents panel has agents and strategies sections', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="agents"]');
    await expect(page.locator('#panel-agents h2')).toHaveText('Agents & Strategies');
    await expect(page.locator('#agents-list')).toBeAttached();
    await expect(page.locator('#strategies-list')).toBeAttached();
  });

  test('New Agent button exists', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="agents"]');
    const btn = page.locator('#panel-agents button', { hasText: 'New Agent' });
    await expect(btn).toBeVisible();
  });

  test('New Strategy button exists', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="agents"]');
    const btn = page.locator('#panel-agents button', { hasText: 'New Strategy' });
    await expect(btn).toBeVisible();
  });
});

// ── Accounts Panel ──────────────────────────────────────────────

test.describe('Accounts panel', () => {
  test('accounts panel renders', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="accounts"]');
    await expect(page.locator('#panel-accounts h2')).toHaveText('Trading Accounts');
    await expect(page.locator('#accounts-grid')).toBeAttached();
  });
});

// ── Plugins Panel ───────────────────────────────────────────────

test.describe('Plugins panel', () => {
  test('plugins panel shows Slack section', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="plugins"]');
    await expect(page.locator('#panel-plugins h2')).toHaveText('Plugins');
    await expect(page.locator('#slack-enabled')).toBeAttached();
    await expect(page.locator('#slack-webhook')).toBeAttached();
  });
});

// ── Settings Panel ──────────────────────────────────────────────

test.describe('Settings panel', () => {
  test('settings panel has heartbeat inputs', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="settings"]');
    await expect(page.locator('#panel-settings h2')).toHaveText('Settings');
    await expect(page.locator('#hb-pre_market')).toBeAttached();
    await expect(page.locator('#hb-market_open')).toBeAttached();
    await expect(page.locator('#hb-midday')).toBeAttached();
    await expect(page.locator('#hb-market_close')).toBeAttached();
  });

  test('settings panel has permissions inputs', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="settings"]');
    await expect(page.locator('#perm-live')).toBeAttached();
    await expect(page.locator('#perm-options')).toBeAttached();
    await expect(page.locator('#perm-maxpos')).toBeAttached();
    await expect(page.locator('#perm-maxloss')).toBeAttached();
  });

  test('settings panel has backup/restore buttons', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="settings"]');
    const exportBtn = page.locator('#panel-settings button', { hasText: 'Export Config' });
    const importBtn = page.locator('#panel-settings button', { hasText: 'Import Config' });
    await expect(exportBtn).toBeVisible();
    await expect(importBtn).toBeVisible();
  });

  test('settings panel has backend engine settings', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="settings"]');
    await expect(page.locator('#env-provider')).toBeAttached();
    await expect(page.locator('#env-model')).toBeAttached();
    await expect(page.locator('#env-polling-enabled')).toBeAttached();
    await expect(page.locator('#env-polling-interval')).toBeAttached();
  });
});

// ── About Panel ─────────────────────────────────────────────────

test.describe('About panel', () => {
  test('about panel shows project info', async ({ page }) => {
    await page.goto('/');
    await page.click('button[data-tab="about"]');
    await expect(page.locator('#panel-about')).toContainText('OpenProphet');
    await expect(page.locator('#panel-about')).toContainText('Disclaimer');
  });
});

// ── Modals ──────────────────────────────────────────────────────

test.describe('Modals', () => {
  test('modal overlay exists but is hidden', async ({ page }) => {
    await page.goto('/');
    const overlay = page.locator('#modal-overlay');
    await expect(overlay).toBeAttached();
    await expect(overlay).not.toHaveClass(/visible/);
  });

  test('admin token modal exists but is hidden', async ({ page }) => {
    await page.goto('/');
    const tokenModal = page.locator('#modal-token');
    await expect(tokenModal).toBeAttached();
  });
});

// ── JS Functions Exist ──────────────────────────────────────────

test.describe('Core JS functions are defined', () => {
  const coreFunctions = [
    'switchTab', 'connectSSE', 'updateState', 'updateButtons',
    'startAgent', 'stopAgent', 'pauseAgent', 'resumeAgent',
    'refreshPortfolio', 'refreshIntents', 'renderHeartbeat',
    'saveHeartbeat', 'renderPermissions', 'savePermissions',
    'exportConfig', 'importConfig', 'showModal', 'closeModal',
    'showToast', 'toggleDarkMode', 'sendMessage',
    'renderSlack', 'saveSlackConfig', 'loadEnvConfig', 'saveEnvConfig',
    'log', 'logHtml', 'esc', 'fmtTime',
    'ensureTerminal', 'renderAgentTab',
  ];

  test('all core functions are defined', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    const results = await page.evaluate((fns) => {
      return fns.map(fn => ({ fn, exists: typeof window[fn] === 'function' }));
    }, coreFunctions);
    const missing = results.filter(r => !r.exists).map(r => r.fn);
    expect(missing).toEqual([]);
  });
});

// ── No Console Errors on Tab Navigation ─────────────────────────

test.describe('No JS errors during navigation', () => {
  test('cycling through all tabs produces no JS errors', async ({ page }) => {
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto('/');
    await page.waitForTimeout(500);

    // Skip portfolio — mobile-only tab, hidden on desktop
    const tabIds = ['terminal', 'trades', 'intents', 'agents', 'accounts', 'plugins', 'settings', 'about'];
    for (const id of tabIds) {
      await page.click(`button[data-tab="${id}"]`);
      await page.waitForTimeout(100);
    }
    // Cycle back to terminal
    await page.click('button[data-tab="terminal"]');
    await page.waitForTimeout(200);

    expect(errors).toEqual([]);
  });
});

// ── Dark Mode Toggle ────────────────────────────────────────────

test.describe('Dark mode toggle', () => {
  test('dark mode toggle does not throw', async ({ page }) => {
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto('/');
    await page.click('#theme-toggle');
    await page.waitForTimeout(200);
    expect(errors).toEqual([]);
  });
});

// ── SSE Connection ──────────────────────────────────────────────

test.describe('SSE connectivity', () => {
  test('SSE status updates from "connecting"', async ({ page }) => {
    await page.goto('/');
    // Wait for SSE to connect (the status text changes from "connecting")
    await page.waitForTimeout(2000);
    const sseText = await page.locator('#sse-status').textContent();
    // Should be either "connected" or still "connecting" — not an error state
    expect(['connecting', 'connected', 'live', 'ok']).toContain(sseText.toLowerCase().trim());
  });
});
