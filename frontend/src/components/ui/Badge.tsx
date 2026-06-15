import React from 'react';

type BadgeVariant = 'default' | 'blue' | 'green' | 'red' | 'yellow' | 'purple' | 'orange';

interface BadgeProps {
  variant?: BadgeVariant;
  children: React.ReactNode;
  style?: React.CSSProperties;
}

const VARIANTS: Record<BadgeVariant, { bg: string; color: string; border: string }> = {
  default: { bg: 'var(--c-bg-subtle)',   color: 'var(--c-fg-muted)',  border: 'var(--c-border)' },
  blue:    { bg: 'var(--c-accent-bg)',   color: 'var(--c-accent)',    border: 'var(--c-accent-muted)' },
  green:   { bg: 'var(--c-success-bg)',  color: 'var(--c-success)',   border: 'transparent' },
  red:     { bg: 'var(--c-danger-bg)',   color: 'var(--c-danger)',    border: 'transparent' },
  yellow:  { bg: 'var(--c-warning-bg)',  color: 'var(--c-warning)',   border: 'transparent' },
  purple:  { bg: 'var(--c-purple-bg)',   color: 'var(--c-purple)',    border: 'transparent' },
  orange:  { bg: 'var(--c-orange-bg)',   color: 'var(--c-orange)',    border: 'transparent' },
};

const Badge: React.FC<BadgeProps> = ({ variant = 'default', children, style }) => {
  const v = VARIANTS[variant] ?? VARIANTS.default;
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 4,
      padding: '2px 7px', borderRadius: 20, fontSize: 11, fontWeight: 500,
      border: `1px solid ${v.border}`, whiteSpace: 'nowrap',
      background: v.bg, color: v.color, lineHeight: 1.6, ...style,
    }}>
      {children}
    </span>
  );
};

export default Badge;
