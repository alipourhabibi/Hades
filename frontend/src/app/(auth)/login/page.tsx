'use client';
import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useAuthStore } from '@/stores/authStore';
import { rpcFetch } from '@/lib/rpc';
import { formatError } from '@/lib/connectError';
import Card from '@/components/ui/Card';
import Input from '@/components/ui/Input';
import Btn from '@/components/ui/Button';
import { IconShield, IconAlert, IconEye } from '@/components/icons';
import Badge from '@/components/ui/Badge';

const GitHubIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/>
  </svg>
);

export default function PageLogin() {
  const router = useRouter();
  const { setAuth } = useAuthStore();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const submit = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!username || !password) { setError('Please fill in all fields.'); return; }
    setError(''); setLoading(true);
    try {
      const data = await rpcFetch<{ token: string }>('/hades.api.authentication.v1.AuthenticationService/Login', { username, password });
      if (!data.token) throw new Error('No token returned');
      setAuth(data.token, username);
      router.push('/');
    } catch (e) {
      setError(formatError(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ minHeight: '100vh', background: 'var(--c-bg-canvas)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '32px 16px' }}>
      <div style={{ marginBottom: 32, display: 'flex', alignItems: 'center', gap: 10 }}>
        <div style={{ width: 36, height: 36, borderRadius: 9, background: 'linear-gradient(135deg, #1f6feb 0%, #8957e5 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <IconShield size={18} style={{ color: '#fff' }}/>
        </div>
        <span style={{ fontSize: 20, fontWeight: 700, color: 'var(--c-fg)', letterSpacing: -0.5 }}>Hades</span>
        <Badge variant="purple" style={{ fontSize: 11 }}>HSR</Badge>
      </div>

      <div style={{ width: '100%', maxWidth: 420 }}>
        <Card style={{ padding: '32px 36px' }}>
          <h2 style={{ margin: '0 0 6px', fontSize: 20, fontWeight: 700, color: 'var(--c-fg)', letterSpacing: -0.3 }}>Sign in to Hades</h2>
          <p style={{ margin: '0 0 24px', fontSize: 13, color: 'var(--c-fg-muted)' }}>Welcome back. Sign in to your schema registry.</p>

          {error && (
            <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8, alignItems: 'center' }}>
              <IconAlert size={14}/>{error}
            </div>
          )}

          <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div>
              <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Username</label>
              <Input value={username} onChange={setUsername} placeholder="your-username" autoFocus/>
            </div>
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)' }}>Password</label>
                <Link href="/forgot-password" style={{ fontSize: 12, color: 'var(--c-accent)' }}>Forgot password?</Link>
              </div>
              <Input value={password} onChange={setPassword} type={showPass ? 'text' : 'password'} placeholder="••••••••"
                suffix={
                  <button type="button" onClick={() => setShowPass(v => !v)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-subtle)', display: 'flex', padding: 0 }}>
                    <IconEye size={14}/>
                  </button>
                }/>
            </div>
            <Btn type="submit" variant="primary" size="lg" style={{ width: '100%', justifyContent: 'center', marginTop: 4 }} disabled={loading || !username || !password}>
              {loading ? 'Signing in…' : 'Sign in'}
            </Btn>
          </form>

          <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '20px 0' }}>
            <div style={{ flex: 1, borderTop: '1px solid var(--c-border)' }}/>
            <span style={{ fontSize: 11, color: 'var(--c-fg-subtle)', flexShrink: 0 }}>or continue with</span>
            <div style={{ flex: 1, borderTop: '1px solid var(--c-border)' }}/>
          </div>

          <div style={{ display: 'flex', gap: 10 }}>
            <button style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, padding: '9px 14px', borderRadius: 6, border: '1px solid var(--c-border)', background: 'var(--c-bg-inset)', color: 'var(--c-fg)', fontFamily: 'inherit', fontSize: 13, fontWeight: 500, cursor: 'pointer' }}>
              <GitHubIcon/>GitHub
            </button>
            <button style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, padding: '9px 14px', borderRadius: 6, border: '1px solid var(--c-border)', background: 'var(--c-bg-inset)', color: 'var(--c-fg)', fontFamily: 'inherit', fontSize: 13, fontWeight: 500, cursor: 'pointer' }}>
              Google
            </button>
          </div>
        </Card>

        <p style={{ textAlign: 'center', fontSize: 13, color: 'var(--c-fg-muted)', marginTop: 20 }}>
          Don&apos;t have an account?{' '}
          <Link href="/signup" style={{ color: 'var(--c-accent)', fontWeight: 500 }}>Create one</Link>
        </p>
      </div>

      <div style={{ marginTop: 32, fontSize: 12, color: 'var(--c-fg-subtle)', textAlign: 'center' }}>
        © 2026 Hades Registry · Open Source
      </div>
    </div>
  );
}
