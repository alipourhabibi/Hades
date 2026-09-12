'use client';
import React, { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { Suspense } from 'react';
import Avatar from '@/components/ui/Avatar';
import Badge from '@/components/ui/Badge';
import Btn from '@/components/ui/Button';
import EmptyState from '@/components/ui/EmptyState';
import Input from '@/components/ui/Input';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import { IconBox, IconSearch, IconGitCommit, IconGlobe, IconLock, IconPlus, IconCode, IconStar, IconPackage } from '@/components/icons';
import { addRecentModule, getUsername } from '@/lib/auth';
import { rpcFetch } from '@/lib/rpc';
import { useWizardStore } from '@/stores/wizardStore';

interface Module {
  id: string;
  name: string;
  ownerId: string;
  visibility: string | number;
  description: string;
  defaultBranch: string;
  createTime?: string;
  updateTime?: string;
}

function isPublic(v: string | number): boolean {
  return v === 'MODULE_VISIBILITY_PUBLIC' || v === 1;
}

function isPrivate(v: string | number): boolean {
  return v === 'MODULE_VISIBILITY_PRIVATE' || v === 2;
}

type Tab = 'mine' | 'browse';

function ModuleList({ modules, onModuleClick }: { modules: Module[]; onModuleClick: (mod: Module) => void }) {
  if (modules.length === 0) return null;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 1, border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden' }}>
      {modules.map((mod, i) => {
        const parts = mod.name.split('/');
        const owner = parts[0];
        const modName = parts.slice(1).join('/');
        const pub = isPublic(mod.visibility);
        return (
          <div
            key={mod.id}
            onClick={() => onModuleClick(mod)}
            style={{ padding: '16px 20px', background: 'var(--c-bg-default)', cursor: 'pointer', display: 'flex', gap: 16, alignItems: 'flex-start', borderBottom: i < modules.length - 1 ? '1px solid var(--c-border-muted)' : 'none' }}
            onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'}
            onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-default)'}
          >
            <Avatar initials={owner.slice(0, 2).toUpperCase()} size={36}/>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-accent)', fontFamily: "'IBM Plex Mono', monospace" }}>
                  {owner}/{modName}
                </span>
                <Badge variant={pub ? 'blue' : 'default'}>
                  {pub ? <><IconGlobe size={10}/> public</> : <><IconLock size={10}/> private</>}
                </Badge>
              </div>
              {mod.description && (
                <p style={{ margin: '0 0 8px', fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5 }}>
                  {mod.description}
                </p>
              )}
              <div style={{ display: 'flex', gap: 16, fontSize: 11, color: 'var(--c-fg-subtle)', flexWrap: 'wrap' }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><IconCode size={11}/>protobuf</span>
                {mod.defaultBranch && (
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><IconGitCommit size={11}/>{mod.defaultBranch}</span>
                )}
                {mod.updateTime && (
                  <span style={{ marginLeft: 'auto' }}>Updated {new Date(mod.updateTime).toLocaleDateString()}</span>
                )}
              </div>
            </div>
            <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
              <Badge variant="default" style={{ fontSize: 10 }}><IconStar size={10}/> 0</Badge>
              <Badge variant="default" style={{ fontSize: 10 }}><IconPackage size={10}/></Badge>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function HomeContent() {
  const router = useRouter();
  const { openWizard } = useWizardStore();
  const [currentUser] = useState(() => getUsername());

  const [tab, setTab] = useState<Tab>(currentUser ? 'mine' : 'browse');

  const [myModules, setMyModules] = useState<Module[]>([]);
  const [myNextToken, setMyNextToken] = useState('');
  const [myLoading, setMyLoading] = useState(false);
  const [myLoadingMore, setMyLoadingMore] = useState(false);

  const [browseModules, setBrowseModules] = useState<Module[]>([]);
  const [browseNextToken, setBrowseNextToken] = useState('');
  const [browseLoading, setBrowseLoading] = useState(false);
  const [browseLoadingMore, setBrowseLoadingMore] = useState(false);
  const [browseSearch, setBrowseSearch] = useState('');

  const [error, setError] = useState<string | null>(null);

  const fetchMyModules = useCallback((pageToken: string, append: boolean) => {
    if (!currentUser) return;
    if (append) setMyLoadingMore(true); else setMyLoading(true);
    rpcFetch<{ modules: Module[]; nextPageToken?: string }>(
      '/hades.api.registry.v1.ModuleService/ListModules',
      { owner: currentUser, pageToken }
    )
      .then(res => {
        const mods = res.modules || [];
        setMyModules(prev => append ? [...prev, ...mods] : mods);
        setMyNextToken(res.nextPageToken || '');
      })
      .catch(e => setError(e.message))
      .finally(() => { if (append) setMyLoadingMore(false); else setMyLoading(false); });
  }, [currentUser]);

  const fetchBrowseModules = useCallback((pageToken: string, append: boolean) => {
    if (append) setBrowseLoadingMore(true); else setBrowseLoading(true);
    rpcFetch<{ modules: Module[]; nextPageToken?: string }>(
      '/hades.api.registry.v1.ModuleService/ListModules',
      { pageToken }
    )
      .then(res => {
        const mods = res.modules || [];
        setBrowseModules(prev => append ? [...prev, ...mods] : mods);
        setBrowseNextToken(res.nextPageToken || '');
      })
      .catch(e => setError(e.message))
      .finally(() => { if (append) setBrowseLoadingMore(false); else setBrowseLoading(false); });
  }, []);

  useEffect(() => {
    if (tab === 'mine' && currentUser) fetchMyModules('', false);
    if (tab === 'browse') fetchBrowseModules('', false);
  }, [tab, fetchMyModules, fetchBrowseModules, currentUser]);

  const handleModuleClick = (mod: Module) => {
    const parts = mod.name.split('/');
    const owner = parts[0];
    const modName = parts.slice(1).join('/');
    addRecentModule({ owner, name: modName, fullName: mod.name, visibility: isPublic(mod.visibility) ? 'public' : 'private' });
    router.push(`/${owner}/${modName}`);
  };

  const browsFiltered = browseModules.filter(mod =>
    !browseSearch ||
    mod.name.toLowerCase().includes(browseSearch.toLowerCase()) ||
    (mod.description || '').toLowerCase().includes(browseSearch.toLowerCase())
  );

  const tabs: { id: Tab; label: string }[] = currentUser
    ? [{ id: 'mine', label: 'My Modules' }, { id: 'browse', label: 'Browse' }]
    : [{ id: 'browse', label: 'Browse' }];

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader
        title="Schema Registry"
        subtitle="Browse and manage Protobuf modules across your organizations."
        actions={<Btn variant="primary" icon={<IconPlus size={14}/>} onClick={openWizard}>New Module</Btn>}
      />
      <Section>
        <div style={{ display: 'flex', gap: 4, marginBottom: 20, borderBottom: '1px solid var(--c-border)', paddingBottom: 0 }}>
          {tabs.map(t => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              style={{
                padding: '8px 16px',
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: tab === t.id ? 600 : 400,
                color: tab === t.id ? 'var(--c-fg-default)' : 'var(--c-fg-muted)',
                borderBottom: tab === t.id ? '2px solid var(--c-accent)' : '2px solid transparent',
                marginBottom: -1,
              }}
            >{t.label}</button>
          ))}
        </div>

        {error && <div className="status-error" style={{ marginBottom: 16 }}>{error}</div>}

        {tab === 'mine' && currentUser && (
          <>
            {myLoading ? (
              <div className="status-loading">Loading your modules…</div>
            ) : myModules.length === 0 ? (
              <EmptyState
                icon={<IconBox size={32}/>}
                title="No modules yet"
                subtitle="Create your first module to get started."
              />
            ) : (
              <>
                <ModuleList modules={myModules} onModuleClick={handleModuleClick}/>
                {myNextToken && (
                  <div style={{ marginTop: 12, textAlign: 'center' }}>
                    <Btn variant="ghost" size="sm" onClick={() => fetchMyModules(myNextToken, true)} disabled={myLoadingMore}>
                      {myLoadingMore ? 'Loading…' : 'Load more'}
                    </Btn>
                  </div>
                )}
              </>
            )}
          </>
        )}

        {tab === 'browse' && (
          <>
            <div style={{ marginBottom: 16 }}>
              <Input
                value={browseSearch}
                onChange={val => setBrowseSearch(val || '')}
                placeholder="Search modules…"
                prefix={<IconSearch size={14}/>}
              />
            </div>
            {browseLoading ? (
              <div className="status-loading">Loading modules…</div>
            ) : browsFiltered.length === 0 ? (
              <EmptyState
                icon={<IconBox size={32}/>}
                title="No modules found"
                subtitle={browseSearch ? 'Try adjusting your search.' : 'No public modules available.'}
              />
            ) : (
              <>
                <ModuleList modules={browsFiltered} onModuleClick={handleModuleClick}/>
                {browseNextToken && !browseSearch && (
                  <div style={{ marginTop: 12, textAlign: 'center' }}>
                    <Btn variant="ghost" size="sm" onClick={() => fetchBrowseModules(browseNextToken, true)} disabled={browseLoadingMore}>
                      {browseLoadingMore ? 'Loading…' : 'Load more'}
                    </Btn>
                  </div>
                )}
              </>
            )}
          </>
        )}
      </Section>
    </div>
  );
}

export default function HomePage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <HomeContent />
    </Suspense>
  );
}
