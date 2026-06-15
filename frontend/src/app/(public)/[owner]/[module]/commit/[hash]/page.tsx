'use client';
import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Avatar from '@/components/ui/Avatar';
import Card from '@/components/ui/Card';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import Divider from '@/components/ui/Divider';
import { IconGitCommit, IconClock } from '@/components/icons';
import { rpcFetch } from '@/lib/rpc';

interface Commit { id: string; commitHash: string; createTime?: string; ownerId?: string; moduleId?: string; fileCount?: number; additions?: number; deletions?: number; }
interface FileDiff { fromPath: string; toPath: string; isNewFile: boolean; isDeletedFile: boolean; isRenamedFile: boolean; additions: number; deletions: number; patch: string; binary: boolean; tooLarge: boolean; }
interface DiffResult { diffs: FileDiff[]; totalAdditions: number; totalDeletions: number; }

function fileBadge(fd: FileDiff): { label: string; color: string } {
  if (fd.isNewFile) return { label: 'New', color: 'var(--c-success)' };
  if (fd.isDeletedFile) return { label: 'Deleted', color: 'var(--c-danger)' };
  if (fd.isRenamedFile) return { label: 'Renamed', color: 'var(--c-accent)' };
  return { label: 'Modified', color: 'var(--c-fg-muted)' };
}

function lineStyle(line: string): React.CSSProperties {
  if (line.startsWith('+++') || line.startsWith('---')) return { color: 'var(--c-fg-muted)' };
  if (line.startsWith('+')) return { background: 'rgba(63,185,80,0.12)', color: 'var(--c-success)' };
  if (line.startsWith('-')) return { background: 'rgba(248,81,73,0.12)', color: 'var(--c-danger)' };
  if (line.startsWith('@@')) return { color: 'var(--c-accent)', background: 'var(--c-accent-bg)' };
  return { color: 'var(--c-fg-muted)' };
}

const FileDiffCard: React.FC<{ fd: FileDiff }> = ({ fd }) => {
  const [collapsed, setCollapsed] = useState(false);
  const badge = fileBadge(fd);
  const displayPath = fd.isDeletedFile ? fd.fromPath : (fd.toPath || fd.fromPath);
  return (
    <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden', marginBottom: 12 }}>
      <div onClick={() => setCollapsed(c => !c)} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 14px', background: 'var(--c-bg-subtle)', cursor: 'pointer', userSelect: 'none' }}>
        <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, color: 'var(--c-fg)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {displayPath}
          {fd.isRenamedFile && fd.fromPath !== fd.toPath && <span style={{ color: 'var(--c-fg-subtle)', marginLeft: 6 }}>← {fd.fromPath}</span>}
        </span>
        <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 7px', borderRadius: 4, background: badge.color + '22', color: badge.color, border: `1px solid ${badge.color}44` }}>{badge.label}</span>
        {fd.additions > 0 && <span style={{ fontSize: 12, color: 'var(--c-success)', fontFamily: 'monospace' }}>+{fd.additions}</span>}
        {fd.deletions > 0 && <span style={{ fontSize: 12, color: 'var(--c-danger)', fontFamily: 'monospace' }}>-{fd.deletions}</span>}
        <span style={{ fontSize: 11, color: 'var(--c-fg-subtle)' }}>{collapsed ? '▶' : '▼'}</span>
      </div>
      {!collapsed && (
        <div>
          {fd.binary
            ? <div style={{ padding: '12px 14px', fontSize: 13, color: 'var(--c-fg-subtle)', fontStyle: 'italic' }}>Binary file not shown</div>
            : fd.tooLarge
              ? <div style={{ padding: '12px 14px', fontSize: 13, color: 'var(--c-fg-subtle)', fontStyle: 'italic' }}>File too large to display</div>
              : fd.patch ? (
                <pre style={{ margin: 0, padding: '10px 0', overflowX: 'auto', fontSize: 12, lineHeight: 1.55, fontFamily: "'IBM Plex Mono', Menlo, monospace" }}>
                  {fd.patch.split('\n').map((line, i) => <div key={i} style={{ padding: '0 14px', ...lineStyle(line) }}>{line || ' '}</div>)}
                </pre>
              ) : <div style={{ padding: '12px 14px', fontSize: 13, color: 'var(--c-fg-subtle)', fontStyle: 'italic' }}>No content changes</div>}
        </div>
      )}
    </div>
  );
};

function CommitDetailContent() {
  const { owner = '', module: moduleName = '', hash = '' } = useParams<{ owner: string; module: string; hash: string }>();
  const router = useRouter();
  const [commit, setCommit] = useState<Commit | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [diffLoading, setDiffLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [diffError, setDiffError] = useState<string | null>(null);

  useEffect(() => {
    if (!hash) return;
    setLoading(true); setError(null);
    rpcFetch<{ commit: Commit }>('/hades.api.registry.v1.CommitService/GetCommit', { commitHash: hash })
      .then(res => setCommit(res.commit))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [hash]);

  useEffect(() => {
    if (!hash) return;
    setDiffLoading(true); setDiffError(null);
    rpcFetch<DiffResult>('/hades.api.registry.v1.DiffService/GetCommitDiff', { commitHash: hash })
      .then(res => setDiff(res))
      .catch(e => setDiffError(e.message))
      .finally(() => setDiffLoading(false));
  }, [hash]);

  if (loading) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-loading">Loading commit…</div></div>;
  if (error || !commit) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-error">{error || 'Commit not found'}</div></div>;

  const shortHash = commit.commitHash.slice(0, 8);
  const authorInitials = (commit.ownerId || owner || 'U').slice(0, 2).toUpperCase();
  const formattedDate = commit.createTime ? new Date(commit.createTime).toLocaleString() : null;
  const fileCount = diff?.diffs.length ?? commit.fileCount;
  const totalAdditions = diff?.totalAdditions ?? commit.additions;
  const totalDeletions = diff?.totalDeletions ?? commit.deletions;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <PageHeader
        breadcrumb={[
          { label: 'Registry', onClick: () => router.push('/') },
          { label: `${owner}/${moduleName}`, onClick: () => router.push(`/${owner}/${moduleName}`) },
          { label: 'Commits', onClick: () => router.push(`/${owner}/${moduleName}`) },
          { label: shortHash },
        ]}
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <IconGitCommit size={18} style={{ color: 'var(--c-fg-muted)' }}/>
            <span style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 15 }}>{commit.commitHash.slice(0, 16)}…</span>
          </div>
        }
      />

      <div style={{ flex: 1, overflowY: 'auto' }}>
        <Section>
          <Card style={{ padding: '20px 24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
              <Avatar initials={authorInitials} size={40}/>
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)' }}>{commit.ownerId || owner}</div>
                {formattedDate && <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: 'var(--c-fg-subtle)', marginTop: 2 }}><IconClock size={12}/>{formattedDate}</div>}
              </div>
            </div>
            <Divider/>
            <div style={{ marginTop: 14, marginBottom: 14 }}>
              <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--c-fg-subtle)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: 0.5 }}>Commit Hash</div>
              <div style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 13, color: 'var(--c-accent)', background: 'var(--c-accent-bg)', border: '1px solid var(--c-accent-muted)', borderRadius: 6, padding: '8px 12px', display: 'inline-block', letterSpacing: 0.5 }}>{commit.commitHash}</div>
            </div>
            <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap' }}>
              {fileCount != null && <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}><span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5 }}>Files</span><span style={{ fontSize: 18, fontWeight: 600, color: 'var(--c-fg)' }}>{fileCount}</span></div>}
              {totalAdditions != null && <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}><span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5 }}>Additions</span><span style={{ fontSize: 18, fontWeight: 600, color: 'var(--c-success)' }}>+{totalAdditions}</span></div>}
              {totalDeletions != null && <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}><span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5 }}>Deletions</span><span style={{ fontSize: 18, fontWeight: 600, color: 'var(--c-danger)' }}>-{totalDeletions}</span></div>}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}><span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5 }}>Module</span><span style={{ fontSize: 13, fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)', fontWeight: 500 }}>{owner}/{moduleName}</span></div>
            </div>
          </Card>
        </Section>

        <Section title="Diff">
          {diffLoading ? <div style={{ padding: '24px 0', textAlign: 'center' }} className="status-loading">Loading diff…</div>
          : diffError ? <Card style={{ padding: '20px 24px' }}><div style={{ color: 'var(--c-danger)', fontSize: 13 }}>{diffError}</div></Card>
          : !diff || diff.diffs.length === 0 ? <Card style={{ padding: '32px 24px', textAlign: 'center' }}><div style={{ color: 'var(--c-fg-subtle)', fontSize: 13 }}>No file changes in this commit.</div></Card>
          : (
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 14, padding: '8px 14px', background: 'var(--c-bg-subtle)', borderRadius: 8, border: '1px solid var(--c-border)', fontSize: 13 }}>
                <span style={{ color: 'var(--c-fg-muted)' }}>{diff.diffs.length} file{diff.diffs.length !== 1 ? 's' : ''} changed</span>
                {diff.totalAdditions > 0 && <span style={{ color: 'var(--c-success)', fontFamily: 'monospace', fontWeight: 600 }}>+{diff.totalAdditions}</span>}
                {diff.totalDeletions > 0 && <span style={{ color: 'var(--c-danger)', fontFamily: 'monospace', fontWeight: 600 }}>-{diff.totalDeletions}</span>}
              </div>
              {diff.diffs.map((fd, i) => <FileDiffCard key={i} fd={fd}/>)}
            </div>
          )}
        </Section>
      </div>
    </div>
  );
}

export default function CommitDetailPage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <CommitDetailContent/>
    </Suspense>
  );
}
