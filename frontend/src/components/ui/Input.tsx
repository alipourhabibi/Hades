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
  // id pairs the field with a <label htmlFor>. Without it an input has no
  // accessible name: a screen reader announces an unlabelled text box, and
  // getByLabel finds nothing, which is the same lookup a screen reader makes.
  // A placeholder is not a substitute; it disappears as soon as the field has
  // content.
  id?: string;
  // autoComplete is what a password manager reads to decide what to fill.
  // Common values here: "username", "current-password", "new-password",
  // "email", "one-time-code".
  autoComplete?: string;
  ariaLabel?: string;
  required?: boolean;
}

const Input: React.FC<InputProps> = ({ value, onChange, placeholder, style, type = 'text', prefix, suffix, onKeyDown, autoFocus, disabled, name, id, autoComplete, ariaLabel, required }) => {
  const [foc, setFoc] = useState(false);
  return (
    <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
      {prefix && (
        <span style={{ position: 'absolute', left: 10, color: 'var(--c-fg-subtle)', pointerEvents: 'none', display: 'flex', zIndex: 1 }}>
          {prefix}
        </span>
      )}
      <input
        type={type} value={value} name={name} id={id}
        autoComplete={autoComplete} aria-label={ariaLabel} required={required}
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
