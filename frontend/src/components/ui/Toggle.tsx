'use client';
import React from 'react';

interface ToggleProps {
  checked: boolean;
  onChange: (v: boolean) => void;
  label?: string;
}

const Toggle: React.FC<ToggleProps> = ({ checked, onChange, label }) => (
  <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
    <div onClick={() => onChange(!checked)} style={{
      width: 36, height: 20, borderRadius: 10,
      background: checked ? 'var(--c-accent)' : 'var(--c-bg-subtle)',
      border: `1px solid ${checked ? 'var(--c-accent)' : 'var(--c-border)'}`,
      position: 'relative', transition: 'all 0.2s', flexShrink: 0,
    }}>
      <div style={{ position: 'absolute', top: 2, left: checked ? 18 : 2, width: 14, height: 14, borderRadius: '50%', background: '#fff', transition: 'left 0.2s' }}/>
    </div>
    {label && <span style={{ fontSize: 13, color: 'var(--c-fg)' }}>{label}</span>}
  </label>
);

export default Toggle;
