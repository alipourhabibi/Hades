'use client';
import React, { useEffect, useState } from 'react';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import Card from '@/components/ui/Card';
import Btn from '@/components/ui/Button';
import Badge from '@/components/ui/Badge';
import { IconGlobe, IconClock, IconX, IconAlert, IconShield } from '@/components/icons';
import { formatError } from '@/lib/connectError';
import { rpcFetch } from '@/lib/rpc';

interface Session { id: string; ipAddress?: string; userAgent?: string; lastActivityAt?: string; createdAt?: string; isCurrent?: boolean; }

function deviceLabel(ua?: string): string {
  if (!ua) return 'Unknown device';
  if (ua.includes('VSCode') || ua.includes('vscode')) return 'VS Code Extension';
  if (ua.includes('hades-cli') || ua.toLowerCase().includes('cli')) return 'CLI (hades-cli)';
  if (ua.includes('Chrome') && ua.includes('Linux')) return 'Chrome on Linux';
  if (ua.includes('Chrome') && ua.includes('Mac')) return 'Chrome on macOS';
  if (ua.includes('Chrome') && ua.includes('Windows')) return 'Chrome on Windows';
  if (ua.includes('Chrome')) return 'Chrome';
  if (ua.includes('Firefox')) return 'Firefox';
  if (ua.includes('Safari') && ua.includes('iPhone')) return 'Safari on iPhone';
  if (ua.includes('Safari')) return 'Safari';
  return ua.slice(0, 40);
}

function deviceEmoji(ua?: string): string {
  if (!ua) return '🖥️';
  if (ua.includes('iPhone') || ua.includes('iOS')) return '📱';
  if (ua.includes('Linux') && !ua.includes('Android')) return '🐧';
  if (ua.includes('Mac')) return '💻';
  if (ua.includes('VSCode') || ua.includes('vscode')) return '⌨️';
  if (ua.toLowerCase().includes('cli')) return '⬛';
  return '🖥️';
}

function deviceOS(ua?: string): string {
  if (!ua) return 'Unknown';
  if (ua.includes('iPhone') || ua.includes('iOS')) return 'iOS';
  if (ua.includes('Linux') && !ua.includes('Android')) return 'Linux';
  if (ua.includes('Mac')) return 'macOS';
  if (ua.includes('Windows')) return 'Windows';
  return '';
}

function fmtActivity(ts?: string): string {
  if (!ts) return 'Unknown';
  try {
    const d = new Date(ts); const now = new Date(); const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    if (diffMin < 2) return 'Active now';
    if (diffMin < 60) return `${diffMin} minutes ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr} hour${diffHr !== 1 ? 's' : ''} ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 7) return `${diffDay} day${diffDay !== 1 ? 's' : ''} ago`;
    return d.toLocaleDateString();
  } catch { return ts; }
}

export default function PageSessions() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revoking, setRevoking] = useState<string | null>(null);

  const loadSessions = async () => {
    try {
      const data = await rpcFetch<{ sessions: Session[] }>('/hades.api.authentication.v1.SessionService/ListSessions', {});
      setSessions(data.sessions || []);
    } catch (e) { setError(formatError(e)); } finally { setLoading(false); }
  };

  useEffect(() => { loadSessions(); }, []);

  const revoke = async (id: string) => {
    setRevoking(id);
    try { await rpcFetch('/hades.api.authentication.v1.SessionService/RevokeSession', { sessionId: id }); setSessions(s => s.filter(x => x.id !== id)); }
    catch (e) { setError(formatError(e)); } finally { setRevoking(null); }
  };

  const revokeAll = async () => {
    try { await rpcFetch('/hades.api.authentication.v1.SessionService/RevokeAllOtherSessions', {}); await loadSessions(); }
    catch (e) { setError(formatError(e)); }
  };

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader title="Active Sessions" subtitle="Manage all devices and sessions currently signed in to your account." actions={<Btn variant="danger" onClick={revokeAll} icon={<IconX size={13}/>}>Revoke all other sessions</Btn>}/>
      <Section>
        {error && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8 }}><IconAlert size={14}/>{error}</div>}

        {loading ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--c-fg-muted)', fontSize: 13 }}>Loading sessions…</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {sessions.map(s => (
              <Card key={s.id} style={{ padding: '16px 20px', display: 'flex', gap: 16, alignItems: 'center' }}>
                <span style={{ fontSize: 20, lineHeight: 1 }}>{deviceEmoji(s.userAgent)}</span>
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: 'var(--c-fg)' }}>{deviceLabel(s.userAgent)}</span>
                    {s.isCurrent && <Badge variant="green">Current session</Badge>}
                  </div>
                  <div style={{ display: 'flex', gap: 16, fontSize: 12, color: 'var(--c-fg-subtle)', flexWrap: 'wrap' }}>
                    {deviceOS(s.userAgent) && <span>{deviceOS(s.userAgent)}</span>}
                    {s.ipAddress && <span style={{ display: 'flex', gap: 4, alignItems: 'center' }}><IconGlobe size={11}/><span style={{ fontFamily: "'IBM Plex Mono', monospace" }}>{s.ipAddress}</span></span>}
                    <span style={{ display: 'flex', gap: 4, alignItems: 'center' }}><IconClock size={11}/>{fmtActivity(s.lastActivityAt)}</span>
                  </div>
                </div>
                {!s.isCurrent && <Btn size="sm" variant="danger" onClick={() => revoke(s.id)} disabled={revoking === s.id}>{revoking === s.id ? '…' : 'Revoke'}</Btn>}
              </Card>
            ))}
            {sessions.length === 0 && <div style={{ textAlign: 'center', padding: 40, color: 'var(--c-fg-muted)', fontSize: 13 }}>No active sessions found.</div>}
          </div>
        )}

        <div style={{ marginTop: 24, padding: '16px 20px', borderRadius: 8, background: 'var(--c-bg-overlay)', border: '1px solid var(--c-border)' }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 6, display: 'flex', gap: 8, alignItems: 'center' }}><IconShield size={13}/>Security tip</div>
          <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.6 }}>If you see a session you don&apos;t recognize, revoke it immediately and change your password. Sessions automatically expire after 30 days of inactivity.</div>
        </div>
      </Section>
    </div>
  );
}
