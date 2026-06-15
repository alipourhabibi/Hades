import React from 'react';

interface SectionProps {
  title?: string;
  children: React.ReactNode;
  action?: React.ReactNode;
  style?: React.CSSProperties;
}

const Section: React.FC<SectionProps> = ({ title, children, action, style }) => (
  <div style={{ padding: '24px 32px', ...style }}>
    {title && (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--c-fg)' }}>{title}</h2>
        {action}
      </div>
    )}
    {children}
  </div>
);

export default Section;
