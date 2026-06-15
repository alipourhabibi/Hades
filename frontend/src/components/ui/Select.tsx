import React from 'react';

interface SelectOption { value: string; label: string; }

interface SelectProps {
  value: string;
  onChange: (v: string) => void;
  options: SelectOption[];
  style?: React.CSSProperties;
}

const Select: React.FC<SelectProps> = ({ value, onChange, options, style }) => (
  <select value={value} onChange={e => onChange(e.target.value)} style={{
    background: 'var(--c-bg-inset)', border: '1px solid var(--c-border)', borderRadius: 6,
    color: 'var(--c-fg)', fontSize: 13, fontFamily: 'inherit', padding: '6px 10px',
    outline: 'none', cursor: 'pointer', ...style,
  }}>
    {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
  </select>
);

export default Select;
