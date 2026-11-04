import React from 'react';

interface ProgressBarProps {
  value: number;
  max?: number;
  color?: string;
  height?: number;
  className?: string;
}

export const ProgressBar: React.FC<ProgressBarProps> = ({
  value,
  max = 100,
  color,
  height = 4,
  className = '',
}) => {
  const percent = Math.min(100, Math.max(0, max > 0 ? (value / max) * 100 : 0));

  return (
    <div
      className={`ui-progress ${className}`}
      style={{ height: `${height}px` }}
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
    >
      <div
        className="ui-progress-fill"
        style={{
          width: `${percent}%`,
          ...(color ? { background: color } : {}),
        }}
      />
    </div>
  );
};
