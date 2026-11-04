import React from 'react';
import { useMultiSetting } from '../store';

interface BoundSelectProps {
  setting: string;
  options: Array<{ value: string; label: string }>;
  fallbackValue?: string;
  disabled?: boolean;
  onChanged?: (val: string) => void;
  className?: string;
  style?: React.CSSProperties;
}

export const BoundSelect: React.FC<BoundSelectProps> = ({
  setting,
  options,
  fallbackValue = '',
  disabled = false,
  onChanged,
  className = 'inp',
  style,
}) => {
  const { value, mixed, isApplying, count, setValue } = useMultiSetting<string>(
    setting,
    fallbackValue
  );

  const handleChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
    const nextVal = e.target.value;
    onChanged?.(nextVal);
    await setValue(nextVal);
  };

  const title = mixed
    ? `Mixed values across ${count} bots`
    : count > 1
    ? `Applying to ${count} bots`
    : undefined;

  return (
    <select
      className={className}
      value={mixed ? '__mixed__' : value || ''}
      onChange={handleChange}
      disabled={disabled || isApplying}
      title={title}
      style={{
        ...style,
        borderColor: mixed ? 'var(--warn, #f59e0b)' : undefined,
        opacity: isApplying ? 0.6 : 1,
      }}
    >
      {mixed && (
        <option value="__mixed__" disabled>
          (Mixed across {count} bots)
        </option>
      )}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  );
};
