import test from 'node:test';
import assert from 'node:assert/strict';
import { botLogStore } from '../src/lib/stores/botLogStore';
import { settingsStore } from '../src/lib/stores/settingsStore';
import { emitLocalEvent, EVENT_NAMES } from '../src/lib/stores/eventBus';

test.beforeEach(async () => {
  // Clear any logs for test bots
  botLogStore.clearLogs('test-bot-1');
  botLogStore.clearLogs('test-bot-2');
  // Default to logs disabled
  await settingsStore.update({ enableLogs: false });
});

test.after(async () => {
  await settingsStore.update({ enableLogs: false });
});

test('1. When enableLogs is false, addLog is suppressed', () => {
  assert.equal(settingsStore.getSnapshot().enableLogs, false);

  botLogStore.addLog('test-bot-1', 'net', 'Connecting to game server...', 'info');
  botLogStore.addLog('test-bot-1', 'net', 'Bot connected to server. WebSocket session active.', 'info');
  botLogStore.addLog('test-bot-1', 'system', 'Bot disconnected from server.', 'warn');

  const snapshot = botLogStore.getSnapshot();
  const logs = snapshot.logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 0, 'No logs should be recorded when enableLogs is false');
});

test('2. When enableLogs is false, status_changed events do not generate NET or SYSTEM logs', () => {
  assert.equal(settingsStore.getSnapshot().enableLogs, false);

  emitLocalEvent(EVENT_NAMES.BOT_STATUS_CHANGED, {
    bot_id: 'test-bot-1',
    status: 'connecting',
    timestamp_ms: Date.now(),
  });

  emitLocalEvent(EVENT_NAMES.BOT_STATUS_CHANGED, {
    bot_id: 'test-bot-1',
    status: 'online',
    timestamp_ms: Date.now(),
  });

  const logs = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 0, 'Status changed events must not generate logs when enableLogs is false');
});

test('3. When enableLogs is true, addLog records log entries', async () => {
  await settingsStore.update({ enableLogs: true });
  assert.equal(settingsStore.getSnapshot().enableLogs, true);

  botLogStore.addLog('test-bot-1', 'net', 'Connecting to game server...', 'info');
  botLogStore.addLog('test-bot-1', 'net', 'Bot connected to server. WebSocket session active.', 'info');

  const logs = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 2, 'Logs should be recorded when enableLogs is true');
  assert.equal(logs[0].message, 'Connecting to game server...');
  assert.equal(logs[0].category, 'net');
  assert.equal(logs[1].message, 'Bot connected to server. WebSocket session active.');
  assert.equal(logs[1].category, 'net');
});

test('4. When enableLogs is true, status_changed events generate logs', async () => {
  await settingsStore.update({ enableLogs: true });

  emitLocalEvent(EVENT_NAMES.BOT_STATUS_CHANGED, {
    bot_id: 'test-bot-1',
    status: 'connecting',
    timestamp_ms: Date.now(),
  });

  emitLocalEvent(EVENT_NAMES.BOT_STATUS_CHANGED, {
    bot_id: 'test-bot-1',
    status: 'online',
    timestamp_ms: Date.now(),
  });

  const logs = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 2);
  assert.equal(logs[0].category, 'net');
  assert.equal(logs[0].message, 'Connecting to game server...');
  assert.equal(logs[1].category, 'net');
  assert.equal(logs[1].message, 'Bot connected to server. WebSocket session active.');
});

test('5. Toggling enableLogs from true to false freezes log entries', async () => {
  await settingsStore.update({ enableLogs: true });

  botLogStore.addLog('test-bot-1', 'combat', 'Attacking target...', 'info');
  assert.equal((botLogStore.getSnapshot().logsByBotId['test-bot-1'] || []).length, 1);

  // Toggle off
  await settingsStore.update({ enableLogs: false });

  // Subsequent logs must be suppressed
  botLogStore.addLog('test-bot-1', 'net', 'Connecting to game server...', 'info');
  emitLocalEvent(EVENT_NAMES.BOT_STATUS_CHANGED, {
    bot_id: 'test-bot-1',
    status: 'disconnected',
    timestamp_ms: Date.now(),
  });

  const logs = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 1, 'Previous logs remain intact, new logs suppressed');
  assert.equal(logs[0].message, 'Attacking target...');
});

test('6. Toggling enableLogs from false to true resumes logging', async () => {
  assert.equal(settingsStore.getSnapshot().enableLogs, false);

  botLogStore.addLog('test-bot-1', 'net', 'Ignored log', 'info');
  assert.equal((botLogStore.getSnapshot().logsByBotId['test-bot-1'] || []).length, 0);

  await settingsStore.update({ enableLogs: true });

  botLogStore.addLog('test-bot-1', 'net', 'Captured log', 'info');
  const logs = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  assert.equal(logs.length, 1);
  assert.equal(logs[0].message, 'Captured log');
});

test('7. Multiple bots log independently when enabled', async () => {
  await settingsStore.update({ enableLogs: true });

  botLogStore.addLog('test-bot-1', 'net', 'Bot 1 packet', 'info');
  botLogStore.addLog('test-bot-2', 'combat', 'Bot 2 attack', 'info');

  const logs1 = botLogStore.getSnapshot().logsByBotId['test-bot-1'] || [];
  const logs2 = botLogStore.getSnapshot().logsByBotId['test-bot-2'] || [];

  assert.equal(logs1.length, 1);
  assert.equal(logs1[0].message, 'Bot 1 packet');
  assert.equal(logs2.length, 1);
  assert.equal(logs2[0].message, 'Bot 2 attack');
});

test('8. clearLogs empties logs for the targeted bot', async () => {
  await settingsStore.update({ enableLogs: true });

  botLogStore.addLog('test-bot-1', 'net', 'Bot 1 msg', 'info');
  botLogStore.addLog('test-bot-2', 'net', 'Bot 2 msg', 'info');

  botLogStore.clearLogs('test-bot-1');

  assert.equal((botLogStore.getSnapshot().logsByBotId['test-bot-1'] || []).length, 0);
  assert.equal((botLogStore.getSnapshot().logsByBotId['test-bot-2'] || []).length, 1);
});
