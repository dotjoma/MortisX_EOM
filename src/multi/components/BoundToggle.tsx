import React from 'react';
import { useMultiSetting } from '../store';

interface BoundToggleProps {
  setting: string;
  fallbackValue?: boolean;
  disabled?: boolean;
  onChanged?: (checked: boolean) => void;
  className?: string;
  title?: string;
}

export const BoundToggle: React.FC<BoundToggleProps> = ({
  setting,
  fallbackValue = false,
  disabled = false,
  onChanged,
  className = '',
  title,
}) => {
  const { value, mixed, isApplying, setValue } = useMultiSetting<boolean>(
    setting,
    fallbackValue
  );

  const handleClick = async () => {
    if (disabled || isApplying) return;
    const nextVal = mixed ? true : !value;
    onChanged?.(nextVal);
    await setValue(nextVal);
  };

  const tooltip =
    title ||
    (mixed
      ? 'Mixed values'
      : undefined);

  return (
    <div
      className={`sw ${value ? 'on' : ''} ${mixed ? 'mixed' : ''} ${className}`}
      onClick={handleClick}
      role="switch"
      aria-checked={mixed ? 'mixed' : value}
      title={tooltip}
      style={{
        cursor: disabled || isApplying ? 'not-allowed' : 'pointer',
        opacity: isApplying ? 0.6 : 1,
        transition: 'opacity 0.2s',
      }}
    />
  );
};
