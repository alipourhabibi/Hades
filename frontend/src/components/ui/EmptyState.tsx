import React from 'react';

interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  subtitle?: string;
  action?: React.ReactNode;
}

const EmptyState: React.FC<EmptyStateProps> = ({ icon, title, subtitle, action }) => (
  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '64px 32px', color: 'var(--c-fg-muted)', gap: 12 }}>
    <div style={{ color: 'var(--c-fg-subtle)', opacity: 0.6 }}>{icon}</div>
    <div style={{ fontSize: 15, fontWeight: 500, color: 'var(--c-fg-muted)' }}>{title}</div>
    {subtitle && <div style={{ fontSize: 13, color: 'var(--c-fg-subtle)', textAlign: 'center', maxWidth: 320, lineHeight: 1.6 }}>{subtitle}</div>}
    {action && <div style={{ marginTop: 8 }}>{action}</div>}
  </div>
);

export default EmptyState;
