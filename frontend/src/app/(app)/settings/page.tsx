'use client';
import React, { useState } from 'react';
import Link from 'next/link';
import PageHeader from '@/components/ui/PageHeader';
import Card from '@/components/ui/Card';
import Btn from '@/components/ui/Button';
import Input from '@/components/ui/Input';
import Toggle from '@/components/ui/Toggle';
import Divider from '@/components/ui/Divider';
import { IconUser, IconShield, IconBell, IconAlert } from '@/components/icons';
import { useAuthStore } from '@/stores/authStore';
import { formatError } from '@/lib/connectError';
import { rpcFetch } from '@/lib/rpc';

const NAV = [
  { id: 'general', label: 'General', icon: <IconUser size={14}/> },
  { id: 'notifications', label: 'Notifications', icon: <IconBell size={14}/> },
  { id: 'security', label: 'Security', icon: <IconShield size={14}/> },
];

export default function PageSettings() {
  const { username } = useAuthStore();
  const [active, setActive] = useState('general');
  const [oldPass, setOldPass] = useState('');
  const [newPass, setNewPass] = useState('');
  const [confirmPass, setConfirmPass] = useState('');
  const [revokeOtherSessions, setRevokeOtherSessions] = useState(false);
  const [passLoading, setPassLoading] = useState(false);
  const [passError, setPassError] = useState('');
  const [passSuccess, setPassSuccess] = useState(false);
  const [notifBreaking, setNotifBreaking] = useState(true);
  const [notifCommits, setNotifCommits] = useState(true);
  const [notifSDKs, setNotifSDKs] = useState(false);

  const changePassword = async () => {
    if (!oldPass || !newPass) { setPassError('Fill in all fields.'); return; }
    if (newPass !== confirmPass) { setPassError('New passwords do not match.'); return; }
    setPassLoading(true); setPassError(''); setPassSuccess(false);
    try {
      await rpcFetch('/hades.api.authentication.v1.AuthenticationService/ChangePassword', { oldPassword: oldPass, newPassword: newPass, revokeOtherSessions });
      setPassSuccess(true); setOldPass(''); setNewPass(''); setConfirmPass('');
    } catch (e) { setPassError(formatError(e)); } finally { setPassLoading(false); }
  };

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader title="Settings" subtitle="Manage your account preferences and security settings."/>
      <div style={{ padding: '24px 32px', display: 'flex', gap: 24 }}>
        <div style={{ width: 200, flexShrink: 0 }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {NAV.map(item => (
              <button key={item.id} onClick={() => setActive(item.id)} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px', borderRadius: 6, background: active === item.id ? 'var(--c-accent-bg)' : 'transparent', border: 'none', borderLeft: `2px solid ${active === item.id ? 'var(--c-accent)' : 'transparent'}`, cursor: 'pointer', fontFamily: 'inherit', fontSize: 13, fontWeight: 500, color: active === item.id ? 'var(--c-accent)' : 'var(--c-fg-muted)', textAlign: 'left' }}>
                {item.icon}{item.label}
              </button>
            ))}
            <Divider style={{ margin: '8px 0' }}/>
            <Link href="/settings/tokens" style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px', borderRadius: 6, textDecoration: 'none', fontSize: 13, fontWeight: 500, color: 'var(--c-fg-muted)', borderLeft: '2px solid transparent' }}>API Tokens</Link>
            <Link href="/settings/sessions" style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px', borderRadius: 6, textDecoration: 'none', fontSize: 13, fontWeight: 500, color: 'var(--c-fg-muted)', borderLeft: '2px solid transparent' }}>Sessions</Link>
          </div>
        </div>

        <div style={{ flex: 1, maxWidth: 640 }}>
          {active === 'general' && (
            <Card style={{ padding: 24 }}>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 20 }}>General</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div>
                  <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Username</label>
                  <Input value={username || ''} onChange={() => {}} disabled style={{ color: 'var(--c-fg-subtle)' }}/>
                  <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', marginTop: 4 }}>Username cannot be changed.</div>
                </div>
                <div style={{ padding: '16px', borderRadius: 8, background: 'var(--c-bg-overlay)', fontSize: 13, color: 'var(--c-fg-muted)' }}>
                  Profile editing (display name, bio, avatar) is coming soon.
                </div>
              </div>
            </Card>
          )}

          {active === 'notifications' && (
            <Card style={{ padding: 24 }}>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 20 }}>Notifications</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
                {[
                  { label: 'Breaking changes', sub: 'Get notified when a module introduces a breaking change.', val: notifBreaking, set: setNotifBreaking },
                  { label: 'New commits', sub: 'Receive notifications when modules you follow are updated.', val: notifCommits, set: setNotifCommits },
                  { label: 'SDK generation', sub: 'Get notified when SDK generation completes or fails.', val: notifSDKs, set: setNotifSDKs },
                ].map((item, i) => (
                  <React.Fragment key={item.label}>
                    {i > 0 && <Divider/>}
                    <div style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '14px 0' }}>
                      <div style={{ flex: 1 }}>
                        <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--c-fg)', marginBottom: 3 }}>{item.label}</div>
                        <div style={{ fontSize: 12, color: 'var(--c-fg-muted)' }}>{item.sub}</div>
                      </div>
                      <Toggle checked={item.val} onChange={item.set}/>
                    </div>
                  </React.Fragment>
                ))}
              </div>
            </Card>
          )}

          {active === 'security' && (
            <Card style={{ padding: 24 }}>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 20 }}>Change Password</div>
              {passError && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8 }}><IconAlert size={14}/>{passError}</div>}
              {passSuccess && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-success-bg)', border: '1px solid var(--c-success)', color: 'var(--c-success)', fontSize: 13, marginBottom: 16 }}>Password changed successfully.</div>}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                <div><label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Current password</label><Input value={oldPass} onChange={setOldPass} type="password" placeholder="••••••••"/></div>
                <div><label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>New password</label><Input value={newPass} onChange={setNewPass} type="password" placeholder="Minimum 8 characters"/></div>
                <div><label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Confirm new password</label><Input value={confirmPass} onChange={setConfirmPass} type="password" placeholder="Re-enter new password"/></div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0' }}>
                  <div style={{ flex: 1 }}><div style={{ fontSize: 13, fontWeight: 500, color: 'var(--c-fg)' }}>Revoke all other sessions</div><div style={{ fontSize: 12, color: 'var(--c-fg-muted)', marginTop: 2 }}>Sign out of all other devices after changing password</div></div>
                  <Toggle checked={revokeOtherSessions} onChange={setRevokeOtherSessions}/>
                </div>
                <Btn variant="primary" onClick={changePassword} disabled={passLoading}>{passLoading ? 'Saving…' : 'Change Password'}</Btn>
              </div>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
