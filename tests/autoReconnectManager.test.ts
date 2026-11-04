import test from 'node:test';
import assert from 'node:assert/strict';
import { AutoReconnectManager, isEligibleForReconnect } from '../src/lib/services/autoReconnectManager';
import type { BotSummary } from '../src/lib/types/bot';

function createMockBot(id: string, status: string, autoReconnect = true, delay = 14, statusMessage = ''): BotSummary {
  return {
    id,
    name: `Bot_${id}`,
    character_name: `Char_${id}`,
    server: 'Server_1',
    status: status as any,
    status_message: statusMessage || status,
    stats: {} as any,
    intervals: {} as any,
    preferences: {
      bypass_logon: false,
      auto_reconnect: autoReconnect,
      auto_reconnect_delay: delay,
      auto_accept_party: false,
      anti_afk: false,
      auto_disconnect_on_gm: false,
      invisible: false,
    },
  };
}

test('1. Auto Reconnect OFF prevents automatic reconnection', async () => {
  let currentTime = 10000;
  let reconnected = false;
  const bot = createMockBot('bot-1', 'disconnected', false, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnected = true; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), null);

  // Advance time beyond 14 seconds
  currentTime += 20000;
  manager.tick();

  assert.equal(reconnected, false);
});

test('2. Auto Reconnect ON initializes the countdown', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);
});

test('3. Countdown decrements correctly every second', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'online', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);

  currentTime += 1000;
  assert.equal(manager.getRemainingSeconds('bot-1'), 13);

  currentTime += 5000;
  assert.equal(manager.getRemainingSeconds('bot-1'), 8);
});

test('4. Countdown reaching zero while DISCONNECTED triggers reconnect', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async (id) => {
      assert.equal(id, 'bot-1');
      reconnectCount++;
    },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);

  currentTime += 14000;
  manager.tick();

  assert.equal(reconnectCount, 1);
});

test('5. Countdown reaching zero while ONLINE skips reconnect', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'online', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);

  currentTime += 14000;
  manager.tick();

  assert.equal(reconnectCount, 0);
});

test('6. Countdown resets after every zero-count event', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'online', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);

  // Advance to 0
  currentTime += 14000;
  manager.tick();

  // Reset to full delay
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);

  // Ticks again
  currentTime += 1000;
  assert.equal(manager.getRemainingSeconds('bot-1'), 13);
});

test('7. CONNECTING skips reconnection', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'connecting', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  currentTime += 14000;
  manager.tick();

  assert.equal(reconnectCount, 0);
  assert.equal(isEligibleForReconnect(bot), false);
});

test('8. AUTHENTICATING skips reconnection', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'connecting', true, 14, 'Authenticating account via HTTP...');

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  currentTime += 14000;
  manager.tick();

  assert.equal(reconnectCount, 0);
  assert.equal(isEligibleForReconnect(bot), false);
});

test('9. DISCONNECTING skips reconnection', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'disconnecting', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  currentTime += 14000;
  manager.tick();

  assert.equal(reconnectCount, 0);
  assert.equal(isEligibleForReconnect(bot), false);
});

test('10. Configuration changes reset the countdown', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'disconnected', true, 30);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), 30);

  // 12 seconds pass (countdown at 18)
  currentTime += 12000;
  assert.equal(manager.getRemainingSeconds('bot-1'), 18);

  // User changes delay to 60 seconds
  bot.preferences.auto_reconnect_delay = 60;
  manager.syncBots([bot]);

  // Immediately resets active countdown to 60 seconds
  assert.equal(manager.getRemainingSeconds('bot-1'), 60);

  // Decrements from 60
  currentTime += 1000;
  assert.equal(manager.getRemainingSeconds('bot-1'), 59);
});

test('11. Disabling Auto Reconnect invalidates pending callbacks', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);

  // Disable auto reconnect
  bot.preferences.auto_reconnect = false;
  manager.syncBots([bot]);

  assert.equal(manager.getRemainingSeconds('bot-1'), null);

  // Advance time beyond 14s
  currentTime += 20000;
  manager.tick();

  assert.equal(reconnectCount, 0);
});

test('12. Re-enabling starts a fresh countdown', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  currentTime += 10000; // was at 4s

  // Turn OFF
  bot.preferences.auto_reconnect = false;
  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('bot-1'), null);

  // Turn ON again
  bot.preferences.auto_reconnect = true;
  manager.syncBots([bot]);

  // Fresh countdown from 14s, not 4s
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);
});

test('13. Multiple bots operate independently', async () => {
  let currentTime = 10000;
  const reconnectedBots: string[] = [];
  const botA = createMockBot('bot-A', 'disconnected', true, 14);
  const botB = createMockBot('bot-B', 'online', true, 30);
  const bots = [botA, botB];

  const manager = new AutoReconnectManager({
    reconnectFn: async (id) => { reconnectedBots.push(id); },
    botLookup: (id) => bots.find((b) => b.id === id),
    now: () => currentTime,
  });

  manager.syncBots(bots);
  assert.equal(manager.getRemainingSeconds('bot-A'), 14);
  assert.equal(manager.getRemainingSeconds('bot-B'), 30);

  // Advance 14s -> botA triggers, botB remains at 16s
  currentTime += 14000;
  manager.tick();

  assert.deepEqual(reconnectedBots, ['bot-A']);
  assert.equal(manager.getRemainingSeconds('bot-A'), 14); // reset
  assert.equal(manager.getRemainingSeconds('bot-B'), 16); // continuing
});

test('14. Manual reconnection does not cause unnecessary reconnect attempts', async () => {
  let currentTime = 10000;
  let reconnectCount = 0;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnectCount++; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);

  // At second 10, user manually reconnects bot
  currentTime += 10000;
  bot.status = 'online' as any;
  bot.status_message = 'Connected';

  // Countdown continues to 0
  currentTime += 4000;
  manager.tick();

  // Evaluates bot, sees it is ONLINE, skips reconnect
  assert.equal(reconnectCount, 0);
});

test('15. Failed reconnect attempts do not break the countdown cycle', async () => {
  let currentTime = 10000;
  let callCount = 0;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {
      callCount++;
      throw new Error('Network timeout');
    },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);

  // Cycle 1 hits 0
  currentTime += 14000;
  manager.tick();
  assert.equal(callCount, 1);
  await new Promise((r) => setTimeout(r, 0));

  // Timer resets and ticks normally for cycle 2
  assert.equal(manager.getRemainingSeconds('bot-1'), 14);
  currentTime += 14000;
  manager.tick();
  assert.equal(callCount, 2);
});

test('16. Bot removal cleans up scheduled callbacks', async () => {
  let currentTime = 10000;
  const botA = createMockBot('bot-A', 'disconnected', true, 14);
  const botB = createMockBot('bot-B', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: (id) => (id === 'bot-B' ? botB : undefined),
    now: () => currentTime,
  });

  manager.syncBots([botA, botB]);
  assert.equal(manager.getRemainingSeconds('bot-A'), 14);

  // Remove botA
  manager.syncBots([botB]);
  assert.equal(manager.getRemainingSeconds('bot-A'), null);
  assert.equal(manager.getState('bot-A'), undefined);
});

test('17. Application shutdown properly cleans up resources', async () => {
  const bot = createMockBot('bot-1', 'disconnected', true, 14);
  const manager = new AutoReconnectManager();

  manager.syncBots([bot]);
  assert.equal(manager.getState('bot-1')?.enabled, true);

  manager.destroy();
  assert.equal(manager.getRemainingSeconds('bot-1'), null);
  assert.equal(manager.getState('bot-1'), undefined);
});

test('18. Timer drift does not cause incorrect countdown behavior', async () => {
  let currentTime = 10000;
  const bot = createMockBot('bot-1', 'disconnected', true, 14);

  const manager = new AutoReconnectManager({
    reconnectFn: async () => {},
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);

  // UI stall for 3.5 seconds
  currentTime += 3500;
  // Deadline-based calculation calculates exact ceil((14000 - 3500)/1000) = 11s
  assert.equal(manager.getRemainingSeconds('bot-1'), 11);

  // UI stall for 8.2 seconds (total 11.7s elapsed)
  currentTime += 8200;
  assert.equal(manager.getRemainingSeconds('bot-1'), 3);
});

test('19. Recoverable statuses (channel_full, server_issue, duplicate_login, paused, object error) are eligible and reconnect', async () => {
  const recoverableStatuses = [
    'channel_full',
    'server_issue',
    'duplicate_login',
    'paused',
    'error',
    'disconnected',
  ];

  for (const status of recoverableStatuses) {
    let currentTime = 10000;
    let reconnected = false;
    const bot = createMockBot('bot-rec', status, true, 14);

    const manager = new AutoReconnectManager({
      reconnectFn: async (id) => {
        assert.equal(id, 'bot-rec');
        reconnected = true;
      },
      botLookup: () => bot,
      now: () => currentTime,
    });

    manager.syncBots([bot]);
    assert.equal(isEligibleForReconnect(bot), true, `status ${status} should be eligible`);

    currentTime += 14000;
    manager.tick();

    assert.equal(reconnected, true, `status ${status} should trigger reconnect`);
  }
});

test('20. Stale authenticating status message on disconnected bot is eligible for reconnect', async () => {
  let currentTime = 10000;
  let reconnected = false;
  const bot = createMockBot('bot-stale', 'disconnected', true, 14, 'Authenticating account via HTTP...');

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnected = true; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(isEligibleForReconnect(bot), true);

  currentTime += 14000;
  manager.tick();
  assert.equal(reconnected, true);
});

test('21. syncBots does not clobber explicitly enabled in-memory bot state when preferences.auto_reconnect is undefined', async () => {
  let currentTime = 10000;
  let reconnected = false;
  // Bot with undefined preferences.auto_reconnect
  const bot: BotSummary = {
    id: 'rhisenix',
    name: 'rhisenix',
    character_name: 'rhisenix',
    server: 'Channel 4',
    status: 'channel_full',
    status_message: 'Channel is Full',
    stats: {} as any,
    intervals: {} as any,
    preferences: {} as any,
  };

  const manager = new AutoReconnectManager({
    reconnectFn: async () => { reconnected = true; },
    botLookup: () => bot,
    now: () => currentTime,
  });

  // User toggles Auto Reconnect ON in UI
  manager.setBotEnabled('rhisenix', true);
  manager.setBotDelay('rhisenix', 30);
  assert.equal(manager.getRemainingSeconds('rhisenix'), 30);

  // Background status/stats update arrives, triggering syncBots with botStore snapshot
  manager.syncBots([bot]);

  // Must NOT be clobbered back to null!
  assert.equal(manager.getRemainingSeconds('rhisenix'), 30);

  // Ticks down over 30 seconds
  currentTime += 10000;
  manager.tick();
  assert.equal(manager.getRemainingSeconds('rhisenix'), 20);

  currentTime += 20000;
  manager.tick();
  assert.equal(manager.getRemainingSeconds('rhisenix'), 30); // reset to delay
  assert.equal(reconnected, true);
});

test('22. Channel is Full bot with 30s delay counts down and triggers reconnectFn', async () => {
  let currentTime = 50000;
  let reconnected = false;
  const bot = createMockBot('rhisenix', 'channel_full', true, 30, 'Channel is Full');

  const manager = new AutoReconnectManager({
    reconnectFn: async (id) => {
      assert.equal(id, 'rhisenix');
      reconnected = true;
    },
    botLookup: () => bot,
    now: () => currentTime,
  });

  manager.syncBots([bot]);
  assert.equal(manager.getRemainingSeconds('rhisenix'), 30);

  // 1 second elapsed
  currentTime += 1000;
  manager.tick();
  assert.equal(manager.getRemainingSeconds('rhisenix'), 29);

  // 29 more seconds elapsed
  currentTime += 29000;
  manager.tick();
  assert.equal(reconnected, true);
});
