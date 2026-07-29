'use client';
import React, { useState, useEffect } from 'react';
import Link from 'next/link';
import PageHeader from '@/components/ui/PageHeader';
import Card from '@/components/ui/Card';
import Btn from '@/components/ui/Button';
import Input from '@/components/ui/Input';
import Toggle from '@/components/ui/Toggle';
import Divider from '@/components/ui/Divider';
import Badge from '@/components/ui/Badge';
import { IconUser, IconShield, IconBell, IconAlert, IconCreditCard, IconCheck } from '@/components/icons';
import { useAuthStore } from '@/stores/authStore';
import { formatError } from '@/lib/connectError';
import { rpcFetch } from '@/lib/rpc';

const NOTIF_KEY = 'hades:notification-prefs';

const NAV = [
  { id: 'general', label: 'General', icon: <IconUser size={14}/> },
  { id: 'notifications', label: 'Notifications', icon: <IconBell size={14}/> },
  { id: 'security', label: 'Security', icon: <IconShield size={14}/> },
  { id: 'billing', label: 'Billing', icon: <IconCreditCard size={14}/> },
];

export default function PageSettings() {
  const { username } = useAuthStore();
  const [active, setActive] = useState('general');
  const [profileDesc, setProfileDesc] = useState('');
  const [profileUrl, setProfileUrl] = useState('');
  const [profileSaving, setProfileSaving] = useState(false);
  const [profileError, setProfileError] = useState('');
  const [profileSuccess, setProfileSuccess] = useState(false);
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
  const [notifSaved, setNotifSaved] = useState(false);

  useEffect(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(NOTIF_KEY) || '{}');
      if ('breaking' in saved) setNotifBreaking(saved.breaking);
      if ('commits' in saved) setNotifCommits(saved.commits);
      if ('sdks' in saved) setNotifSDKs(saved.sdks);
    } catch { /* ignore */ }
  }, []);

  const saveProfile = async () => {
    setProfileSaving(true); setProfileError(''); setProfileSuccess(false);
    try {
      await rpcFetch('/hades.api.identity.v1.UserService/UpdateUser', { description: profileDesc, url: profileUrl });
      setProfileSuccess(true);
    } catch (e) { setProfileError(formatError(e)); } finally { setProfileSaving(false); }
  };

  const saveNotifications = () => {
    localStorage.setItem(NOTIF_KEY, JSON.stringify({ breaking: notifBreaking, commits: notifCommits, sdks: notifSDKs }));
    setNotifSaved(true);
    setTimeout(() => setNotifSaved(false), 2000);
  };

  const changePassword = async () => {
    if (!oldPass || !newPass) { setPassError('Fill in all fields.'); return; }
    if (newPass !== confirmPass) { setPassError('New passwords do not match.'); return; }
    setPassLoading(true); setPassError(''); setPassSuccess(false);
    try {
      await rpcFetch('/hades.api.auth.v1.AuthenticationService/ChangePassword', { oldPassword: oldPass, newPassword: newPass, revokeOtherSessions });
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
                <div>
                  <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Bio</label>
                  <Input value={profileDesc} onChange={setProfileDesc} placeholder="Tell others about yourself…"/>
                </div>
                <div>
                  <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Website</label>
                  <Input value={profileUrl} onChange={setProfileUrl} placeholder="https://example.com"/>
                </div>
                {profileError && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, display: 'flex', gap: 8 }}><IconAlert size={14}/>{profileError}</div>}
                {profileSuccess && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-success-bg)', border: '1px solid var(--c-success)', color: 'var(--c-success)', fontSize: 13 }}>Profile updated.</div>}
                <Btn variant="primary" onClick={saveProfile} disabled={profileSaving}>{profileSaving ? 'Saving…' : 'Save profile'}</Btn>
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
              <div style={{ marginTop: 20, display: 'flex', alignItems: 'center', gap: 12 }}>
                <Btn variant="primary" onClick={saveNotifications}>{notifSaved ? <><IconCheck size={12}/> Saved</> : 'Save preferences'}</Btn>
                {notifSaved && <span style={{ fontSize: 12, color: 'var(--c-success)' }}>Preferences saved locally.</span>}
              </div>
            </Card>
          )}

          {active === 'billing' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Card style={{ padding: 24 }}>
                <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 4 }}>Current plan</div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 20 }}>
                  <span style={{ fontSize: 28, fontWeight: 700, color: 'var(--c-accent)' }}>Free</span>
                  <Badge variant="green">Active</Badge>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {[
                    ['Public modules', 'Unlimited'],
                    ['Private modules', 'Unlimited'],
                    ['Storage', 'Unlimited'],
                    ['SDK generation', 'Included'],
                    ['Team members', 'Unlimited'],
                  ].map(([feat, val]) => (
                    <div key={feat} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, padding: '8px 0', borderBottom: '1px solid var(--c-border-muted)' }}>
                      <span style={{ color: 'var(--c-fg-muted)' }}>{feat}</span>
                      <span style={{ fontWeight: 500, color: 'var(--c-fg)' }}>{val}</span>
                    </div>
                  ))}
                </div>
              </Card>
              <Card style={{ padding: 24, background: 'var(--c-bg-overlay)' }}>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 8 }}>Paid plans coming soon</div>
                <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.6 }}>
                  Hades is currently free for all users. Paid plans with additional features (priority support, SLA guarantees, advanced analytics) are planned for a future release.
                </div>
              </Card>
            </div>
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
