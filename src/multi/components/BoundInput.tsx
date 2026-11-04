import React, { useEffect, useState, useRef } from 'react';
import { useMultiSetting } from '../store';

interface BoundInputProps {
  setting: string;
  type?: 'number' | 'text';
  min?: number;
  max?: number;
  step?: number | string;
  fallbackValue?: number | string;
  debounceMs?: number;
  disabled?: boolean;
  onChanged?: (val: any) => void;
  className?: string;
  style?: React.CSSProperties;
  placeholder?: string;
}

export const BoundInput: React.FC<BoundInputProps> = ({
  setting,
  type = 'text',
  min,
  max,
  step,
  fallbackValue = type === 'number' ? 0 : '',
  debounceMs = 400,
  disabled = false,
  onChanged,
  className = 'inp',
  style,
  placeholder,
}) => {
  const { value, mixed, isApplying, count, setValue } = useMultiSetting<any>(
    setting,
    fallbackValue
  );

  const [localVal, setLocalVal] = useState<string>(
    value !== undefined && value !== null ? String(value) : ''
  );
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!mixed && value !== undefined && value !== null) {
      setLocalVal(String(value));
    }
  }, [value, mixed]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value;
    setLocalVal(raw);

    if (timerRef.current) {
      clearTimeout(timerRef.current);
    }

    timerRef.current = setTimeout(async () => {
      let finalVal: any = raw;
      if (type === 'number') {
        const num = parseFloat(raw);
        if (isNaN(num)) return;
        finalVal = num;
        if (min !== undefined && finalVal < min) finalVal = min;
        if (max !== undefined && finalVal > max) finalVal = max;
      }
      onChanged?.(finalVal);
      await setValue(finalVal);
    }, debounceMs);
  };

  const title = mixed
    ? `Mixed values across ${count} bots`
    : count > 1
    ? `Applying to ${count} bots`
    : undefined;

  return (
    <input
      type={type}
      min={min}
      max={max}
      step={step}
      value={mixed && localVal === '' ? '' : localVal}
      placeholder={mixed ? `Mixed (${count} bots)` : placeholder}
      onChange={handleChange}
      disabled={disabled || isApplying}
      className={className}
      title={title}
      style={{
        ...style,
        borderColor: mixed ? 'var(--warn, #f59e0b)' : undefined,
        opacity: isApplying ? 0.6 : 1,
      }}
    />
  );
};
