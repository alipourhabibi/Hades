'use client';
import React, { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Suspense } from 'react';
import Avatar from '@/components/ui/Avatar';
import Badge from '@/components/ui/Badge';
import Btn from '@/components/ui/Button';
import EmptyState from '@/components/ui/EmptyState';
import Input from '@/components/ui/Input';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import { IconBox, IconSearch, IconGitCommit, IconGlobe, IconLock, IconPlus, IconCode, IconStar, IconPackage } from '@/components/icons';
import { addRecentModule } from '@/lib/auth';
import { rpcFetch } from '@/lib/rpc';

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
  return v === 'E_VISIBILITY_PUBLIC' || v === 1;
}

function isPrivate(v: string | number): boolean {
  return v === 'E_VISIBILITY_PRIVATE' || v === 2;
}

type Filter = 'all' | 'public' | 'private';

function HomeContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [modules, setModules] = useState<Module[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const filter = (searchParams.get('filter') as Filter) ?? 'all';
  const search = searchParams.get('q') ?? '';

  const updateParam = (key: string, val: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (val) params.set(key, val); else params.delete(key);
    router.replace(`/?${params.toString()}`);
  };

  useEffect(() => {
    setLoading(true);
    rpcFetch<{ modules: Module[] }>('/hades.api.registry.v1.ModuleService/ListModules', {})
      .then(res => setModules(res.modules || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const filtered = modules.filter(mod => {
    const matchSearch =
      !search ||
      mod.name.toLowerCase().includes(search.toLowerCase()) ||
      (mod.description || '').toLowerCase().includes(search.toLowerCase());
    const matchFilter =
      filter === 'all' ||
      (filter === 'public' && isPublic(mod.visibility)) ||
      (filter === 'private' && isPrivate(mod.visibility));
    return matchSearch && matchFilter;
  });

  const handleModuleClick = (mod: Module) => {
    const parts = mod.name.split('/');
    const owner = parts[0];
    const modName = parts.slice(1).join('/');
    addRecentModule({ owner, name: modName, fullName: mod.name, visibility: isPublic(mod.visibility) ? 'public' : 'private' });
    router.push(`/${owner}/${modName}`);
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 300 }}>
        <div className="status-loading">Loading modules…</div>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 300 }}>
        <div className="status-error">{error}</div>
      </div>
    );
  }

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader
        title="Schema Registry"
        subtitle="Browse and manage Protobuf modules across your organizations."
        actions={<Btn variant="primary" icon={<IconPlus size={14}/>} onClick={() => {}}>New Module</Btn>}
      />
      <Section>
        <div style={{ display: 'flex', gap: 10, marginBottom: 16 }}>
          <div style={{ flex: 1 }}>
            <Input
              value={search}
              onChange={val => updateParam('q', val || null)}
              placeholder="Search modules…"
              prefix={<IconSearch size={14}/>}
            />
          </div>
        </div>

        <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
          {(['all', 'public', 'private'] as Filter[]).map(f => (
            <Btn
              key={f}
              size="sm"
              variant={filter === f ? 'primary' : 'ghost'}
              onClick={() => updateParam('filter', f === 'all' ? null : f)}
              style={{ textTransform: 'capitalize' }}
            >{f}</Btn>
          ))}
          <span style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--c-fg-subtle)', alignSelf: 'center' }}>
            {filtered.length} module{filtered.length !== 1 ? 's' : ''}
          </span>
        </div>

        {filtered.length === 0 ? (
          <EmptyState
            icon={<IconBox size={32}/>}
            title="No modules found"
            subtitle={search ? 'Try adjusting your search or filters.' : 'No modules have been created yet.'}
          />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 1, border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden' }}>
            {filtered.map((mod, i) => {
              const parts = mod.name.split('/');
              const owner = parts[0];
              const modName = parts.slice(1).join('/');
              const pub = isPublic(mod.visibility);

              return (
                <div
                  key={mod.id}
                  onClick={() => handleModuleClick(mod)}
                  style={{ padding: '16px 20px', background: 'var(--c-bg-default)', cursor: 'pointer', display: 'flex', gap: 16, alignItems: 'flex-start', borderBottom: i < filtered.length - 1 ? '1px solid var(--c-border-muted)' : 'none' }}
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
                        <span style={{ marginLeft: 'auto' }}>
                          Updated {new Date(mod.updateTime).toLocaleDateString()}
                        </span>
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
