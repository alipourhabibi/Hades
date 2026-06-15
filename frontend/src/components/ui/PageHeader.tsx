import React from 'react';
import { IconChevronRight } from '../icons';

interface Breadcrumb {
  label: string;
  onClick?: () => void;
}

interface PageHeaderProps {
  title: React.ReactNode;
  subtitle?: string;
  actions?: React.ReactNode;
  breadcrumb?: Breadcrumb[];
}

const PageHeader: React.FC<PageHeaderProps> = ({ title, subtitle, actions, breadcrumb }) => (
  <div style={{ padding: '24px 32px 20px', borderBottom: '1px solid var(--c-border)', flexShrink: 0 }}>
    {breadcrumb && (
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, fontSize: 12, color: 'var(--c-fg-muted)', flexWrap: 'wrap' }}>
        {breadcrumb.map((b, i) => (
          <React.Fragment key={i}>
            {i > 0 && <IconChevronRight size={12}/>}
            <span style={{ color: b.onClick ? 'var(--c-accent)' : 'var(--c-fg-muted)', cursor: b.onClick ? 'pointer' : 'default' }} onClick={b.onClick}>{b.label}</span>
          </React.Fragment>
        ))}
      </div>
    )}
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
      <div>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: 'var(--c-fg)', letterSpacing: -0.3 }}>{title}</h1>
        {subtitle && <p style={{ margin: '4px 0 0', fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5 }}>{subtitle}</p>}
      </div>
      {actions && <div style={{ display: 'flex', gap: 8, flexShrink: 0, flexWrap: 'wrap' }}>{actions}</div>}
    </div>
  </div>
);

export default PageHeader;
