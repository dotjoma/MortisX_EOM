import React, { useEffect, useState } from 'react';
import { Modal } from '../shared/Modal';

interface AutoReconnectModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentDelay: number;
  targetCount?: number;
  onSave: (delay: number) => Promise<any> | void;
}

export const AutoReconnectModal: React.FC<AutoReconnectModalProps> = ({
  isOpen,
  onClose,
  currentDelay,
  targetCount = 1,
  onSave,
}) => {
  const [delayInput, setDelayInput] = useState<string>(String(currentDelay || 14));
  const [error, setError] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setDelayInput(String(currentDelay || 14));
      setError(null);
      setIsSaving(false);
    }
  }, [isOpen, currentDelay]);

  const validateValue = (val: string): number | null => {
    const trimmed = val.trim();
    if (!trimmed) {
      setError('Please enter a delay in seconds.');
      return null;
    }
    const num = Number(trimmed);
    if (!Number.isInteger(num)) {
      setError('Delay must be a whole number of seconds.');
      return null;
    }
    if (num < 1) {
      setError('Delay must be at least 1 second.');
      return null;
    }
    if (num > 3600) {
      setError('Delay cannot exceed 3600 seconds (1 hour).');
      return null;
    }
    setError(null);
    return num;
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setDelayInput(val);
    validateValue(val);
  };

  const handleSave = async () => {
    const validated = validateValue(delayInput);
    if (validated === null) return;

    try {
      setIsSaving(true);
      await onSave(validated);
      onClose();
    } catch (err: any) {
      setError(err?.message || 'Failed to save configuration');
    } finally {
      setIsSaving(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !error && !isSaving) {
      handleSave();
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Auto Reconnect Configuration"
      icon="settings"
      maxWidth="380px"
      footer={
        <div style={{ display: 'flex', width: '100%', justifyContent: 'flex-end', gap: '8px' }}>
          <button type="button" className="b" onClick={onClose} disabled={isSaving}>
            Cancel
          </button>
          <button
            type="button"
            className="b pri"
            onClick={handleSave}
            disabled={isSaving || error !== null || !delayInput.trim()}
          >
            {isSaving ? 'Saving...' : targetCount > 1 ? `Save (${targetCount} bots)` : 'Save'}
          </button>
        </div>
      }
    >
      <div style={{ padding: '12px 14px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
        <div>
          <label
            htmlFor="reconnect-delay-input"
            style={{
              display: 'block',
              fontSize: '11px',
              fontWeight: 600,
              color: 'var(--t, #eee)',
              marginBottom: '4px',
            }}
          >
            Maximum Auto Reconnect Delay
          </label>
          <div style={{ fontSize: '10.5px', color: 'var(--m, #aaa)', marginBottom: '8px' }}>
            Time in seconds to wait before checking bot connection state.
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <input
              id="reconnect-delay-input"
              type="number"
              className="inp"
              min={1}
              max={3600}
              step={1}
              value={delayInput}
              onChange={handleInputChange}
              onKeyDown={handleKeyDown}
              autoFocus
              style={{
                fontFamily: 'var(--mono)',
                fontSize: '12px',
                width: '100px',
              }}
            />
            <span style={{ fontSize: '11px', color: 'var(--m, #aaa)' }}>seconds</span>
          </div>
        </div>

        {error && (
          <div
            role="alert"
            style={{
              fontSize: '11px',
              color: 'var(--danger, #ff4d4f)',
              padding: '4px 6px',
              background: 'rgba(255, 77, 79, 0.1)',
              borderRadius: '4px',
              border: '1px solid rgba(255, 77, 79, 0.3)',
            }}
          >
            {error}
          </div>
        )}
      </div>
    </Modal>
  );
};
