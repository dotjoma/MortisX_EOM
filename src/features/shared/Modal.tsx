import React, { useEffect, useRef } from 'react';
import { Icon, IconName } from '../../components/Icon';

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  icon?: IconName;
  header?: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: number | string;
  closeOnEsc?: boolean;
  closeOnBackdrop?: boolean;
  className?: string;
  children: React.ReactNode;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  icon,
  header,
  footer,
  maxWidth = '330px',
  closeOnEsc = true,
  closeOnBackdrop = true,
  className = '',
  children,
}) => {
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen || !closeOnEsc) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, closeOnEsc, onClose]);

  if (!isOpen) return null;

  const handleBackdropClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (closeOnBackdrop && e.target === e.currentTarget) {
      onClose();
    }
  };

  return (
    <div
      className="modal-overlay"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
    >
      <div
        ref={dialogRef}
        className={`dlg ${className}`}
        style={{
          maxWidth: typeof maxWidth === 'number' ? `${maxWidth}px` : maxWidth,
        }}
      >
        {/* Header: custom header (e.g. tabs) or default titlebar */}
        {header ? (
          header
        ) : title ? (
          <div className="modal-header">
            <div className="modal-header-title">
              {icon && <Icon name={icon} size={16} />}
              <span>{title}</span>
            </div>
            <button
              type="button"
              className="modal-close-btn"
              onClick={onClose}
              title="Close (Esc)"
              aria-label="Close"
            >
              <Icon name="close" size={16} label="Close" />
            </button>
          </div>
        ) : null}

        {/* Content Body */}
        {children}

        {/* Footer */}
        {footer && <div className="dlg-foot">{footer}</div>}
      </div>
    </div>
  );
};
