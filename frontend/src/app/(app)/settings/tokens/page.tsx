'use client';
import React, { useEffect, useState } from 'react';
import PageHeader from '@/components/ui/PageHeader';
import Section from '@/components/ui/Section';
import Card from '@/components/ui/Card';
import Btn from '@/components/ui/Button';
import Input from '@/components/ui/Input';
import Badge from '@/components/ui/Badge';
import { IconKey, IconPlus, IconX, IconCheck, IconAlert } from '@/components/icons';
import { formatError } from '@/lib/connectError';
import { rpcFetch } from '@/lib/rpc';

interface APIToken { id: string; name: string; prefix: string; scopes: string[]; last_used_at?: string; expires_at?: string; created_at?: string; }

export default function PageTokens() {
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showNew, setShowNew] = useState(false);
  const [newName, setNewName] = useState('');
  const [newScopes, setNewScopes] = useState('');
  const [creating, setCreating] = useState(false);
  const [newTokenValue, setNewTokenValue] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);

  const loadTokens = async () => {
    try {
      const data = await rpcFetch<{ tokens: APIToken[] }>('/hades.api.authentication.v1.APITokenService/ListAPITokens', {});
      setTokens(data.tokens || []);
    } catch (e) { setError(formatError(e)); } finally { setLoading(false); }
  };

  useEffect(() => { loadTokens(); }, []);

  const createToken = async () => {
    if (!newName) return;
    setCreating(true);
    try {
      const scopes = newScopes ? newScopes.split(',').map(s => s.trim()).filter(Boolean) : [];
      const data = await rpcFetch<{ id: string; token: string; prefix: string; created_at: string }>('/hades.api.authentication.v1.APITokenService/CreateAPIToken', { name: newName, scopes });
      setNewTokenValue(data.token); setShowNew(false); setNewName(''); setNewScopes('');
      await loadTokens();
    } catch (e) { setError(formatError(e)); } finally { setCreating(false); }
  };

  const revokeToken = async (id: string) => {
    setRevoking(id);
    try {
      await rpcFetch('/hades.api.authentication.v1.APITokenService/RevokeAPIToken', { id });
      setTokens(t => t.filter(tok => tok.id !== id));
    } catch (e) { setError(formatError(e)); } finally { setRevoking(null); }
  };

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <PageHeader title="API Tokens" subtitle="Create personal access tokens for use with the Hades CLI and API." actions={<Btn variant="primary" icon={<IconPlus size={13}/>} onClick={() => setShowNew(v => !v)}>New Token</Btn>}/>
      <Section>
        {error && <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8 }}><IconAlert size={14}/>{error}</div>}

        {newTokenValue && (
          <Card style={{ padding: '16px 20px', marginBottom: 16, background: 'var(--c-success-bg)', borderColor: 'var(--c-success)' }}>

            <code style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 12, background: 'var(--c-bg-inset)', padding: '8px 12px', borderRadius: 6, display: 'block', wordBreak: 'break-all', color: 'var(--c-fg)' }}>{newTokenValue}</code>
            <Btn size="sm" variant="ghost" style={{ marginTop: 10 }} onClick={() => setNewTokenValue(null)}><IconX size={12}/>Dismiss</Btn>
          </Card>
        )}

        {showNew && (
          <Card style={{ padding: '20px', marginBottom: 16 }}>
            <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--c-fg)', marginBottom: 16 }}>Create new token</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div><label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Token name</label><Input value={newName} onChange={setNewName} placeholder="e.g. ci-token"/></div>
              <div><label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Scopes (comma-separated, optional)</label><Input value={newScopes} onChange={setNewScopes} placeholder="module:read, module:write"/></div>
              <div style={{ display: 'flex', gap: 8 }}>
                <Btn variant="primary" onClick={createToken} disabled={creating || !newName}>{creating ? 'Creating…' : 'Create Token'}</Btn>
                <Btn variant="ghost" onClick={() => { setShowNew(false); setNewName(''); setNewScopes(''); }}>Cancel</Btn>
              </div>
            </div>
          </Card>
        )}

        {loading ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--c-fg-muted)', fontSize: 13 }}>Loading tokens…</div>
        ) : tokens.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--c-fg-muted)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
            <IconKey size={28} style={{ opacity: 0.4 }}/><div style={{ fontSize: 14, fontWeight: 500 }}>No API tokens</div><div style={{ fontSize: 13, color: 'var(--c-fg-subtle)' }}>Create a token to use with the CLI or API.</div>
          </div>
        ) : (
          <div style={{ border: '1px solid var(--c-border)', borderRadius: 8, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--c-border)', background: 'var(--c-bg-overlay)' }}>
                  {['Name', 'Token', 'Scopes', 'Last Used', 'Expires'].map(h => <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 11, fontWeight: 600, color: 'var(--c-fg-muted)', letterSpacing: 0.5, textTransform: 'uppercase' }}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {tokens.map((tok, i) => (
                  <tr key={tok.id} style={{ borderBottom: i < tokens.length - 1 ? '1px solid var(--c-border-muted)' : 'none' }}>
                    <td style={{ padding: '12px 16px', fontSize: 13, color: 'var(--c-fg)', fontWeight: 500 }}>{tok.name}</td>
                    <td style={{ padding: '12px 16px' }}><code style={{ fontFamily: "'IBM Plex Mono', monospace", fontSize: 12, color: 'var(--c-accent)', background: 'var(--c-accent-bg)', padding: '2px 8px', borderRadius: 4 }}>{tok.prefix}</code></td>
                    <td style={{ padding: '12px 16px' }}><div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>{(tok.scopes || []).map(s => <Badge key={s} variant="default">{s}</Badge>)}{(tok.scopes || []).length === 0 && <span style={{ fontSize: 12, color: 'var(--c-fg-subtle)' }}>full access</span>}</div></td>

                    <td style={{ padding: '12px 16px', fontSize: 12, color: 'var(--c-fg-subtle)' }}>{tok.expires_at ? new Date(tok.expires_at).toLocaleDateString() : 'Never'}</td>
                    <td style={{ padding: '12px 16px' }}><Btn size="sm" variant="danger" onClick={() => revokeToken(tok.id)} disabled={revoking === tok.id}>{revoking === tok.id ? '…' : 'Revoke'}</Btn></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </div>
  );
}
