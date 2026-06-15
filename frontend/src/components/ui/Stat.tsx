import React from 'react';

interface StatProps {
  label: string;
  value: React.ReactNode;
  sub?: string;
  icon?: React.ReactNode;
  accent?: string;
}

const Stat: React.FC<StatProps> = ({ label, value, sub, icon, accent }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
    <div style={{ fontSize: 11, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5, display: 'flex', alignItems: 'center', gap: 5 }}>
      {icon}{label}
    </div>
    <div style={{ fontSize: 24, fontWeight: 700, color: accent || 'var(--c-fg)', letterSpacing: -0.5 }}>{value}</div>
    {sub && <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)' }}>{sub}</div>}
  </div>
);

export default Stat;
