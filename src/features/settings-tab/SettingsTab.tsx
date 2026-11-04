import React, { useState } from 'react';
import { settingsStore, useSettingsStore } from '../../lib/stores/settingsStore';

export const SettingsTab: React.FC = () => {
  const settings = useSettingsStore();
  const [saving, setSaving] = useState(false);

  const toggle = async (key: 'discordRichPresence') => {
    if (saving) return;
    setSaving(true);
    try {
      await settingsStore.update({ [key]: !settings[key] });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: 22 }}>
      <div style={{ fontSize: 16, fontWeight: 700, marginBottom: 6 }}>Settings</div>
      <div style={{ color: 'var(--m)', fontSize: 12, marginBottom: 20 }}>
        Choose which app features are active.
      </div>
      <section
        style={{
          maxWidth: 520,
          border: '1px solid var(--b)',
          borderRadius: 8,
          padding: 16,
          display: 'flex',
          flexDirection: 'column',
          gap: 16,
        }}
      >
        {/* Discord Rich Presence */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <div style={{ fontSize: 13, fontWeight: 600 }}>Discord Rich Presence</div>
            <div style={{ color: 'var(--m)', fontSize: 11, marginTop: 4 }}>
              Show your current section and running bot count on Discord.
            </div>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={settings.discordRichPresence ?? true}
            aria-label="Discord Rich Presence"
            disabled={saving}
            onClick={() => toggle('discordRichPresence')}
            style={{
              width: 42,
              height: 24,
              padding: 3,
              border: 0,
              borderRadius: 12,
              background: settings.discordRichPresence ? 'var(--a, #7c5cff)' : 'var(--b, #555)',
              cursor: saving ? 'default' : 'pointer',
              opacity: saving ? 0.55 : 1,
            }}
          >
            <span
              style={{
                display: 'block',
                width: 18,
                height: 18,
                borderRadius: '50%',
                background: '#fff',
                transform: settings.discordRichPresence ? 'translateX(18px)' : 'translateX(0)',
                transition: 'transform 120ms ease',
              }}
            />
          </button>
        </div>
      </section>
    </div>
  );
};
