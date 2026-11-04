import React, { useEffect, useState } from 'react';
import { BotSummary, ClassBuildConfig } from '../../lib/types/bot';
import { botStore } from '../../lib/stores/botStore';
import { Icon } from '../../components/Icon';
import { useMultiSelection, multiRunCommand } from '../../multi';

interface CharacterStatsPanelProps {
  bot?: BotSummary | null;
}

const CharacterStatsContent: React.FC<{ bot: BotSummary }> = ({ bot }) => {
  const { target, selectedCount } = useMultiSelection();
  const [add, setAdd] = useState({ str: 0, sta: 0, dex: 0, int: 0 });
  const [autoStat, setAutoStat] = useState<number>(bot.stats.auto_stat ?? 0);
  const [classBuild, setClassBuild] = useState<ClassBuildConfig | null>(null);
  const [isApplying, setIsApplying] = useState(false);
  const [statusMsg, setStatusMsg] = useState('');

  const curStr = bot.stats.str || 15;
  const curSta = bot.stats.sta || 15;
  const curDex = bot.stats.dex || 15;
  const curInt = bot.stats.int || 15;
  const totalFreePoints = bot.stats.stat_points || 0;
  const botJob = (bot.stats.job || 'vagrant').toLowerCase();

  const pendingPoints = add.str + add.sta + add.dex + add.int;
  const remainingPoints = totalFreePoints - pendingPoints;

  useEffect(() => {
    if (bot.stats.auto_stat !== undefined) {
      setAutoStat(bot.stats.auto_stat);
    }
  }, [bot.stats.auto_stat]);

  useEffect(() => {
    let mounted = true;
    botStore
      .fetchStatsJson()
      .then((builds) => {
        if (mounted && builds && builds[botJob]) {
          setClassBuild(builds[botJob]);
        }
      })
      .catch(() => {});
    return () => {
      mounted = false;
    };
  }, [botJob]);

  const handleStep = (key: 'str' | 'sta' | 'dex' | 'int', delta: number) => {
    if (delta > 0 && pendingPoints >= totalFreePoints) return;
    setAdd((prev) => ({
      ...prev,
      [key]: Math.max(0, prev[key] + delta),
    }));
  };

  const handleInput = (key: 'str' | 'sta' | 'dex' | 'int', valStr: string) => {
    const val = parseInt(valStr) || 0;
    const otherPending = pendingPoints - add[key];
    const maxAllowed = Math.max(0, totalFreePoints - otherPending);
    setAdd((prev) => ({
      ...prev,
      [key]: Math.min(maxAllowed, Math.max(0, val)),
    }));
  };

  const handleReset = () => {
    setAdd({ str: 0, sta: 0, dex: 0, int: 0 });
    setStatusMsg('');
  };

  const handleApply = async () => {
    if (pendingPoints <= 0 && autoStat === (bot.stats.auto_stat || 0) && selectedCount <= 1) {
      setStatusMsg('No points to apply.');
      return;
    }
    setIsApplying(true);
    setStatusMsg('Applying stats...');
    try {
      const res = await multiRunCommand(target, 'main.stats.allocate', {
        add_str: add.str,
        add_sta: add.sta,
        add_dex: add.dex,
        add_int: add.int,
        auto_stat: autoStat,
      });

      if (res.error) {
        setStatusMsg('Error: ' + res.error);
      } else if (res.summary.failed > 0) {
        const failReasons = res.summary.results
          .filter((r) => r.status === 2)
          .map((r) => `${r.botId}: ${r.reason || 'failed'}`)
          .join(', ');
        setStatusMsg(`Failed (${res.summary.failed}): ${failReasons}`);
      } else if (res.summary.skipped > 0) {
        const skipReasons = res.summary.results
          .filter((r) => r.status === 1)
          .map((r) => `${r.botId}: ${r.reason || 'skipped'}`)
          .join(', ');
        setStatusMsg(`Updated ${res.summary.ok}, Skipped ${res.summary.skipped} (${skipReasons})`);
        setAdd({ str: 0, sta: 0, dex: 0, int: 0 });
      } else {
        setAdd({ str: 0, sta: 0, dex: 0, int: 0 });
        setStatusMsg(
          selectedCount > 1
            ? `Stats saved for ${res.summary.ok} bots!`
            : 'Stats saved successfully!'
        );
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String((err as any)?.message || err);
      setStatusMsg('Error: ' + msg);
    } finally {
      setIsApplying(false);
    }
  };

  const handleAutoBuildFromStatsJson = async () => {
    if (selectedCount > 1) {
      setIsApplying(true);
      setStatusMsg('Auto-allocating for selected bots...');
      try {
        const res = await multiRunCommand(target, 'main.stats.autofill', {});
        if (res.error) {
          setStatusMsg('Error: ' + res.error);
        } else if (res.summary.failed > 0) {
          setStatusMsg(`Auto-fill: ${res.summary.ok} ok, ${res.summary.failed} failed`);
        } else if (res.summary.skipped > 0) {
          setStatusMsg(`Auto-fill: ${res.summary.ok} ok, ${res.summary.skipped} skipped (no points)`);
        } else {
          setStatusMsg(`Auto-filled stats for all ${res.summary.ok} bots!`);
        }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String((err as any)?.message || err);
        setStatusMsg('Error: ' + msg);
      } finally {
        setIsApplying(false);
      }
      return;
    }

    if (!classBuild) {
      setStatusMsg('No stats.json build found for ' + botJob);
      return;
    }
    if (totalFreePoints <= 0) {
      setStatusMsg('No free stat points available.');
      return;
    }

    if (classBuild.mode === 'cap') {
      const capVal = classBuild.cap_value || 43;
      const capStat = classBuild.cap_stat || 'dex';
      const thenStat = classBuild.then_stat || 'str';

      let curVal = curDex;
      if (capStat === 'str') curVal = curStr;
      else if (capStat === 'sta') curVal = curSta;
      else if (capStat === 'int') curVal = curInt;

      const needed = Math.max(0, capVal - curVal);
      const addCap = Math.min(totalFreePoints, needed);
      const rem = totalFreePoints - addCap;

      const newAdd = { str: 0, sta: 0, dex: 0, int: 0 };
      newAdd[capStat as keyof typeof newAdd] = addCap;
      newAdd[thenStat as keyof typeof newAdd] += rem;
      setAdd(newAdd);
      setAutoStat(classBuild.auto_stat);
      setStatusMsg(`Auto-filled: ${classBuild.description}`);
    } else if (classBuild.mode === 'pure') {
      const stat = classBuild.cap_stat || 'int';
      const newAdd = { str: 0, sta: 0, dex: 0, int: 0 };
      newAdd[stat as keyof typeof newAdd] = totalFreePoints;
      setAdd(newAdd);
      setAutoStat(classBuild.auto_stat);
      setStatusMsg(`Auto-filled Pure ${stat.toUpperCase()}`);
    }
  };

  const handleSaveToStatsJson = async () => {
    try {
      const allBuilds = await botStore.fetchStatsJson();
      const updatedBuilds = { ...allBuilds };

      let mode = 'pure';
      let capStat = 'str';
      let thenStat = 'str';
      let capVal = 0;

      if (add.dex > 0 && curDex + add.dex === 43) {
        mode = 'cap';
        capStat = 'dex';
        capVal = 43;
        thenStat = 'str';
      } else if (add.int >= add.str && add.int >= add.dex && add.int >= add.sta) {
        mode = 'pure';
        capStat = 'int';
        thenStat = 'int';
      } else if (add.dex >= add.str && add.dex >= add.sta) {
        mode = 'pure';
        capStat = 'dex';
        thenStat = 'dex';
      } else if (add.sta >= add.str) {
        mode = 'pure';
        capStat = 'sta';
        thenStat = 'sta';
      }

      updatedBuilds[botJob] = {
        job: botJob,
        mode,
        cap_stat: capStat,
        cap_value: capVal,
        then_stat: thenStat,
        auto_stat: autoStat,
        description: `Custom build for ${botJob}`,
      };

      await botStore.saveStatsJson(updatedBuilds);
      setClassBuild(updatedBuilds[botJob]);
      setStatusMsg('Saved build template to stats.json!');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String((err as any)?.message || err);
      setStatusMsg('Failed to save stats.json: ' + msg);
    }
  };

  const statsList = [
    { key: 'str' as const, label: 'STR', base: curStr, id: 1 },
    { key: 'sta' as const, label: 'STA', base: curSta, id: 2 },
    { key: 'dex' as const, label: 'DEX', base: curDex, id: 3 },
    { key: 'int' as const, label: 'INT', base: curInt, id: 4 },
  ];

  return (
    <div className="pn" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <h5>
        <Icon name="person" />
        Character Stats
        <span style={{ marginLeft: 'auto', fontSize: '10px', color: 'var(--sw)', fontWeight: 600 }}>
          {bot.stats.job || 'Vagrant'}
        </span>
      </h5>
      <div className="pb" style={{ padding: '9px 12px', overflowY: 'auto', flex: 1 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
          {statsList.map((st) => (
            <div
              key={st.key}
              style={{
                display: 'grid',
                gridTemplateColumns: '40px 32px 1fr 24px 24px 24px',
                alignItems: 'center',
                gap: '6px',
                fontSize: '11px',
              }}
            >
              <b style={{ color: '#74a6e8' }}>{st.label}</b>
              <span style={{ fontFamily: 'var(--mono)', fontWeight: 600 }}>{st.base}</span>
              <input
                type="number"
                className="inp"
                min={0}
                max={totalFreePoints}
                value={add[st.key]}
                onChange={(e) => handleInput(st.key, e.target.value)}
                style={{ height: '24px', textAlign: 'right', padding: '0 6px', fontSize: '11px' }}
              />
              <button
                type="button"
                className="b"
                style={{ height: '24px', padding: 0, fontWeight: 700, fontSize: '12px' }}
                onClick={() => handleStep(st.key, 1)}
                title={`Add 1 to ${st.label}`}
              >
                +
              </button>
              <button
                type="button"
                className="b"
                style={{ height: '24px', padding: 0, fontWeight: 700, fontSize: '12px' }}
                onClick={() => handleStep(st.key, -1)}
                title={`Subtract 1 from ${st.label}`}
              >
                -
              </button>
              <label
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                }}
                title={`Auto-Stat: Put all future points into ${st.label}`}
              >
                <input
                  type="radio"
                  name="auto_stat_grp"
                  checked={autoStat === st.id}
                  onChange={() => setAutoStat(st.id)}
                  style={{ accentColor: 'var(--y)', cursor: 'pointer' }}
                />
              </label>
            </div>
          ))}

          {/* Stat Points Row */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '6px 2px 4px',
              borderTop: '1px solid var(--ln)',
              marginTop: '4px',
              fontSize: '11px',
            }}
          >
            <span style={{ color: '#74a6e8', fontWeight: 600 }}>Stat Points</span>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <b style={{ fontFamily: 'var(--mono)', color: remainingPoints > 0 ? 'var(--y)' : 'var(--m)' }}>
                {remainingPoints}
              </b>
              <label
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  fontSize: '10.5px',
                  color: 'var(--m)',
                  cursor: 'pointer',
                }}
                title="No Auto-Stat: Keep future points unassigned"
              >
                <input
                  type="radio"
                  name="auto_stat_grp"
                  checked={autoStat === 0}
                  onChange={() => setAutoStat(0)}
                  style={{ accentColor: 'var(--y)', cursor: 'pointer' }}
                />
                <span>Manual</span>
              </label>
            </div>
          </div>

          {/* Apply & Reset Buttons */}
          <div style={{ display: 'flex', gap: '6px', margin: '4px 0' }}>
            <button
              type="button"
              className="b pri"
              style={{ flex: 1, height: '26px', fontWeight: 700, fontSize: '11px' }}
              disabled={isApplying}
              onClick={handleApply}
            >
              {isApplying ? 'Applying...' : 'Apply'}
            </button>
            <button
              type="button"
              className="b"
              style={{ flex: 1, height: '26px', fontSize: '11px' }}
              disabled={isApplying}
              onClick={handleReset}
            >
              Reset
            </button>
          </div>

          {/* Quick Helper Actions */}
          <div style={{ display: 'flex', gap: '6px' }}>
            <button
              type="button"
              className="b"
              style={{ flex: 1, height: '24px', fontSize: '10.5px', color: 'var(--sw)', padding: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
              onClick={handleAutoBuildFromStatsJson}
              title="Auto-fill points based on stats.json meta rule"
            >
              <Icon name="bolt" size={14} /> Auto-Fill
            </button>
            <button
              type="button"
              className="b"
              style={{ flex: 1, height: '24px', fontSize: '10.5px', padding: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
              onClick={handleSaveToStatsJson}
              title="Save current build rule to stats.json for this class"
            >
              <Icon name="save" size={14} /> Save Template
            </button>
          </div>

          {statusMsg && (
            <div
              style={{
                fontSize: '10.5px',
                color: statusMsg.startsWith('Error') ? 'var(--r)' : 'var(--g)',
                textAlign: 'center',
                marginTop: '2px',
              }}
            >
              {statusMsg}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export const CharacterStatsPanel: React.FC<CharacterStatsPanelProps> = ({ bot }) => {
  if (!bot) {
    return null;
  }
  return <CharacterStatsContent bot={bot} />;
};
