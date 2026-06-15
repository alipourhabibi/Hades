'use client';
import { useEffect, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { rpcFetch } from '@/lib/rpc';

export default function VerifyEmailPage() {
  const { token } = useParams<{ token: string }>();
  const router = useRouter();
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading');
  const [message, setMessage] = useState('');

  useEffect(() => {
    if (!token) return;
    rpcFetch('/hades.api.authentication.v1.AuthenticationService/VerifyEmail', { token })
      .then(() => {
        setStatus('success');
        setTimeout(() => router.push('/login'), 2500);
      })
      .catch((e: Error) => {
        setStatus('error');
        setMessage(e.message);
      });
  }, [token]);

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', flexDirection: 'column', gap: 16 }}>
      {status === 'loading' && <p style={{ color: 'var(--c-fg-muted)' }}>Verifying your email…</p>}
      {status === 'success' && (
        <>
          <p style={{ color: 'var(--c-success)', fontWeight: 600, fontSize: 18 }}>Email verified!</p>
          <p style={{ color: 'var(--c-fg-muted)', fontSize: 14 }}>Redirecting to login…</p>
        </>
      )}
      {status === 'error' && (
        <>
          <p style={{ color: 'var(--c-danger)', fontWeight: 600, fontSize: 18 }}>Verification failed</p>
          <p style={{ color: 'var(--c-fg-muted)', fontSize: 14 }}>{message}</p>
        </>
      )}
    </div>
  );
}
