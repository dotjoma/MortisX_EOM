import React from 'react';

export type BadgeVariant = 'default' | 'pk' | 'safe' | 'market' | 'current';

interface BadgeProps {
  variant?: BadgeVariant;
  children: React.ReactNode;
  className?: string;
  title?: string;
}

export const Badge: React.FC<BadgeProps> = ({
  variant = 'default',
  children,
  className = '',
  title,
}) => {
  return (
    <span className={`badge badge-${variant} ${className}`} title={title}>
      {children}
    </span>
  );
};
