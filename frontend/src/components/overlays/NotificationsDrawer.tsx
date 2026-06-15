'use client';
import React, { useEffect, useState } from 'react';
import { IconX, IconBell, IconGitCommit, IconBox, IconAlert } from '../icons';

interface NotificationsDrawerProps {
  onClose: () => void;
}

interface Notification {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
}

const NotificationsDrawer: React.FC<NotificationsDrawerProps> = ({ onClose }) => {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [filter, setFilter] = useState('all');
  const [open, setOpen] = useState(false);

  useEffect(() => {
    setTimeout(() => setOpen(true), 10);
    // TODO: load from NotificationService when gen clients are available
    setNotifications([]);
  }, []);

  const handleClose = () => {
    setOpen(false);
    setTimeout(onClose, 200);
  };

  const filters = ['all', 'breaking', 'commits', 'sdks'];
  const typeIcon = (type: string) => {
    if (type.includes('breaking')) return <IconAlert size={14}/>;
    if (type.includes('commit')) return <IconGitCommit size={14}/>;
    if (type.includes('sdk')) return <IconBox size={14}/>;
    return <IconBell size={14}/>;
  };

  return (
    <>
      {/* Backdrop */}
      <div onClick={handleClose} style={{ position: 'fixed', inset: 0, zIndex: 100 }}/>
      {/* Drawer */}
      <div style={{
        position: 'fixed', top: 0, right: 0, bottom: 0, width: 380,
        background: 'var(--c-bg-default)', borderLeft: '1px solid var(--c-border)',
        boxShadow: '-8px 0 40px rgba(0,0,0,0.3)', zIndex: 101,
        display: 'flex', flexDirection: 'column',
        transform: open ? 'translateX(0)' : 'translateX(100%)',
        transition: 'transform 0.2s ease',
      }}>
        {/* Header */}
        <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--c-border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <IconBell size={16}/>
            <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)' }}>Notifications</span>
          </div>
          <button onClick={handleClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex' }}>
            <IconX size={16}/>
          </button>
        </div>

        {/* Filter tabs */}
        <div style={{ display: 'flex', borderBottom: '1px solid var(--c-border)', padding: '0 12px' }}>
          {filters.map(f => (
            <button key={f} onClick={() => setFilter(f)} style={{
              padding: '8px 12px', fontSize: 12, fontWeight: 500, textTransform: 'capitalize',
              background: 'none', border: 'none', cursor: 'pointer', fontFamily: 'inherit',
              color: filter === f ? 'var(--c-fg)' : 'var(--c-fg-muted)',
              borderBottom: `2px solid ${filter === f ? 'var(--c-accent)' : 'transparent'}`,
              marginBottom: -1,
            }}>{f === 'all' ? 'All' : f}</button>
          ))}
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {notifications.length === 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', gap: 12, color: 'var(--c-fg-muted)' }}>
              <IconBell size={28} style={{ opacity: 0.4 }}/>
              <div style={{ fontSize: 14, fontWeight: 500 }}>All caught up</div>
              <div style={{ fontSize: 12, color: 'var(--c-fg-subtle)' }}>No new notifications</div>
            </div>
          ) : notifications.map(n => (
            <div key={n.id} style={{
              padding: '12px 16px', display: 'flex', gap: 12, alignItems: 'flex-start',
              borderBottom: '1px solid var(--c-border-muted)', cursor: 'pointer',
              background: n.read ? 'transparent' : 'var(--c-accent-bg)',
            }}>
              <div style={{ width: 30, height: 30, borderRadius: '50%', background: 'var(--c-bg-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, color: 'var(--c-fg-muted)' }}>
                {typeIcon(n.type)}
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 13, fontWeight: n.read ? 400 : 600, color: 'var(--c-fg)', marginBottom: 2 }}>{n.title}</div>
                <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{n.body}</div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
};

export default NotificationsDrawer;
