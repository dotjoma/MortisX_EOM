import React from 'react';
import { useMultiPendingConfirm } from '../store';

export const ConfirmModal: React.FC = () => {
  const { pendingConfirm, confirm, cancel } = useMultiPendingConfirm();

  if (!pendingConfirm) return null;

  const { request } = pendingConfirm;
  const botCount = request.bots.length;

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        background: 'rgba(0, 0, 0, 0.75)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999,
        padding: '20px',
      }}
    >
      <div
        style={{
          background: 'var(--bg2, #141b26)',
          border: '1px solid var(--danger, #ff6b6b)',
          borderRadius: '8px',
          width: '420px',
          maxWidth: '100%',
          boxShadow: '0 8px 24px rgba(0, 0, 0, 0.5)',
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <div
          style={{
            padding: '12px 16px',
            background: 'rgba(255, 107, 107, 0.15)',
            borderBottom: '1px solid rgba(255, 107, 107, 0.3)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <span style={{ fontWeight: 700, color: 'var(--danger, #ff6b6b)', fontSize: '13px' }}>
            ⚠️ High Risk Multi-Bot Action
          </span>
        </div>

        <div style={{ padding: '16px', fontSize: '12px', lineHeight: 1.5, color: '#e2e8f0' }}>
          <p style={{ margin: '0 0 10px' }}>
            You are applying a high-risk action (<strong>{request.entry}</strong>) to{' '}
            <strong>{botCount} bots</strong> simultaneously.
          </p>

          <div
            style={{
              maxHeight: '130px',
              overflowY: 'auto',
              background: 'rgba(0,0,0,0.3)',
              borderRadius: '4px',
              padding: '8px 12px',
              margin: '0 0 12px',
              border: '1px solid var(--ln, #1e293b)',
            }}
          >
            <div style={{ fontSize: '10.5px', color: 'var(--m, #94a3b8)', marginBottom: '4px' }}>
              Affected Bots:
            </div>
            {request.bots.map((b) => (
              <div key={b.id} style={{ fontSize: '11.5px', padding: '2px 0' }}>
                • <strong>{b.name}</strong> <span style={{ opacity: 0.6 }}>({b.id})</span>
              </div>
            ))}
          </div>

          <p style={{ margin: 0, fontSize: '11px', color: 'var(--m, #94a3b8)' }}>
            Are you sure you want to proceed?
          </p>
        </div>

        <div
          style={{
            padding: '12px 16px',
            background: 'rgba(0,0,0,0.2)',
            borderTop: '1px solid var(--ln, #1e293b)',
            display: 'flex',
            justifyContent: 'flex-end',
            gap: '8px',
          }}
        >
          <button
            type="button"
            className="b"
            style={{ padding: '6px 14px', fontSize: '12px' }}
            onClick={cancel}
          >
            Cancel
          </button>
          <button
            type="button"
            className="b danger"
            style={{
              padding: '6px 14px',
              fontSize: '12px',
              background: 'var(--danger, #ff6b6b)',
              color: '#fff',
              border: 'none',
              borderRadius: '4px',
              fontWeight: 600,
              cursor: 'pointer',
            }}
            onClick={confirm}
          >
            Confirm & Execute
          </button>
        </div>
      </div>
    </div>
  );
};
