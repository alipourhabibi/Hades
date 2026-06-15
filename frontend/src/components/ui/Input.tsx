'use client';
import React, { useState } from 'react';

interface InputProps {
  value: string;
  onChange?: (v: string) => void;
  placeholder?: string;
  style?: React.CSSProperties;
  type?: string;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
  autoFocus?: boolean;
  disabled?: boolean;
  name?: string;
}

const Input: React.FC<InputProps> = ({ value, onChange, placeholder, style, type = 'text', prefix, suffix, onKeyDown, autoFocus, disabled, name }) => {
  const [foc, setFoc] = useState(false);
  return (
    <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
      {prefix && (
        <span style={{ position: 'absolute', left: 10, color: 'var(--c-fg-subtle)', pointerEvents: 'none', display: 'flex', zIndex: 1 }}>
          {prefix}
        </span>
      )}
      <input
        type={type} value={value} name={name}
        onChange={e => onChange && onChange(e.target.value)}
        placeholder={placeholder} autoFocus={autoFocus}
        onKeyDown={onKeyDown} disabled={disabled}
        onFocus={() => setFoc(true)} onBlur={() => setFoc(false)}
        style={{
          width: '100%', background: 'var(--c-bg-inset)',
          border: `1px solid ${foc ? 'var(--c-accent)' : 'var(--c-border)'}`,
          borderRadius: 6, color: 'var(--c-fg)', fontSize: 13, fontFamily: 'inherit',
          padding: `8px ${suffix ? '34px' : '12px'} 8px ${prefix ? '34px' : '12px'}`,
          outline: 'none', transition: 'border-color 0.15s', ...style,
        }}
      />
      {suffix && (
        <span style={{ position: 'absolute', right: 10, color: 'var(--c-fg-subtle)', display: 'flex' }}>
          {suffix}
        </span>
      )}
    </div>
  );
};

export default Input;
