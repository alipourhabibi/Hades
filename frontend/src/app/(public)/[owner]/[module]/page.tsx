'use client';
import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Avatar from '@/components/ui/Avatar';
import Card from '@/components/ui/Card';
import CodeBlock from '@/components/ui/CodeBlock';
import Divider from '@/components/ui/Divider';
import EmptyState from '@/components/ui/EmptyState';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import Stat from '@/components/ui/Stat';
import Table from '@/components/ui/Table';
import Tabs from '@/components/ui/Tabs';
import Toggle from '@/components/ui/Toggle';
import Btn from '@/components/ui/Button';
import Input from '@/components/ui/Input';
import FileViewer from '@/components/ui/FileViewer';
import {
  IconBox, IconGitCommit, IconTag, IconCode, IconGlobe, IconLock, IconPackage,
  IconClock, IconBranch, IconDownload, IconStar, IconFolder, IconFile, IconChevronRight,
} from '@/components/icons';
import { addRecentModule } from '@/lib/auth';
import { DOMAIN } from '@/lib/config';
import { rpcFetch } from '@/lib/rpc';

interface Module { id: string; name: string; ownerId: string; visibility: string | number; description: string; defaultBranch: string; createTime?: string; updateTime?: string; }
interface Commit { id: string; commitHash: string; createTime?: string; ownerId?: string; moduleId?: string; }
interface SDK { id: string; moduleId: string; commitId?: string; language: string; plugin?: string; status?: string; outputLocation?: string; }
type FileEntryType = number | string;
interface FileEntry { name: string; path: string; type: FileEntryType; oid: string; mode: number; }

function isEntryDir(type: FileEntryType): boolean { return type === 2 || type === 'FILE_ENTRY_TYPE_DIR'; }
function isPublic(v: string | number): boolean { return v === 'E_VISIBILITY_PUBLIC' || v === 1; }
function fmtDate(ts?: string): string { if (!ts) return '-'; try { return new Date(ts).toLocaleDateString(); } catch { return ts; } }

const LANG_EMOJIS: Record<string, string> = { go: '🐹', typescript: '🔷', python: '🐍', java: '☕', rust: '🦀', swift: '🦅' };
function getLangEmoji(lang: string): string { return LANG_EMOJIS[lang.toLowerCase()] || '📦'; }
function getInstallCmd(lang: string, owner: string, mod: string): string {
  switch (lang.toLowerCase()) {
    case 'go': return `go get ${DOMAIN}/${owner}/${mod}/gen/go`;
    case 'typescript': return `npm install @buf/${owner}_${mod}`;
    case 'python': return `pip install buf-${owner}-${mod}`;
    default: return `# Install ${lang} SDK for ${owner}/${mod}`;
  }
}

const MODULE_TABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'files', label: 'Files' },
  { id: 'commits', label: 'Commits' },
  { id: 'versions', label: 'Versions' },
  { id: 'sdks', label: 'SDKs' },
  { id: 'dependencies', label: 'Dependencies' },
  { id: 'ci', label: 'CI / Lint' },
  { id: 'settings', label: 'Settings' },
];

interface FileRowProps { icon: React.ReactNode; name: string; oid: string; isDir: boolean; isLast: boolean; nameStyle?: React.CSSProperties; onClick: () => void; }
const FileRow: React.FC<FileRowProps> = ({ icon, name, oid, isDir, isLast, nameStyle, onClick }) => {
  const [hovered, setHovered] = React.useState(false);
  return (
    <div role="button" onClick={onClick} onMouseEnter={() => setHovered(true)} onMouseLeave={() => setHovered(false)}
      style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 16px', borderBottom: isLast ? 'none' : '1px solid var(--c-border-muted)', cursor: 'pointer', background: hovered ? 'var(--c-bg-overlay)' : 'transparent', transition: 'background 0.1s' }}>
      {icon}
      <span style={{ flex: 1, fontSize: 13, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', ...nameStyle }}>{name}</span>
      <span style={{ fontSize: 10, fontWeight: 500, color: isDir ? '#e3a14f' : 'var(--c-fg-subtle)', background: isDir ? 'rgba(227,161,79,0.1)' : 'var(--c-bg-overlay)', padding: '1px 6px', borderRadius: 4, flexShrink: 0, textTransform: 'uppercase', letterSpacing: '0.04em' }}>{isDir ? 'dir' : 'file'}</span>
      {oid && <code style={{ fontSize: 11, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg-subtle)', background: 'var(--c-bg-overlay)', padding: '1px 6px', borderRadius: 4, flexShrink: 0 }}>{oid.slice(0, 7)}</code>}
    </div>
  );
};

function ModuleDetailContent() {
  const { owner = '', module: moduleName = '' } = useParams<{ owner: string; module: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();

  const activeTab = searchParams.get('tab') ?? 'overview';
  const dirPath = searchParams.get('dir') ?? '';
  const openFilePath = searchParams.get('file') ?? null;
  const openFile: FileEntry | null = openFilePath
    ? { name: openFilePath.split('/').pop() ?? openFilePath, path: openFilePath, type: 1, oid: '', mode: 0 }
    : null;

  const [mod, setMod] = useState<Module | null>(null);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [sdks, setSdks] = useState<SDK[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [settingsDesc, setSettingsDesc] = useState('');
  const [settingsPublic, setSettingsPublic] = useState(false);
  const [settingsSaved, setSettingsSaved] = useState(false);
  const [dirEntries, setDirEntries] = useState<FileEntry[]>([]);
  const [dirLoading, setDirLoading] = useState(false);
  const [dirError, setDirError] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const updateParams = (updates: Record<string, string | null>, replace = false) => {
    const params = new URLSearchParams(searchParams.toString());
    for (const [k, v] of Object.entries(updates)) {
      if (v) params.set(k, v); else params.delete(k);
    }
    const url = `/${owner}/${moduleName}?${params.toString()}`;
    if (replace) router.replace(url); else router.push(url);
  };

  const setTab = (tab: string) => updateParams({ tab, ...(tab !== 'files' ? { dir: null, file: null } : {}) });
  const navigateToDir = (path: string) => { setDirEntries([]); updateParams({ tab: 'files', dir: path || null, file: null }); };
  const openFileView = (entry: FileEntry) => { setFileContent(null); setFileError(null); updateParams({ tab: 'files', file: entry.path, dir: dirPath || null }); };

  useEffect(() => {
    if (!owner || !moduleName) return;
    setLoading(true); setError(null);
    rpcFetch<{ module: Module }>('/hades.api.registry.v1.ModuleService/GetModule', { owner, name: moduleName })
      .then(modRes => {
        const m = modRes.module;
        setMod(m); setSettingsDesc(m.description || ''); setSettingsPublic(isPublic(m.visibility));
        addRecentModule({ owner, name: moduleName, fullName: `${owner}/${moduleName}`, visibility: isPublic(m.visibility) ? 'public' : 'private' });
        Promise.allSettled([
          rpcFetch<{ commits: Commit[] }>('/hades.api.registry.v1.CommitService/ListCommits', { owner, module: moduleName }),
          rpcFetch<{ sdkJobs: SDK[] }>('/hades.api.registry.v1.SDKService/ListSDKs', { owner, module: moduleName }),
        ]).then(([commitResult, sdkResult]) => {
          if (commitResult.status === 'fulfilled') setCommits(commitResult.value.commits || []);
          if (sdkResult.status === 'fulfilled') setSdks(sdkResult.value.sdkJobs || []);
        });
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [owner, moduleName]);

  useEffect(() => {
    if (activeTab !== 'files' || openFilePath || !owner || !moduleName) return;
    setDirLoading(true); setDirError(null);
    rpcFetch<{ entries: FileEntry[] }>('/hades.api.registry.v1.TreeService/ListModuleFiles', { owner, module: moduleName, path: dirPath })
      .then(res => setDirEntries(res.entries || []))
      .catch(e => setDirError(e.message))
      .finally(() => setDirLoading(false));
  }, [activeTab, owner, moduleName, dirPath, openFilePath]);

  useEffect(() => {
    if (!openFilePath || !owner || !moduleName) return;
    setFileLoading(true); setFileError(null); setFileContent(null);
    rpcFetch<{ content: string }>('/hades.api.registry.v1.TreeService/GetFileContent', { owner, module: moduleName, path: openFilePath })
      .then(res => setFileContent(atob(res.content || '')))
      .catch(e => setFileError(e.message))
      .finally(() => setFileLoading(false));
  }, [owner, moduleName, openFilePath]);

  if (loading) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-loading">Loading module…</div></div>;
  if (error || !mod) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-error">{error || 'Module not found'}</div></div>;

  const pub = isPublic(mod.visibility);
  const latestCommit = commits[0] || null;
  const bufYaml = `version: v2\nmodules:\n  - path: .\n    name: ${DOMAIN}/${owner}/${moduleName}\nlint:\n  use:\n    - DEFAULT\nbreaking:\n  use:\n    - FILE`;
  const dirSegments = dirPath ? dirPath.split('/').filter(Boolean) : [];
  const sortedEntries = [...dirEntries].sort((a, b) => { const aD = isEntryDir(a.type); const bD = isEntryDir(b.type); if (aD !== bD) return aD ? -1 : 1; return a.name.localeCompare(b.name); });

  const renderBreadcrumb = (fileEntry?: FileEntry | null) => {
    const allSegments = fileEntry ? fileEntry.path.split('/').filter(Boolean) : dirSegments;
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap', fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 14 }}>
        <span role="button" style={{ color: 'var(--c-accent)', cursor: 'pointer', fontFamily: "'IBM Plex Mono', monospace" }} onClick={() => navigateToDir('')}>{owner}/{moduleName}</span>
        {allSegments.map((seg, i) => {
          const isLast = i === allSegments.length - 1;
          const segPath = allSegments.slice(0, i + 1).join('/');
          const isFilename = fileEntry && isLast;
          return (
            <React.Fragment key={segPath}>
              <IconChevronRight size={12} style={{ opacity: 0.4, flexShrink: 0 }}/>
              <span role={!isFilename ? 'button' : undefined} style={{ fontFamily: "'IBM Plex Mono', monospace", color: isLast ? 'var(--c-fg)' : 'var(--c-accent)', cursor: isFilename ? 'default' : 'pointer', fontWeight: isLast ? 600 : 400 }} onClick={() => { if (!isFilename) navigateToDir(segPath); }}>{seg}</span>
            </React.Fragment>
          );
        })}
      </div>
    );
  };

  const renderFiles = () => {
    if (openFile) {
      return (
        <div style={{ padding: '20px 32px' }}>
          {renderBreadcrumb(openFile)}
          {fileLoading && <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, padding: '40px 20px', textAlign: 'center', color: 'var(--c-fg-muted)', fontSize: 13 }}>Loading…</div>}
          {fileError && <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, padding: '40px 20px', textAlign: 'center', color: 'var(--c-danger)', fontSize: 13 }}>{fileError}</div>}
          {!fileLoading && !fileError && fileContent !== null && <FileViewer filename={openFile.name} content={fileContent} oid={openFile.oid}/>}
        </div>
      );
    }
    const parentPath = dirSegments.slice(0, -1).join('/');
    return (
      <div style={{ padding: '20px 32px' }}>
        {renderBreadcrumb(null)}
        <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden', background: 'var(--c-bg-default)' }}>
          {dirPath && <FileRow icon={<IconFolder size={14} style={{ color: 'var(--c-fg-subtle)' }}/>} name=".." oid="" isDir isLast={!dirLoading && sortedEntries.length === 0} onClick={() => navigateToDir(parentPath)}/>}
          {dirLoading && <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--c-fg-muted)', fontSize: 13 }}>Loading…</div>}
          {dirError && <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--c-danger)', fontSize: 13 }}>{dirError}</div>}
          {!dirLoading && !dirError && sortedEntries.length === 0 && !dirPath && <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--c-fg-muted)', fontSize: 13 }}>No files found. Push your first commit to see files here.</div>}
          {!dirLoading && !dirError && sortedEntries.map((entry, i) => {
            const dir = isEntryDir(entry.type);
            return <FileRow key={entry.path} icon={dir ? <IconFolder size={14} style={{ color: '#e3a14f' }}/> : <IconFile size={14} style={{ color: 'var(--c-fg-subtle)' }}/>} name={entry.name} oid={entry.oid} isDir={dir} isLast={i === sortedEntries.length - 1} nameStyle={{ color: dir ? 'var(--c-accent)' : 'var(--c-fg)' }} onClick={() => dir ? navigateToDir(entry.path) : openFileView(entry)}/>;
          })}
        </div>
      </div>
    );
  };

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader
        breadcrumb={[
          { label: 'Registry', onClick: () => router.push('/') },
          { label: owner, onClick: () => router.push(`/${owner}`) },
          { label: moduleName },
        ]}
        title={<span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 18 }}>{owner}/{moduleName}</span>}
        subtitle={mod.description || undefined}
        actions={<>
          <Btn size="sm" icon={<IconStar size={13}/>}>Star</Btn>
          <Btn size="sm" icon={<IconDownload size={13}/>}>Clone</Btn>
          <Btn size="sm" variant="primary" icon={<IconCode size={13}/>} onClick={() => router.push(`/${owner}/${moduleName}/sdks`)}>Get SDKs</Btn>
        </>}
      />

      <div style={{ padding: '0 32px' }}>
        <Tabs tabs={MODULE_TABS.map(t => ({ ...t, count: t.id === 'commits' ? commits.length : t.id === 'sdks' ? sdks.length : undefined }))} active={activeTab} onChange={setTab}/>
      </div>

      {activeTab === 'overview' && (
        <div style={{ padding: '24px 32px', display: 'flex', gap: 24 }}>
          <div style={{ flex: 1 }}>
            {latestCommit && (
              <Card style={{ padding: 20, marginBottom: 16 }}>
                <h3 style={{ margin: '0 0 12px', fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>Latest Commit</h3>
                <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
                  <Avatar initials={owner.slice(0, 2)} size={28}/>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 13, color: 'var(--c-fg)', marginBottom: 4, fontWeight: 600, fontFamily: "'IBM Plex Mono', monospace", overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{latestCommit.commitHash}</div>
                    <div style={{ fontSize: 12, color: 'var(--c-fg-subtle)', display: 'flex', gap: 12 }}>
                      <span style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)', cursor: 'pointer' }} onClick={() => router.push(`/${owner}/${moduleName}/commit/${latestCommit.commitHash}`)}>{latestCommit.commitHash.slice(0, 7)}</span>
                      <span>{latestCommit.ownerId || owner}</span>
                      <span>{fmtDate(latestCommit.createTime)}</span>
                    </div>
                  </div>
                </div>
              </Card>
            )}
            <Card style={{ padding: 20 }}>
              <h3 style={{ margin: '0 0 14px', fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>buf.yaml</h3>
              <CodeBlock lang="yaml" code={bufYaml}/>
            </Card>
          </div>
          <div style={{ width: 220, flexShrink: 0 }}>
            <Card style={{ padding: 20, marginBottom: 12 }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <Stat label="Visibility" value={pub ? 'Public' : 'Private'} icon={pub ? <IconGlobe size={14}/> : <IconLock size={14}/>}/>
                <Divider/>
                <Stat label="Commits" value={String(commits.length)} icon={<IconGitCommit size={14}/>}/>
                <Divider/>
                <Stat label="SDKs" value={String(sdks.length)} icon={<IconPackage size={14}/>}/>
              </div>
            </Card>
            <Card style={{ padding: 16 }}>
              {mod.defaultBranch && (<><div style={{ fontSize: 12, color: 'var(--c-fg-subtle)', marginBottom: 4 }}>Branch</div><div style={{ fontSize: 13, color: 'var(--c-fg)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 6 }}><IconBranch size={12}/>{mod.defaultBranch}</div></>)}
              {mod.createTime && (<><div style={{ fontSize: 12, color: 'var(--c-fg-subtle)', marginBottom: 4 }}>Created</div><div style={{ fontSize: 13, color: 'var(--c-fg)', display: 'flex', alignItems: 'center', gap: 6 }}><IconClock size={12}/>{fmtDate(mod.createTime)}</div></>)}
            </Card>
          </div>
        </div>
      )}

      {activeTab === 'files' && renderFiles()}

      {activeTab === 'commits' && (
        <Section>
          {commits.length === 0 ? <EmptyState icon={<IconGitCommit size={40}/>} title="No commits yet" subtitle="Push your first Protobuf files to create a commit."/> : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 1, border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden' }}>
              {commits.map((c, i) => (
                <div key={c.id} style={{ background: 'var(--c-bg-default)', borderBottom: i < commits.length - 1 ? '1px solid var(--c-border-muted)' : 'none' }}>
                  <div style={{ padding: '14px 18px', display: 'flex', gap: 12, alignItems: 'flex-start', cursor: 'pointer' }} onClick={() => router.push(`/${owner}/${moduleName}/commit/${c.commitHash}`)} onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'} onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}>
                    <Avatar initials={owner.slice(0, 2)} size={28}/>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 4, fontFamily: "'IBM Plex Mono', monospace" }}>{c.commitHash.slice(0, 32)}{c.commitHash.length > 32 ? '…' : ''}</div>
                      <div style={{ fontSize: 12, color: 'var(--c-fg-subtle)', display: 'flex', gap: 12 }}><span>{c.ownerId || owner}</span><span>committed {fmtDate(c.createTime)}</span></div>
                    </div>
                    <code style={{ fontSize: 12, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)', background: 'var(--c-accent-bg)', padding: '2px 8px', borderRadius: 4, flexShrink: 0 }}>{c.commitHash.slice(0, 7)}</code>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Section>
      )}

      {activeTab === 'versions' && (
        <Section>
          {commits.length === 0 ? <EmptyState icon={<IconTag size={40}/>} title="No versions yet"/> : (
            <Table columns={[
              { key: 'version', label: 'Version', render: v => <span style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)', fontSize: 13 }}>{String(v)}</span> },
              { key: 'hash', label: 'Commit', render: v => <code style={{ fontSize: 12, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg-muted)' }}>{String(v)}</code> },
              { key: 'date', label: 'Date' },
            ]} rows={commits.map((c, i) => ({ version: `v${commits.length - i}`, hash: c.commitHash.slice(0, 12), date: fmtDate(c.createTime), _commit: c }))} onRowClick={row => router.push(`/${owner}/${moduleName}/commit/${(row as { _commit: Commit })._commit.commitHash}`)}/>
          )}
        </Section>
      )}

      {activeTab === 'sdks' && (
        <Section>
          {sdks.length === 0 ? <EmptyState icon={<IconCode size={40}/>} title="No SDKs generated" subtitle="SDK generation runs automatically when you push commits."/> : (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 12 }}>
              {sdks.map(sdk => (
                <Card key={sdk.id} style={{ padding: 18 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
                    <span style={{ fontSize: 22 }}>{getLangEmoji(sdk.language)}</span>
                    <div><div style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)', textTransform: 'capitalize' }}>{sdk.language}</div>{sdk.status && <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)' }}>{sdk.status}</div>}</div>
                    <Btn size="sm" style={{ marginLeft: 'auto' }} icon={<IconDownload size={12}/>}>Install</Btn>
                  </div>
                  <CodeBlock code={getInstallCmd(sdk.language, owner, moduleName)} style={{ marginBottom: 0 }}/>
                </Card>
              ))}
            </div>
          )}
        </Section>
      )}

      {activeTab === 'dependencies' && <Section><EmptyState icon={<IconBox size={40}/>} title="No dependencies" subtitle="This module has no external dependencies declared."/></Section>}
      {activeTab === 'ci' && <Section><EmptyState icon={<IconCode size={40}/>} title="CI / Lint" subtitle="Connect your repository to enable lint checks and breaking change detection."/></Section>}

      {activeTab === 'settings' && (
        <Section>
          <div style={{ maxWidth: 500, display: 'flex', flexDirection: 'column', gap: 20 }}>
            <div>
              <label style={{ display: 'block', fontSize: 13, fontWeight: 500, color: 'var(--c-fg)', marginBottom: 6 }}>Description</label>
              <Input value={settingsDesc} onChange={val => { setSettingsDesc(val); setSettingsSaved(false); }} placeholder="Short description of this module"/>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div><div style={{ fontSize: 13, fontWeight: 500, color: 'var(--c-fg)' }}>Public visibility</div><div style={{ fontSize: 12, color: 'var(--c-fg-muted)', marginTop: 2 }}>Anyone can view this module if enabled</div></div>
              <Toggle checked={settingsPublic} onChange={v => { setSettingsPublic(v); setSettingsSaved(false); }}/>
            </div>
            <div>
              <Btn variant="primary" onClick={() => setSettingsSaved(true)}>Save changes</Btn>
              {settingsSaved && <span style={{ marginLeft: 10, fontSize: 12, color: 'var(--c-success)' }}>Saved</span>}
            </div>
          </div>
        </Section>
      )}
    </div>
  );
}

export default function ModuleDetailPage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <ModuleDetailContent/>
    </Suspense>
  );
}
