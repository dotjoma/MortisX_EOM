import React from 'react';

interface ToggleSwitchProps {
  checked: boolean;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
}

export const ToggleSwitch: React.FC<ToggleSwitchProps> = ({ checked, onChange, disabled }) => {
  return (
    <div
      className={`sw ${checked ? 'on' : ''}`}
      onClick={() => {
        if (!disabled && onChange) {
          onChange(!checked);
        }
      }}
      role="switch"
      aria-checked={checked}
      style={{ cursor: disabled ? 'not-allowed' : 'pointer' }}
    />
  );
};
