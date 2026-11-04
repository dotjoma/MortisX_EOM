import React, { useSyncExternalStore } from 'react';
import { multiStore } from '../store';

export const ResultToast: React.FC = () => {
  const s = useSyncExternalStore(multiStore.subscribe, multiStore.getState);
  const toast = s.toast;
  if (!toast) return null;

  const { summary, title } = toast;
  const isErr = summary.failed > 0;

  return (
    <div
      style={{
        position: 'fixed',
        bottom: 20,
        right: 20,
        zIndex: 9999,
        background: isErr ? '#2a1215' : '#142218',
        border: `1px solid ${isErr ? '#e74c3c' : '#2ecc71'}`,
        color: '#fff',
        borderRadius: '6px',
        padding: '8px 14px',
        fontSize: '12px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
      }}
    >
      <div>
        {title && <div style={{ fontWeight: 700, marginBottom: '2px' }}>{title}</div>}
        <div>
          Total: {summary.total} | OK: {summary.ok} | Skipped: {summary.skipped} | Failed: {summary.failed}
        </div>
      </div>
      <button
        type="button"
        onClick={() => multiStore.dismissToast()}
        style={{
          background: 'none',
          border: 'none',
          color: '#aaa',
          cursor: 'pointer',
          padding: '0 4px',
          fontSize: '14px',
        }}
      >
        ✕
      </button>
    </div>
  );
};
