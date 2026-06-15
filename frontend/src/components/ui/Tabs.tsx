'use client';
import React from 'react';

interface TabItem {
  id: string;
  label: string;
  icon?: React.ReactNode;
  count?: number;
}

interface TabsProps {
  tabs: TabItem[];
  active: string;
  onChange: (id: string) => void;
}

const Tabs: React.FC<TabsProps> = ({ tabs, active, onChange }) => (
  <div style={{ display: 'flex', borderBottom: '1px solid var(--c-border)', gap: 0, overflowX: 'auto' }}>
    {tabs.map(t => (
      <button key={t.id} onClick={() => onChange(t.id)} style={{
        padding: '10px 16px', fontSize: 13, fontWeight: 500, flexShrink: 0,
        background: 'transparent', border: 'none', cursor: 'pointer',
        color: active === t.id ? 'var(--c-fg)' : 'var(--c-fg-muted)',
        borderBottom: `2px solid ${active === t.id ? 'var(--c-accent)' : 'transparent'}`,
        marginBottom: -1, fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 6,
        transition: 'color 0.1s', whiteSpace: 'nowrap',
      }}>
        {t.icon}{t.label}
        {t.count != null && (
          <span style={{ background: 'var(--c-bg-subtle)', color: 'var(--c-fg-muted)', borderRadius: 10, padding: '1px 6px', fontSize: 11 }}>{t.count}</span>
        )}
      </button>
    ))}
  </div>
);

export default Tabs;
