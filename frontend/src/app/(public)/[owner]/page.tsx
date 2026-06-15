'use client';
import React, { useEffect, useState } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import { Suspense } from 'react';
import Avatar from '@/components/ui/Avatar';
import Badge from '@/components/ui/Badge';
import Btn from '@/components/ui/Button';
import Card from '@/components/ui/Card';
import EmptyState from '@/components/ui/EmptyState';
import Input from '@/components/ui/Input';
import Section from '@/components/ui/Section';
import Tabs from '@/components/ui/Tabs';
import Table from '@/components/ui/Table';
import { useAuthStore } from '@/stores/authStore';
import {
  IconBox, IconUser, IconBuilding, IconGlobe, IconLock,
  IconGitCommit, IconClock, IconShield, IconLink, IconMail, IconGear,
} from '@/components/icons';
import { rpcFetch } from '@/lib/rpc';
import { getToken } from '@/lib/auth';

interface User { id: string; username: string; email?: string; description?: string; url?: string; type?: number | string; createTime?: string; updateTime?: string; }
interface OrgMember { user: { id: string; username: string }; role: string; }
interface Module { id: string; name: string; ownerId: string; visibility: number | string; description?: string; updateTime?: string; }

const ORG_TABS = [{ id: 'overview', label: 'Overview' }, { id: 'modules', label: 'Modules' }, { id: 'members', label: 'Members' }];
const USER_TABS = [{ id: 'modules', label: 'Modules' }, { id: 'orgs', label: 'Organizations' }, { id: 'activity', label: 'Activity' }];

function isPublic(v: string | number | undefined): boolean {
  return v === 'E_VISIBILITY_PUBLIC' || v === 1;
}

const ModuleList: React.FC<{ modules: Module[]; navigate: (path: string) => void }> = ({ modules, navigate }) => {
  if (modules.length === 0) {
    return <Section><EmptyState icon={<IconBox size={40}/>} title="No modules yet" subtitle="No public modules have been published." /></Section>;
  }
  return (
    <Section>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {modules.map(mod => {
          const parts = mod.name.split('/');
          const modOwner = parts[0];
          const modName = parts.slice(1).join('/');
          return (
            <Card key={mod.id} hover onClick={() => navigate(`/${modOwner}/${modName}`)} style={{ padding: '12px 16px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
                    <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, fontWeight: 600, color: 'var(--c-accent)' }}>{modOwner}/{modName}</span>
                    <Badge variant={isPublic(mod.visibility) ? 'blue' : 'default'}>{isPublic(mod.visibility) ? <><IconGlobe size={10}/> public</> : <><IconLock size={10}/> private</>}</Badge>
                  </div>
                  {mod.description && <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', lineHeight: 1.4 }}>{mod.description}</div>}
                </div>
                {mod.updateTime && <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', flexShrink: 0 }}>{new Date(mod.updateTime).toLocaleDateString()}</div>}
              </div>
            </Card>
          );
        })}
      </div>
    </Section>
  );
};

function ProfileContent() {
  const { owner = '' } = useParams<{ owner: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { username: loggedInUsername } = useAuthStore();
  const isOwnProfile = loggedInUsername === owner;

  const [isOrg, setIsOrg] = useState<boolean | null>(null);
  const [org, setOrg] = useState<User | null>(null);
  const [orgModuleCount, setOrgModuleCount] = useState(0);
  const [orgMemberCount, setOrgMemberCount] = useState(0);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [orgModules, setOrgModules] = useState<Module[]>([]);
  const [user, setUser] = useState<User | null>(null);
  const [userModuleCount, setUserModuleCount] = useState(0);
  const [userOrgs, setUserOrgs] = useState<User[]>([]);
  const [userModules, setUserModules] = useState<Module[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const activeTab = searchParams.get('tab') ?? '';
  const setTab = (tab: string, replace = false) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set('tab', tab);
    if (replace) router.replace(`/${owner}?${params.toString()}`);
    else router.push(`/${owner}?${params.toString()}`);
  };

  const [editOpen, setEditOpen] = useState(false);
  const [editDescription, setEditDescription] = useState('');
  const [editUrl, setEditUrl] = useState('');
  const [editSaving, setEditSaving] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);

  const handleSaveProfile = () => {
    setEditSaving(true); setEditError(null);
    rpcFetch<{ user: User }>('/hades.api.registry.v1.UserService/UpdateUser', { description: editDescription, url: editUrl })
      .then(res => { setUser(prev => prev ? { ...prev, ...res.user } : res.user); setEditOpen(false); })
      .catch(e => setEditError(e.message))
      .finally(() => setEditSaving(false));
  };

  useEffect(() => {
    if (!getToken()) { router.replace('/login'); return; }
    if (!owner) return;
    setLoading(true); setError(null);
    rpcFetch<{ user: User; moduleCount: number; organizations: User[] }>('/hades.api.registry.v1.UserService/GetUser', { username: owner })
      .then(res => {
        setIsOrg(false); setUser(res.user); setUserModuleCount(res.moduleCount || 0); setUserOrgs(res.organizations || []);
        if (!searchParams.get('tab')) setTab('modules', true);
        return rpcFetch<{ modules: Module[] }>('/hades.api.registry.v1.ModuleService/ListModules', { owner });
      })
      .then(modRes => { setUserModules(modRes.modules || []); setLoading(false); })
      .catch(() => {
        rpcFetch<{ org: User; moduleCount: number; memberCount: number }>('/hades.api.registry.v1.OrgService/GetOrg', { name: owner })
          .then(orgRes => {
            setIsOrg(true); setOrg(orgRes.org); setOrgModuleCount(orgRes.moduleCount || 0); setOrgMemberCount(orgRes.memberCount || 0);
            if (!searchParams.get('tab')) setTab('overview', true);
            return Promise.all([
              rpcFetch<{ modules: Module[] }>('/hades.api.registry.v1.ModuleService/ListModules', { owner }),
              rpcFetch<{ members: OrgMember[] }>('/hades.api.registry.v1.OrgService/ListOrgMembers', { orgName: owner }),
            ]);
          })
          .then(([modRes, memRes]) => { setOrgModules(modRes.modules || []); setMembers(memRes.members || []); setLoading(false); })
          .catch(e => { setError(e.message); setLoading(false); });
      });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner]);

  if (loading) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-loading">Loading profile…</div></div>;
  if (error || (!org && !user)) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-error">{error || 'Profile not found'}</div></div>;

  if (isOrg && org) {
    type MemberRow = { username: string; role: string; _member: OrgMember };
    const memberRows: MemberRow[] = members.map(m => ({ username: m.user?.username || '', role: m.role || '', _member: m }));
    return (
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <div style={{ padding: '28px 32px 20px', borderBottom: '1px solid var(--c-border)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 20, flexWrap: 'wrap' }}>
            <Avatar initials={(org.username || '').slice(0, 2).toUpperCase()} size={60} style={{ borderRadius: 12 }}/>
            <div style={{ flex: 1 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 4 }}>
                <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: 'var(--c-fg)', letterSpacing: -0.3 }}>{org.username}</h1>
                <Badge variant="purple"><IconBuilding size={10}/> organization</Badge>
              </div>
              <div style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 4 }}>@{org.username}</div>
              {org.description && <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5, marginBottom: 6 }}>{org.description}</div>}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 24, marginTop: 16, flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--c-fg-muted)' }}><IconUser size={14}/><strong style={{ color: 'var(--c-fg)' }}>{orgMemberCount}</strong> member{orgMemberCount !== 1 ? 's' : ''}</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--c-fg-muted)' }}><IconBox size={14}/><strong style={{ color: 'var(--c-fg)' }}>{orgModuleCount}</strong> module{orgModuleCount !== 1 ? 's' : ''}</div>
          </div>
        </div>
        <Tabs tabs={ORG_TABS} active={activeTab} onChange={tab => setTab(tab)}/>
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {activeTab === 'overview' && (
            <div>
              <Section>
                <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap' }}>
                  <Card style={{ flex: 1, minWidth: 140, padding: '18px 20px', textAlign: 'center' }}><div style={{ fontSize: 28, fontWeight: 700, color: 'var(--c-accent)' }}>{orgModuleCount}</div><div style={{ fontSize: 12, color: 'var(--c-fg-muted)', marginTop: 4 }}><IconBox size={12}/> Modules</div></Card>
                  <Card style={{ flex: 1, minWidth: 140, padding: '18px 20px', textAlign: 'center' }}><div style={{ fontSize: 28, fontWeight: 700, color: 'var(--c-accent)' }}>{orgMemberCount}</div><div style={{ fontSize: 12, color: 'var(--c-fg-muted)', marginTop: 4 }}><IconUser size={12}/> Members</div></Card>
                </div>
              </Section>
            </div>
          )}
          {activeTab === 'modules' && <ModuleList modules={orgModules} navigate={router.push}/>}
          {activeTab === 'members' && (
            <Section>
              {memberRows.length === 0 ? <EmptyState icon={<IconUser size={40}/>} title="No members"/> : (
                <Table<MemberRow> columns={[
                  { key: 'username', label: 'Member', render: (val) => <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><Avatar initials={String(val).slice(0, 2).toUpperCase()} size={24}/><span style={{ cursor: 'pointer', color: 'var(--c-accent)' }} onClick={() => router.push(`/${val}`)}>@{String(val)}</span></div> },
                  { key: 'role', label: 'Role', render: val => <Badge variant={String(val) === 'admin' ? 'purple' : 'default'}><IconShield size={10}/> {String(val) || 'member'}</Badge> },
                ]} rows={memberRows}/>
              )}
            </Section>
          )}
        </div>
      </div>
    );
  }

  if (!user) return null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ padding: '28px 32px 20px', borderBottom: '1px solid var(--c-border)' }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 20, flexWrap: 'wrap' }}>
          <Avatar initials={(user.username || 'U').slice(0, 2).toUpperCase()} size={72} style={{ borderRadius: 12 }}/>
          <div style={{ flex: 1 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4, flexWrap: 'wrap' }}>
              <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: 'var(--c-fg)', letterSpacing: -0.3 }}>{user.username}</h1>
              <Badge variant="default"><IconUser size={10}/> user</Badge>
            </div>
            <div style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 6 }}>@{owner}</div>
            {user.description && <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5, marginBottom: 6 }}>{user.description}</div>}
            <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginTop: 6 }}>
              {user.email && <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--c-fg-muted)' }}><IconMail size={12}/> {user.email}</div>}
              {user.url && <a href={user.url} target="_blank" rel="noopener noreferrer" style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--c-accent)', textDecoration: 'none' }}><IconLink size={12}/> {user.url}</a>}
              {user.createTime && <div style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--c-fg-muted)' }}><IconClock size={12}/> Joined {new Date(user.createTime).toLocaleDateString()}</div>}
            </div>
            <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap', marginTop: 10 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--c-fg-muted)' }}><IconBox size={14}/><strong style={{ color: 'var(--c-fg)' }}>{userModuleCount}</strong> module{userModuleCount !== 1 ? 's' : ''}</div>
              {userOrgs.length > 0 && <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--c-fg-muted)' }}><IconBuilding size={14}/><strong style={{ color: 'var(--c-fg)' }}>{userOrgs.length}</strong> org{userOrgs.length !== 1 ? 's' : ''}</div>}
            </div>
          </div>
          {isOwnProfile && <Btn icon={<IconGear size={13}/>} onClick={() => { setEditDescription(user?.description || ''); setEditUrl(user?.url || ''); setEditError(null); setEditOpen(true); }} style={{ flexShrink: 0, alignSelf: 'flex-start' }}>Edit Profile</Btn>}
        </div>
      </div>

      <Tabs tabs={USER_TABS} active={activeTab} onChange={tab => setTab(tab)}/>

      <div style={{ flex: 1, overflowY: 'auto' }}>
        {activeTab === 'modules' && <ModuleList modules={userModules} navigate={router.push}/>}
        {activeTab === 'orgs' && (
          <Section>
            {userOrgs.length === 0 ? <EmptyState icon={<IconBuilding size={40}/>} title="No organizations" subtitle="Organizations this user belongs to will appear here."/> : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {userOrgs.map(o => (
                  <Card key={o.id} hover onClick={() => router.push(`/${o.username}`)} style={{ padding: '14px 18px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
                      <Avatar initials={(o.username || '').slice(0, 2).toUpperCase()} size={40} style={{ borderRadius: 8 }}/>
                      <div style={{ flex: 1 }}>
                        <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 2 }}>{o.username}</div>
                        {o.description && <div style={{ fontSize: 12, color: 'var(--c-fg-muted)' }}>{o.description}</div>}
                      </div>
                      <Badge variant="purple"><IconBuilding size={10}/> org</Badge>
                    </div>
                  </Card>
                ))}
              </div>
            )}
          </Section>
        )}
        {activeTab === 'activity' && (
          <Section>
            {userModules.length === 0 ? <EmptyState icon={<IconGitCommit size={40}/>} title="No recent activity" subtitle="Module pushes and commits will appear here."/> : (
              <div style={{ position: 'relative', paddingLeft: 32 }}>
                <div style={{ position: 'absolute', left: 11, top: 8, bottom: 8, width: 2, background: 'var(--c-border)' }}/>
                {userModules.slice(0, 10).map(mod => {
                  const parts = mod.name.split('/');
                  return (
                    <div key={mod.id} style={{ position: 'relative', marginBottom: 20 }}>
                      <div style={{ position: 'absolute', left: -28, top: 3, width: 10, height: 10, borderRadius: '50%', background: 'var(--c-accent)', border: '2px solid var(--c-bg-default)', zIndex: 1 }}/>
                      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
                        <span style={{ fontSize: 13, color: 'var(--c-fg-muted)' }}>Pushed to</span>
                        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, color: 'var(--c-accent)', cursor: 'pointer' }} onClick={() => router.push(`/${parts[0]}/${parts.slice(1).join('/')}`)}>
                          {mod.name}
                        </span>
                        {mod.updateTime && <span style={{ fontSize: 11, color: 'var(--c-fg-subtle)' }}>{new Date(mod.updateTime).toLocaleDateString()}</span>}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </Section>
        )}
      </div>

      {editOpen && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 1000, background: 'rgba(0,0,0,0.55)', display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => setEditOpen(false)}>
          <div onClick={e => e.stopPropagation()} style={{ background: 'var(--c-bg-default)', border: '1px solid var(--c-border)', borderRadius: 12, padding: '28px 28px 24px', width: 420, boxShadow: '0 12px 40px rgba(0,0,0,0.45)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}><IconGear size={16} style={{ color: 'var(--c-fg-muted)' }}/><span style={{ fontSize: 16, fontWeight: 700, color: 'var(--c-fg)' }}>Edit Profile</span></div>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: 'block', fontSize: 12, fontWeight: 600, color: 'var(--c-fg-muted)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: 0.4 }}>Bio</label>
              <textarea value={editDescription} onChange={e => setEditDescription(e.target.value)} placeholder="Tell people a little about yourself…" rows={3} style={{ width: '100%', boxSizing: 'border-box', background: 'var(--c-bg-inset)', border: '1px solid var(--c-border)', borderRadius: 6, color: 'var(--c-fg)', fontSize: 13, fontFamily: 'inherit', padding: '8px 12px', outline: 'none', resize: 'vertical', lineHeight: 1.5 }}/>
            </div>
            <div style={{ marginBottom: 20 }}>
              <label style={{ display: 'block', fontSize: 12, fontWeight: 600, color: 'var(--c-fg-muted)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: 0.4 }}>Website / URL</label>
              <Input value={editUrl} onChange={setEditUrl} placeholder="https://example.com" prefix={<IconLink size={14}/>}/>
            </div>
            {editError && <div style={{ fontSize: 12, color: 'var(--c-danger)', marginBottom: 14, padding: '8px 12px', background: 'var(--c-danger-bg)', borderRadius: 6 }}>{editError}</div>}
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Btn size="sm" variant="ghost" onClick={() => setEditOpen(false)} disabled={editSaving}>Cancel</Btn>
              <Btn size="sm" variant="primary" onClick={handleSaveProfile} disabled={editSaving}>{editSaving ? 'Saving…' : 'Save changes'}</Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default function ProfilePage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <ProfileContent/>
    </Suspense>
  );
}
