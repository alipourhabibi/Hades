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
import { IconShield, IconAlert, IconEye, IconCheck, IconX } from '@/components/icons';
import Badge from '@/components/ui/Badge';

function passwordStrength(p: string) {
  if (!p) return { score: 0, label: '', color: 'transparent' };
  let s = 0;
  if (p.length >= 8) s++;
  if (/[A-Z]/.test(p)) s++;
  if (/[0-9]/.test(p)) s++;
  if (/[^A-Za-z0-9]/.test(p)) s++;
  const levels = [
    { label: 'Too short', color: 'var(--c-danger)' },
    { label: 'Weak',      color: 'var(--c-danger)' },
    { label: 'Fair',      color: 'var(--c-warning)' },
    { label: 'Good',      color: 'var(--c-warning)' },
    { label: 'Strong',    color: 'var(--c-success)' },
  ];
  return { score: s, ...levels[Math.min(s, 4)] };
}

export default function PageSignup() {
  const router = useRouter();
  const { setAuth } = useAuthStore();
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [agreed, setAgreed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const strength = passwordStrength(password);

  const submit = async () => {
    if (!email || !username || !password || !confirm) { setError('Please fill in all fields.'); return; }
    if (password !== confirm) { setError('Passwords do not match.'); return; }
    if (!agreed) { setError('Please agree to the terms.'); return; }
    setError(''); setLoading(true);
    try {
      await rpcFetch('/hades.api.authentication.v1.AuthenticationService/Register', { username, email, password });
      try {
        const data = await rpcFetch<{ token: string }>('/hades.api.authentication.v1.AuthenticationService/Login', { username, password });
        if (data.token) { setAuth(data.token, username); router.push('/'); return; }
      } catch { /* login after register failed, redirect to login page */ }
      router.push('/login');
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

      <div style={{ width: '100%', maxWidth: 520 }}>
        <Card style={{ padding: '32px 36px' }}>
          <h2 style={{ margin: '0 0 6px', fontSize: 20, fontWeight: 700, color: 'var(--c-fg)', letterSpacing: -0.3 }}>Create your account</h2>
          <p style={{ margin: '0 0 24px', fontSize: 13, color: 'var(--c-fg-muted)' }}>Self-hosted, open-source Protobuf schema registry.</p>

          {error && (
            <div style={{ padding: '10px 14px', borderRadius: 6, background: 'var(--c-danger-bg)', border: '1px solid var(--c-danger)', color: 'var(--c-danger)', fontSize: 13, marginBottom: 16, display: 'flex', gap: 8, alignItems: 'center' }}>
              <IconAlert size={14}/>{error}
            </div>
          )}

          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div>
                <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Email address</label>
                <Input value={email} onChange={setEmail} type="email" placeholder="you@company.com"/>
              </div>
              <div>
                <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Username</label>
                <Input value={username} onChange={setUsername} placeholder="jane.doe" prefix={<span style={{ fontSize: 13 }}>@</span>}/>
              </div>
            </div>
            <div>
              <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Password</label>
              <Input value={password} onChange={setPassword} type={showPass ? 'text' : 'password'} placeholder="Minimum 8 characters"
                suffix={<button type="button" onClick={() => setShowPass(v => !v)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-subtle)', display: 'flex', padding: 0 }}><IconEye size={14}/></button>}/>
              {password && (
                <div style={{ marginTop: 8 }}>
                  <div style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
                    {[0,1,2,3].map(i => (
                      <div key={i} style={{ flex: 1, height: 3, borderRadius: 2, background: i < strength.score ? strength.color : 'var(--c-border)', transition: 'background 0.2s' }}/>
                    ))}
                  </div>
                  <span style={{ fontSize: 11, color: strength.color }}>{strength.label}</span>
                </div>
              )}
            </div>
            <div>
              <label style={{ fontSize: 12, fontWeight: 500, color: 'var(--c-fg-muted)', display: 'block', marginBottom: 6 }}>Confirm password</label>
              <Input value={confirm} onChange={setConfirm} type="password" placeholder="Re-enter password"
                suffix={confirm ? <span style={{ color: confirm === password ? 'var(--c-success)' : 'var(--c-danger)' }}>{confirm === password ? <IconCheck size={14}/> : <IconX size={14}/>}</span> : undefined}/>
            </div>
            <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start', cursor: 'pointer', fontSize: 13, color: 'var(--c-fg-muted)', lineHeight: 1.5 }}>
              <input type="checkbox" checked={agreed} onChange={e => setAgreed(e.target.checked)} style={{ marginTop: 2, accentColor: 'var(--c-accent)', width: 14, height: 14, flexShrink: 0 }}/>
              <span>I agree to the <span style={{ color: 'var(--c-accent)' }}>Terms of Service</span> and <span style={{ color: 'var(--c-accent)' }}>Privacy Policy</span>.</span>
            </label>
            <Btn variant="primary" size="lg" style={{ width: '100%', justifyContent: 'center' }} onClick={submit} disabled={loading}>
              {loading ? 'Creating account…' : 'Create account'}
            </Btn>
          </div>
        </Card>
        <p style={{ textAlign: 'center', fontSize: 13, color: 'var(--c-fg-muted)', marginTop: 20 }}>
          Already have an account?{' '}
          <Link href="/login" style={{ color: 'var(--c-accent)', fontWeight: 500 }}>Sign in</Link>
        </p>
      </div>
    </div>
  );
}
