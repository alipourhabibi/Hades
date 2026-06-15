'use client';
import React, { useEffect, useState, useCallback, useRef, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import Avatar from '@/components/ui/Avatar';
import Badge from '@/components/ui/Badge';
import Card from '@/components/ui/Card';
import EmptyState from '@/components/ui/EmptyState';
import Input from '@/components/ui/Input';
import PageHeader from '@/components/ui/PageHeader';
import { IconSearch, IconBox, IconGlobe, IconLock, IconUser, IconBuilding } from '@/components/icons';
import { rpcFetch } from '@/lib/rpc';

interface Module { id: string; name: string; ownerId: string; visibility: string | number; description?: string; updateTime?: string; }
interface UserResult { id: string; username: string; description?: string; email?: string; }
interface OrgResult { id: string; username: string; description?: string; }

type TypeFilter = 'all' | 'modules' | 'users' | 'orgs';

const TYPE_FILTERS: { id: TypeFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'modules', label: 'Modules' },
  { id: 'users', label: 'Users' },
  { id: 'orgs', label: 'Organizations' },
];

function isPublic(v: string | number): boolean {
  return v === 'E_VISIBILITY_PUBLIC' || v === 1;
}

function SearchContent() {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [query, setQuery] = useState(searchParams.get('q') || '');
  const typeFilter = (searchParams.get('type') as TypeFilter) ?? 'all';
  const [modules, setModules] = useState<Module[]>([]);
  const [users, setUsers] = useState<UserResult[]>([]);
  const [orgs, setOrgs] = useState<OrgResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const updateParam = (key: string, val: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (val) params.set(key, val); else params.delete(key);
    router.replace(`/search?${params.toString()}`);
  };

  const doSearch = useCallback((q: string) => {
    setLoading(true); setError(null); setSearched(true);
    const trimmed = q.trim();
    Promise.all([
      rpcFetch<{ modules: Module[] }>('/hades.api.registry.v1.ModuleService/ListModules', trimmed ? { owner: trimmed } : {}),
      rpcFetch<{ users: UserResult[] }>('/hades.api.registry.v1.UserService/ListUsers', { query: trimmed }),
      rpcFetch<{ organizations: OrgResult[] }>('/hades.api.registry.v1.OrgService/ListOrganizations', { query: trimmed }),
    ])
      .then(([modRes, userRes, orgRes]) => {
        setModules(modRes.modules || []);
        setUsers(userRes.users || []);
        setOrgs(orgRes.organizations || []);
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const q = searchParams.get('q') || '';
    setQuery(q);
    if (q) doSearch(q);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleQueryChange = useCallback((val: string) => {
    setQuery(val);
    updateParam('q', val || null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (val.trim()) {
      debounceRef.current = setTimeout(() => doSearch(val.trim()), 400);
    } else {
      setModules([]); setUsers([]); setOrgs([]); setSearched(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doSearch, searchParams]);

  const showModules = typeFilter === 'all' || typeFilter === 'modules';
  const showUsers   = typeFilter === 'all' || typeFilter === 'users';
  const showOrgs    = typeFilter === 'all' || typeFilter === 'orgs';
  const totalCount = (showModules ? modules.length : 0) + (showUsers ? users.length : 0) + (showOrgs ? orgs.length : 0);

  const sectionLabel = (label: string) => (
    <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 10, marginTop: 20 }}>{label}</div>
  );

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
      <PageHeader title="Search" subtitle="Search modules, users, and organizations across the registry." />
      <div style={{ flex: 1, padding: '0 32px 32px' }}>
        <form onSubmit={e => { e.preventDefault(); if (query.trim()) doSearch(query.trim()); }} style={{ marginBottom: 20 }}>
          <Input value={query} onChange={handleQueryChange} placeholder="Search modules, users, organizations…" prefix={<IconSearch size={18} style={{ color: 'var(--c-fg-subtle)' }}/>} style={{ fontSize: 15 }}/>
        </form>

        <div style={{ display: 'flex', gap: 8, marginBottom: 24, flexWrap: 'wrap' }}>
          {TYPE_FILTERS.map(f => (
            <button key={f.id} onClick={() => updateParam('type', f.id === 'all' ? null : f.id)} style={{ padding: '5px 14px', fontSize: 13, fontWeight: 500, borderRadius: 20, border: `1px solid ${typeFilter === f.id ? 'var(--c-accent)' : 'var(--c-border)'}`, background: typeFilter === f.id ? 'var(--c-accent-bg)' : 'var(--c-bg-default)', color: typeFilter === f.id ? 'var(--c-accent)' : 'var(--c-fg-muted)', cursor: 'pointer', fontFamily: 'inherit', transition: 'all 0.1s' }}>
              {f.id === 'modules' && <IconBox size={12} style={{ marginRight: 4, verticalAlign: 'middle' }}/>}
              {f.id === 'users' && <IconUser size={12} style={{ marginRight: 4, verticalAlign: 'middle' }}/>}
              {f.id === 'orgs' && <IconBuilding size={12} style={{ marginRight: 4, verticalAlign: 'middle' }}/>}
              {f.label}
            </button>
          ))}
        </div>

        {searched && !loading && <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 16 }}>{totalCount} result{totalCount !== 1 ? 's' : ''}{query ? <> for <strong>&quot;{query}&quot;</strong></> : ''}</div>}
        {loading && <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '48px 0' }}><div className="status-loading">Searching…</div></div>}
        {error && <div className="status-error" style={{ padding: '16px 0' }}>{error}</div>}

        {!loading && !error && (
          <>
            {!searched && <EmptyState icon={<IconSearch size={40}/>} title="Search the registry" subtitle="Enter a name to find modules, users, or organizations."/>}
            {searched && totalCount === 0 && <EmptyState icon={<IconSearch size={40}/>} title={query ? `No results for "${query}"` : 'No results found'} subtitle="Try a different search term or check your spelling."/>}

            {showModules && modules.length > 0 && (
              <div>
                {typeFilter === 'all' && sectionLabel('Modules')}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {modules.map(mod => {
                    const parts = mod.name.split('/');
                    const ownerName = parts[0];
                    const modName = parts.slice(1).join('/');
                    return (
                      <Card key={mod.id} hover onClick={() => router.push(`/${ownerName}/${modName}`)} style={{ padding: '14px 18px' }}>
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                          <Avatar initials={ownerName.slice(0, 2)} size={32}/>
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginBottom: 3 }}>
                              <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 14, fontWeight: 600, color: 'var(--c-accent)' }}>{ownerName}/{modName}</span>
                              <Badge variant={isPublic(mod.visibility) ? 'blue' : 'default'}>{isPublic(mod.visibility) ? <><IconGlobe size={10}/> public</> : <><IconLock size={10}/> private</>}</Badge>
                              <Badge variant="default">module</Badge>
                            </div>
                            {mod.description && <p style={{ margin: 0, fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{mod.description}</p>}
                          </div>
                        </div>
                      </Card>
                    );
                  })}
                </div>
              </div>
            )}

            {showUsers && users.length > 0 && (
              <div>
                {typeFilter === 'all' && sectionLabel('Users')}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {users.map(u => (
                    <Card key={u.id} hover onClick={() => router.push(`/${u.username}`)} style={{ padding: '12px 16px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                        <Avatar initials={(u.username || 'U').slice(0, 2).toUpperCase()} size={36}/>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
                            <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>@{u.username}</span>
                            <Badge variant="default"><IconUser size={10}/> user</Badge>
                          </div>
                          {u.description && <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{u.description}</div>}
                        </div>
                      </div>
                    </Card>
                  ))}
                </div>
              </div>
            )}

            {showOrgs && orgs.length > 0 && (
              <div>
                {typeFilter === 'all' && sectionLabel('Organizations')}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {orgs.map(o => (
                    <Card key={o.id} hover onClick={() => router.push(`/${o.username}`)} style={{ padding: '12px 16px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                        <Avatar initials={(o.username || 'O').slice(0, 2).toUpperCase()} size={36} style={{ borderRadius: 8 }}/>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
                            <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>@{o.username}</span>
                            <Badge variant="purple"><IconBuilding size={10}/> org</Badge>
                          </div>
                          {o.description && <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{o.description}</div>}
                        </div>
                      </div>
                    </Card>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

export default function SearchPage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <SearchContent />
    </Suspense>
  );
}
