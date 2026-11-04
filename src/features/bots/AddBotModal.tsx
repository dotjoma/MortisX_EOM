import React, { useEffect, useMemo, useRef, useState } from 'react';
import { ToggleSwitch } from '../shared/ToggleSwitch';
import { Modal } from '../shared/Modal';
import { ProgressBar } from '../shared/ProgressBar';
import { botStore } from '../../lib/stores/botStore';
import { useChannels } from '../../lib/hooks/useChannels';

interface AddBotModalProps {
  isOpen: boolean;
  onClose: () => void;
}

const SYLLABLES_START = [
  'flar', 'saint', 'dark', 'rhis', 'luda', 'law', 'keil', 'aris', 'madr', 'vyn', 'ael', 'bran', 'kor', 'nym',
];
const SYLLABLES_MID = ['ri', 'gal', 'ine', 'on', 'en', 'os', 'el', 'an', 'da'];
const SYLLABLES_END = ['ff', 'or', 'ix', 'on', 'er', 'en', 'is', 'ar', 'ia'];
const LETTERS = 'abcdefghijklmnopqrstuvwxyz';

const SECOND_JOBS_PREVIEW: Record<string, Array<{ id: string; name: string }>> = {
  assist: [
    { id: 'ringmaster', name: 'Ringmaster' },
    { id: 'billposter', name: 'Billposter' },
  ],
  mercenary: [
    { id: 'blade', name: 'Blade' },
    { id: 'knight', name: 'Knight' },
  ],
  acrobat: [
    { id: 'jester', name: 'Jester' },
    { id: 'ranger', name: 'Ranger' },
  ],
  magician: [
    { id: 'psykeeper', name: 'Psykeeper' },
    { id: 'elementor', name: 'Elementor' },
  ],
};

function generateRandomPassword(length = 12): string {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%';
  let pass = '';
  for (let i = 0; i < length; i++) {
    pass += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return pass;
}

function createRng(initialSeed: number) {
  let s = initialSeed | 0;
  return function () {
    s = (s + 0x6d2b79f5) | 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export const AddBotModal: React.FC<AddBotModalProps> = ({ isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'login' | 'register'>('login');
  const { channels } = useChannels(isOpen);
  const channelList = useMemo(() => channels.map((c) => c.name), [channels]);

  // Login form state
  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [loginServer, setLoginServer] = useState('Channel 2 (Safe - No Giants)');

  // Register form state
  const [autoRegister, setAutoRegister] = useState(false);
  const [singleUsername, setSingleUsername] = useState('');
  const [registerPassword, setRegisterPassword] = useState('');
  const [registerServer, setRegisterServer] = useState('Channel 2 (Safe - No Giants)');

  // Target Job choices
  const [target1stJob, setTarget1stJob] = useState('assist');
  const [target2ndJob, setTarget2ndJob] = useState('ringmaster');

  // Auto Register options
  const [nameStyle, setNameStyle] = useState<'num' | 'read' | 'rand'>('num');
  const [accountCount, setAccountCount] = useState(10);
  const [startAt, setStartAt] = useState(1);
  const [prefix, setPrefix] = useState('bot');
  const [digits, setDigits] = useState(3);
  const [letterCase, setLetterCase] = useState<'low' | 'cap' | 'up'>('low');
  const [seedBase, setSeedBase] = useState(12345);

  // Execution state
  const [isRunning, setIsRunning] = useState(false);
  const [progress, setProgress] = useState<{ current: number; total: number } | null>(null);
  const [statusText, setStatusText] = useState('');
  const [statusType, setStatusType] = useState<'normal' | 'ok' | 'err'>('normal');
  const [failures, setFailures] = useState<Array<{ name: string; reason: string }>>([]);
  const [showFailures, setShowFailures] = useState(false);

  const abortRef = useRef<boolean>(false);

  // Generate names deterministically based on seedBase
  const generatedNames = useMemo(() => {
    const n = Math.max(1, Math.min(500, accountCount));
    const rng = createRng(seedBase);
    const out: string[] = [];
    const used = new Set<string>();

    if (nameStyle === 'num') {
      const p = prefix.trim() || 'bot';
      for (let i = 0; i < n; i++) {
        const numStr = String(startAt + i).padStart(digits, '0');
        out.push(`${p}${numStr}`);
      }
    } else {
      let attempts = 0;
      while (out.length < n && attempts++ < 10000) {
        let w = '';
        if (nameStyle === 'read') {
          const s = SYLLABLES_START[Math.floor(rng() * SYLLABLES_START.length)];
          const m = SYLLABLES_MID[Math.floor(rng() * SYLLABLES_MID.length)];
          const e = SYLLABLES_END[Math.floor(rng() * SYLLABLES_END.length)];
          w = `${s}${m}${e}`;
        } else {
          for (let j = 0; j < 6; j++) {
            w += LETTERS.charAt(Math.floor(rng() * 26));
          }
        }
        if (!used.has(w)) {
          used.add(w);
          out.push(w);
        }
      }
    }

    return out.map((name) => {
      if (letterCase === 'cap') {
        return name.charAt(0).toUpperCase() + name.slice(1).toLowerCase();
      }
      if (letterCase === 'up') {
        return name.toUpperCase();
      }
      return name.toLowerCase();
    });
  }, [accountCount, nameStyle, startAt, prefix, digits, letterCase, seedBase]);

  // Preview formatted text
  const previewSummary = useMemo(() => {
    const len = generatedNames.length;
    if (len <= 4) {
      return `${len} accounts: ${generatedNames.join(', ')}`;
    }
    return `${len} accounts: ${generatedNames.slice(0, 4).join(', ')} … ${generatedNames[len - 1]}`;
  }, [generatedNames]);

  // Reset or close on Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (!isRunning) {
          onClose();
        }
      }
    };
    if (isOpen) {
      window.addEventListener('keydown', handleKeyDown);
    }
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, isRunning, onClose]);

  const handleShuffle = () => {
    setSeedBase((Math.random() * 1e9) | 0);
  };

  const handleCountChange = (delta: number) => {
    setAccountCount((prev) => Math.max(1, Math.min(500, prev + delta)));
  };

  const handleLoginSubmit = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    setStatusText('');
    setStatusType('normal');

    const username = loginUsername.trim();
    if (!username) {
      setStatusText('Enter a username.');
      setStatusType('err');
      return;
    }
    if (!loginPassword) {
      setStatusText('Enter a password.');
      setStatusType('err');
      return;
    }

    try {
      await botStore.loginBot({
        username,
        password: loginPassword,
        server: loginServer,
        target_1st_job: target1stJob,
        target_2nd_job: target2ndJob,
      });
      onClose();
    } catch (err: unknown) {
      const msg = err && typeof err === 'object' && 'message' in err ? String(err.message) : 'Failed to add bot';
      setStatusText(msg);
      setStatusType('err');
    }
  };

  const handleRegisterSubmit = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    setStatusText('');
    setStatusType('normal');
    setFailures([]);
    setShowFailures(false);

    if (!autoRegister && !registerPassword) {
      setStatusText('Enter a password.');
      setStatusType('err');
      return;
    }

    const targetList = autoRegister ? generatedNames : [singleUsername.trim()];
    if (!targetList[0]) {
      setStatusText('Enter a username.');
      setStatusType('err');
      return;
    }

    setIsRunning(true);
    abortRef.current = false;
    setProgress({ current: 0, total: targetList.length });

    let registeredCount = 0;
    const currentFailures: Array<{ name: string; reason: string }> = [];

    for (let i = 0; i < targetList.length; i++) {
      if (abortRef.current) {
        break;
      }

      const name = targetList[i];
      const accountPassword = autoRegister ? generateRandomPassword() : registerPassword;
      setProgress({ current: i + 1, total: targetList.length });
      setStatusText(`Registering ${name} (${i + 1}/${targetList.length})`);
      setStatusType('normal');

      try {
        await botStore.registerBot({
          username: name,
          password: accountPassword,
          server: registerServer,
          target_1st_job: target1stJob,
          target_2nd_job: target2ndJob,
        });
        registeredCount++;
      } catch (err: unknown) {
        const errorReason =
          err && typeof err === 'object' && 'message' in err
            ? String(err.message)
            : 'Server rejection or network timeout';
        currentFailures.push({ name, reason: errorReason });
      }
    }

    setFailures(currentFailures);
    setIsRunning(false);

    if (abortRef.current) {
      setStatusText(`Stopped. Registered ${registeredCount}, failed ${currentFailures.length}.`);
      setStatusType(currentFailures.length > 0 ? 'err' : 'normal');
    } else if (currentFailures.length === 0) {
      setStatusText(
        `Done. ${registeredCount} account${registeredCount > 1 ? 's' : ''} registered.`
      );
      setStatusType('ok');
    } else {
      setStatusText(`Done. Registered ${registeredCount}, failed ${currentFailures.length}.`);
      setStatusType('err');
    }
  };

  const handleCancelOrStop = () => {
    if (isRunning) {
      abortRef.current = true;
      setStatusText('Stopping registration...');
    } else {
      onClose();
    }
  };

  if (!isOpen) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleCancelOrStop}
      closeOnEsc={!isRunning}
      closeOnBackdrop={!isRunning}
      maxWidth="330px"
      header={
        <div className="dlg-tabs" role="tablist">
          <button
            type="button"
            className={`dlg-tab ${activeTab === 'login' ? 'on' : ''}`}
            role="tab"
            aria-selected={activeTab === 'login'}
            onClick={() => {
              if (!isRunning) setActiveTab('login');
            }}
          >
            Login
          </button>
          <button
            type="button"
            className={`dlg-tab ${activeTab === 'register' ? 'on' : ''}`}
            role="tab"
            aria-selected={activeTab === 'register'}
            onClick={() => {
              if (!isRunning) setActiveTab('register');
            }}
          >
            Register
          </button>
        </div>
      }
      footer={
        <>
          <button type="button" className="b" onClick={handleCancelOrStop}>
            {isRunning ? 'Stop' : 'Cancel'}
          </button>
          <button
            type="button"
            className="b pri"
            disabled={isRunning}
            onClick={() => {
              if (activeTab === 'login') {
                handleLoginSubmit();
              } else {
                handleRegisterSubmit();
              }
            }}
          >
            {activeTab === 'login'
              ? 'Add'
              : autoRegister
              ? `Register ${generatedNames.length}`
              : 'Register'}
          </button>
        </>
      }
    >

        {/* LOGIN TAB BODY */}
        {activeTab === 'login' && (
          <form className="dlg-body" onSubmit={handleLoginSubmit}>
            <label className="dlg-field">
              Username
              <input
                type="text"
                placeholder="Account username"
                autoComplete="off"
                value={loginUsername}
                onChange={(e) => setLoginUsername(e.target.value)}
                autoFocus
              />
            </label>

            <label className="dlg-field">
              Password
              <input
                type="password"
                placeholder="Account password"
                value={loginPassword}
                onChange={(e) => setLoginPassword(e.target.value)}
              />
            </label>

            <label className="dlg-field">
              Channel
              <select value={loginServer} onChange={(e) => setLoginServer(e.target.value)}>
                {channelList.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px' }}>
              <label className="dlg-field">
                Target 1st Job
                <select
                  value={target1stJob}
                  onChange={(e) => {
                    const j = e.target.value;
                    setTarget1stJob(j);
                    setTarget2ndJob(SECOND_JOBS_PREVIEW[j]?.[0]?.id || 'ringmaster');
                  }}
                >
                  <option value="assist">Assist</option>
                  <option value="mercenary">Mercenary</option>
                  <option value="acrobat">Acrobat</option>
                  <option value="magician">Magician</option>
                </select>
              </label>
              <label className="dlg-field">
                Target 2nd Job
                <select
                  value={target2ndJob}
                  onChange={(e) => setTarget2ndJob(e.target.value)}
                >
                  {(SECOND_JOBS_PREVIEW[target1stJob] || SECOND_JOBS_PREVIEW.assist).map((sec) => (
                    <option key={sec.id} value={sec.id}>
                      {sec.name}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            {statusText && <div className={`dlg-status ${statusType}`}>{statusText}</div>}
          </form>
        )}

        {/* REGISTER TAB BODY */}
        {activeTab === 'register' && (
          <form className="dlg-body" onSubmit={handleRegisterSubmit}>
            {/* Auto Register Toggle Box */}
            <div className="dlg-auto">
              <label className="dlg-auto-header">
                <span>
                  <b>Auto Register</b>
                  <br />
                  <span style={{ color: 'var(--m)', fontSize: '11.5px' }}>
                    Create many accounts at once
                  </span>
                </span>
                <ToggleSwitch
                  checked={autoRegister}
                  onChange={(v) => !isRunning && setAutoRegister(v)}
                />
              </label>

              {autoRegister && (
                <div className="dlg-auto-opts">
                  <label className="dlg-field">
                    Name style
                    <select
                      value={nameStyle}
                      disabled={isRunning}
                      onChange={(e) => setNameStyle(e.target.value as 'num' | 'read' | 'rand')}
                    >
                      <option value="num">Numbered (dravok001)</option>
                      <option value="read">Readable names (kaelthorn)</option>
                      <option value="rand">Random small letters (xkqpmz)</option>
                    </select>
                  </label>

                  <div className="dlg-row">
                    <label className="dlg-field">
                      Number of accounts
                      <div className="stepper">
                        <button
                          type="button"
                          disabled={isRunning}
                          onClick={() => handleCountChange(-1)}
                          aria-label="Decrease"
                        >
                          −
                        </button>
                        <input
                          type="number"
                          min="1"
                          max="500"
                          disabled={isRunning}
                          value={accountCount}
                          onChange={(e) =>
                            setAccountCount(Math.max(1, Math.min(500, parseInt(e.target.value) || 1)))
                          }
                        />
                        <button
                          type="button"
                          disabled={isRunning}
                          onClick={() => handleCountChange(1)}
                          aria-label="Increase"
                        >
                          +
                        </button>
                      </div>
                    </label>

                    {nameStyle === 'num' ? (
                      <label className="dlg-field">
                        Start at
                        <input
                          type="number"
                          min="0"
                          disabled={isRunning}
                          value={startAt}
                          onChange={(e) => setStartAt(Math.max(0, parseInt(e.target.value) || 0))}
                        />
                      </label>
                    ) : (
                      <div />
                    )}
                  </div>

                  {nameStyle === 'num' && (
                    <div className="dlg-row">
                      <label className="dlg-field">
                        Username prefix
                        <input
                          type="text"
                          disabled={isRunning}
                          value={prefix}
                          onChange={(e) => setPrefix(e.target.value)}
                        />
                      </label>
                      <label className="dlg-field">
                        Digits
                        <select
                          disabled={isRunning}
                          value={digits}
                          onChange={(e) => setDigits(parseInt(e.target.value) || 3)}
                        >
                          <option value="1">1 (1, 2…)</option>
                          <option value="2">2 (01, 02…)</option>
                          <option value="3">3 (001, 002…)</option>
                        </select>
                      </label>
                    </div>
                  )}

                  <label className="dlg-field">
                    Preview
                    <div className="dlg-preview">{previewSummary}</div>
                    {nameStyle !== 'num' && (
                      <button
                        type="button"
                        className="b"
                        disabled={isRunning}
                        onClick={handleShuffle}
                        style={{ marginTop: '4px', height: '26px' }}
                      >
                        Shuffle names
                      </button>
                    )}
                  </label>

                  <label className="dlg-field">
                    Letter case
                    <select
                      disabled={isRunning}
                      value={letterCase}
                      onChange={(e) => setLetterCase(e.target.value as 'low' | 'cap' | 'up')}
                    >
                      <option value="low">lowercase (kaelthorn)</option>
                      <option value="cap">Capitalize (Kaelthorn)</option>
                      <option value="up">ALL CAPS (KAELTHORN)</option>
                    </select>
                  </label>
                </div>
              )}
            </div>

            {/* Single mode username field */}
            {!autoRegister && (
              <label className="dlg-field">
                Username
                <input
                  type="text"
                  placeholder="Choose a username"
                  autoComplete="off"
                  disabled={isRunning}
                  value={singleUsername}
                  onChange={(e) => setSingleUsername(e.target.value)}
                  autoFocus
                />
              </label>
            )}

            {!autoRegister && (
              <label className="dlg-field">
                Password
                <input
                  type="password"
                  placeholder="Password"
                  disabled={isRunning}
                  value={registerPassword}
                  onChange={(e) => setRegisterPassword(e.target.value)}
                />
              </label>
            )}

            <label className="dlg-field">
              Channel
              <select
                disabled={isRunning}
                value={registerServer}
                onChange={(e) => setRegisterServer(e.target.value)}
              >
                {channelList.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px' }}>
              <label className="dlg-field">
                Target 1st Job
                <select
                  disabled={isRunning}
                  value={target1stJob}
                  onChange={(e) => {
                    const j = e.target.value;
                    setTarget1stJob(j);
                    setTarget2ndJob(SECOND_JOBS_PREVIEW[j]?.[0]?.id || 'ringmaster');
                  }}
                >
                  <option value="assist">Assist</option>
                  <option value="mercenary">Mercenary</option>
                  <option value="acrobat">Acrobat</option>
                  <option value="magician">Magician</option>
                </select>
              </label>
              <label className="dlg-field">
                Target 2nd Job
                <select
                  disabled={isRunning}
                  value={target2ndJob}
                  onChange={(e) => setTarget2ndJob(e.target.value)}
                >
                  {(SECOND_JOBS_PREVIEW[target1stJob] || SECOND_JOBS_PREVIEW.assist).map((sec) => (
                    <option key={sec.id} value={sec.id}>
                      {sec.name}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            {/* Progress bar */}
            {progress && (
              <ProgressBar
                value={progress.current}
                max={progress.total}
                height={6}
                className="dlg-prog"
              />
            )}

            {/* Status line */}
            <div className={`dlg-status ${statusType}`}>{statusText}</div>

            {/* Failure inspection accordion */}
            {failures.length > 0 && (
              <div style={{ fontSize: '11px' }}>
                <button
                  type="button"
                  className="b"
                  style={{ height: '22px', fontSize: '10.5px', padding: '0 6px' }}
                  onClick={() => setShowFailures(!showFailures)}
                >
                  {showFailures ? 'Hide failed details' : `View ${failures.length} failed`}
                </button>
                {showFailures && (
                  <div
                    className="dlg-preview"
                    style={{ marginTop: '5px', maxHeight: '70px', color: 'var(--r)' }}
                  >
                    {failures.map((f, idx) => (
                      <div key={idx}>
                        <b>{f.name}</b>: {f.reason}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </form>
        )}
    </Modal>
  );
};
