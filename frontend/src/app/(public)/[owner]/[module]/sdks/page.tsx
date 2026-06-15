'use client';
import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Badge from '@/components/ui/Badge';
import Card from '@/components/ui/Card';
import CodeBlock from '@/components/ui/CodeBlock';
import EmptyState from '@/components/ui/EmptyState';
import PageHeader from '@/components/ui/PageHeader';
import Btn from '@/components/ui/Button';
import { IconCode, IconDownload } from '@/components/icons';
import { DOMAIN } from '@/lib/config';
import { rpcFetch } from '@/lib/rpc';

interface SDK { id: string; moduleId: string; commitId?: string; language: string; plugin?: string; status?: string; outputLocation?: string; }

const LANG_EMOJIS: Record<string, string> = { go: '🐹', typescript: '🔷', python: '🐍', java: '☕', rust: '🦀', swift: '🦅' };
function getLangEmoji(lang: string): string { return LANG_EMOJIS[lang.toLowerCase()] ?? '📦'; }

function getInstallCmd(lang: string, owner: string, mod: string): string {
  switch (lang.toLowerCase()) {
    case 'go': return `GOPROXY=https://${DOMAIN}/go,off GONOSUMDB=* \\\n  go get ${DOMAIN}/gen/go/${owner}/${mod}@latest`;
    case 'typescript': return `npm install @buf/${owner}_${mod}`;
    case 'python': return `pip install buf-${owner}-${mod}`;
    case 'java': return `// In build.gradle:\nimplementation "build.buf.gen:${owner}_${mod}:0.1.0"`;
    case 'rust': return `# In Cargo.toml:\nbuf-${owner}-${mod} = "0.1"`;
    case 'swift': return `.package(url: "https://${DOMAIN}/gen/go/${owner}/${mod}", from: "1.0.0")`;
    default: return `# Install ${lang} SDK for ${owner}/${mod}`;
  }
}

function getUsageCode(lang: string, owner: string, mod: string): string {
  switch (lang.toLowerCase()) {
    case 'go': return `import (\n  pb "${DOMAIN}/gen/go/${owner}/${mod}/proto"\n)\n\n// Use generated types\nmsg := &pb.MyRequest{}`;
    case 'typescript': return `import { MyRequest } from "@buf/${owner}_${mod}";\n\nconst req = new MyRequest();`;
    case 'python': return `from buf_${owner}_${mod} import my_pb2\n\nreq = my_pb2.MyRequest()`;
    default: return `// See documentation for ${lang} usage examples`;
  }
}

function SDKContent() {
  const { owner = '', module: moduleName = '' } = useParams<{ owner: string; module: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [sdks, setSdks] = useState<SDK[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const selectedLang = searchParams.get('lang');

  const setLang = (lang: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set('lang', lang);
    router.push(`/${owner}/${moduleName}/sdks?${params.toString()}`);
  };

  useEffect(() => {
    if (!owner || !moduleName) return;
    setLoading(true); setError(null);
    rpcFetch<{ sdkJobs: SDK[] }>('/hades.api.registry.v1.SDKService/ListSDKs', { owner, module: moduleName })
      .then(res => {
        const jobs = res.sdkJobs || [];
        setSdks(jobs);
        if (jobs.length > 0 && !searchParams.get('lang')) setLang(jobs[0].language);
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner, moduleName]);

  const languages = Array.from(new Set(sdks.map(s => s.language)));
  const activeSdk = sdks.find(s => s.language === selectedLang) || sdks[0] || null;

  if (loading) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-loading">Loading SDKs…</div></div>;
  if (error) return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 300 }}><div className="status-error">{error}</div></div>;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <PageHeader
        breadcrumb={[
          { label: 'Registry', onClick: () => router.push('/') },
          { label: `${owner}/${moduleName}`, onClick: () => router.push(`/${owner}/${moduleName}`) },
          { label: 'SDKs' },
        ]}
        title="Generated SDKs"
        subtitle={`Client libraries generated from ${owner}/${moduleName}`}
      />

      {sdks.length === 0 ? (
        <EmptyState icon={<IconCode size={40}/>} title="No SDKs generated" subtitle="SDK generation runs automatically when you push new commits."/>
      ) : (
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
          <div style={{ width: 180, flexShrink: 0, borderRight: '1px solid var(--c-border)', overflowY: 'auto', padding: '16px 0' }}>
            <div style={{ padding: '0 12px 8px', fontSize: 11, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.5 }}>Languages</div>
            {languages.map(lang => {
              const isActive = lang === selectedLang;
              return (
                <button key={lang} onClick={() => setLang(lang)} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 10, padding: '8px 14px', background: isActive ? 'var(--c-accent-bg)' : 'transparent', border: 'none', borderLeft: `2px solid ${isActive ? 'var(--c-accent)' : 'transparent'}`, cursor: 'pointer', color: isActive ? 'var(--c-accent)' : 'var(--c-fg-muted)', fontSize: 13, fontWeight: isActive ? 600 : 400, fontFamily: 'inherit', textAlign: 'left', transition: 'all 0.1s' }}>
                  <span style={{ fontSize: 18, lineHeight: 1 }}>{getLangEmoji(lang)}</span>
                  <span style={{ textTransform: 'capitalize' }}>{lang}</span>
                </button>
              );
            })}
          </div>

          <div style={{ flex: 1, overflowY: 'auto', padding: '24px 32px' }}>
            {activeSdk && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24, flexWrap: 'wrap', gap: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    <span style={{ fontSize: 36 }}>{getLangEmoji(activeSdk.language)}</span>
                    <div>
                      <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600, color: 'var(--c-fg)', textTransform: 'capitalize' }}>{activeSdk.language} SDK</h2>
                      <div style={{ display: 'flex', gap: 8, marginTop: 4 }}>
                        {activeSdk.status && <Badge variant={activeSdk.status === 'success' ? 'green' : activeSdk.status === 'error' ? 'red' : 'yellow'}>{activeSdk.status}</Badge>}
                        {activeSdk.plugin && <Badge variant="default">{activeSdk.plugin}</Badge>}
                      </div>
                    </div>
                  </div>
                  {activeSdk.outputLocation && <Btn variant="primary" icon={<IconDownload size={14}/>} onClick={() => window.open(activeSdk.outputLocation, '_blank')}>Download</Btn>}
                </div>
                <div style={{ marginBottom: 24 }}>
                  <h3 style={{ margin: '0 0 10px', fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>Installation</h3>
                  <CodeBlock code={getInstallCmd(activeSdk.language, owner, moduleName)} lang={activeSdk.language.toLowerCase() === 'typescript' ? 'bash' : activeSdk.language.toLowerCase()}/>
                </div>
                <div style={{ marginBottom: 24 }}>
                  <h3 style={{ margin: '0 0 10px', fontSize: 13, fontWeight: 600, color: 'var(--c-fg)' }}>Usage</h3>
                  <CodeBlock code={getUsageCode(activeSdk.language, owner, moduleName)} lang={activeSdk.language.toLowerCase()}/>
                </div>
                {activeSdk.commitId && (
                  <Card style={{ padding: '12px 16px' }}>
                    <div style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 12, color: 'var(--c-fg-muted)' }}>
                      <span>Generated from commit</span>
                      <span style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-accent)' }}>{activeSdk.commitId.slice(0, 12)}</span>
                    </div>
                  </Card>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

export default function SDKPage() {
  return (
    <Suspense fallback={<div style={{ padding: 40, color: 'var(--c-fg-muted)' }}>Loading…</div>}>
      <SDKContent/>
    </Suspense>
  );
}
