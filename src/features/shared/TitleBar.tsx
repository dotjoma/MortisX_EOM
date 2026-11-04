import React from 'react';
import { Icon } from '../../components/Icon';
import { APP_NAME, APP_VERSION_STRING } from '../../config/version';

export const TitleBar: React.FC = () => {
  const getRuntime = () => (typeof window !== 'undefined' ? (window as any)?.runtime : null);

  const handleMinimize = (e: React.MouseEvent) => {
    e.stopPropagation();
    getRuntime()?.WindowMinimise?.();
  };

  const handleMaximize = (e: React.MouseEvent) => {
    e.stopPropagation();
    getRuntime()?.WindowToggleMaximise?.();
  };

  const handleClose = (e: React.MouseEvent) => {
    e.stopPropagation();
    getRuntime()?.Quit?.();
  };

  const handleDoubleClick = (e: React.MouseEvent) => {
    if (!(e.target as HTMLElement).closest('.wc')) {
      getRuntime()?.WindowToggleMaximise?.();
    }
  };

  return (
    <div
      className="tb"
      onDoubleClick={handleDoubleClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        userSelect: 'none',
        height: '32px',
        background: '#161922',
        borderBottom: '1px solid #232734',
        padding: '0 8px',
        // @ts-expect-error wails draggable CSS property
        '--wails-draggable': 'drag',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', pointerEvents: 'none' }}>
        <img
          src="/icon.png"
          alt="Icon"
          style={{ width: '15px', height: '15px', borderRadius: '3px', objectFit: 'contain' }}
        />
        <span style={{ fontWeight: 600, fontSize: '12px', color: '#e2e8f0', display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span>{APP_NAME}</span>
          <span
            style={{
              fontSize: '10px',
              fontWeight: 500,
              color: '#ffd959',
              background: 'rgba(255, 217, 89, 0.12)',
              border: '1px solid rgba(255, 217, 89, 0.25)',
              padding: '1px 5px',
              borderRadius: '3px',
              letterSpacing: '0.3px',
            }}
          >
            {APP_VERSION_STRING}
          </span>
        </span>
      </div>

      <div className="wc" style={{ display: 'flex', gap: '4px', pointerEvents: 'auto', alignItems: 'center' }}>
        <button
          className="wcb btn-icon"
          onClick={handleMinimize}
          title="Minimize"
          style={{
            background: 'transparent',
            border: 'none',
            color: '#8892b0',
            cursor: 'pointer',
            padding: '4px 6px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Icon name="minimize" size={14} label="Minimize" />
        </button>
        <button
          className="wcb btn-icon"
          onClick={handleMaximize}
          title="Maximize"
          style={{
            background: 'transparent',
            border: 'none',
            color: '#8892b0',
            cursor: 'pointer',
            padding: '4px 6px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Icon name="crop_square" size={14} label="Maximize" />
        </button>
        <button
          className="wcb close btn-icon"
          onClick={handleClose}
          title="Close"
          style={{
            background: 'transparent',
            border: 'none',
            color: '#8892b0',
            cursor: 'pointer',
            padding: '4px 6px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Icon name="close" size={14} label="Close" />
        </button>
      </div>
    </div>
  );
};
