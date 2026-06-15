'use client';
import React, { useState } from 'react';

type BtnVariant = 'default' | 'primary' | 'danger' | 'ghost' | 'success';
type BtnSize = 'sm' | 'md' | 'lg';

interface BtnProps {
  variant?: BtnVariant;
  size?: BtnSize;
  children?: React.ReactNode;
  onClick?: (e: React.MouseEvent) => void;
  style?: React.CSSProperties;
  disabled?: boolean;
  icon?: React.ReactNode;
  type?: 'button' | 'submit' | 'reset';
}

const VAR_MAP: Record<BtnVariant, { bg: string; bgH: string; border: string; color: string }> = {
  default: { bg: 'var(--btn-bg)',         bgH: 'var(--btn-bg-hover)',    border: 'var(--c-border)',      color: 'var(--c-fg)' },
  primary: { bg: '#1f6feb',               bgH: '#388bfd',                border: 'transparent',          color: '#fff' },
  danger:  { bg: 'transparent',           bgH: 'var(--c-danger-bg)',     border: 'var(--c-danger)',      color: 'var(--c-danger)' },
  ghost:   { bg: 'transparent',           bgH: 'var(--c-bg-overlay)',    border: 'transparent',          color: 'var(--c-fg-muted)' },
  success: { bg: 'var(--c-success-bg)',   bgH: 'var(--c-success-bg)',    border: 'var(--c-success)',     color: 'var(--c-success)' },
};

const SZ_MAP: Record<BtnSize, { padding: string; fontSize: number }> = {
  sm: { padding: '4px 10px',  fontSize: 12 },
  md: { padding: '6px 14px',  fontSize: 13 },
  lg: { padding: '10px 20px', fontSize: 14 },
};

const Btn: React.FC<BtnProps> = ({ variant = 'default', size = 'md', children, onClick, style, disabled, icon, type = 'button' }) => {
  const [hov, setHov] = useState(false);
  const v = VAR_MAP[variant] ?? VAR_MAP.default;
  const sz = SZ_MAP[size];
  return (
    <button type={type} onClick={onClick} disabled={disabled}
      onMouseEnter={() => setHov(true)} onMouseLeave={() => setHov(false)}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 6,
        borderRadius: 6, border: `1px solid ${v.border}`, cursor: disabled ? 'not-allowed' : 'pointer',
        background: hov && !disabled ? v.bgH : v.bg, color: v.color,
        fontFamily: 'inherit', fontWeight: 500, transition: 'all 0.1s',
        opacity: disabled ? 0.5 : 1, lineHeight: 1, ...sz, ...style,
      }}>
      {icon}{children}
    </button>
  );
};

export default Btn;
