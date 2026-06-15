'use client';

import { getToken } from './auth';

export async function rpcFetch<T>(path: string, body: unknown): Promise<T> {
  const token = getToken();
  const url = path.startsWith('/api/rpc/') ? path : `/api/rpc${path.startsWith('/') ? '' : '/'}${path}`;
  let resp: Response;
  try {
    resp = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(body),
    });
  } catch (err) {
    throw new Error('Backend unreachable. Is the server running?');
  }

  if (!resp.ok) {
    const text = await resp.text();
    try {
      const json = JSON.parse(text);
      if (json?.message) throw new Error(json.message);
    } catch (e) {
      if (!(e instanceof SyntaxError)) throw e;
    }
    throw new Error(text || `Request failed: ${resp.status}`);
  }

  const contentType = resp.headers.get('content-type') ?? '';
  if (!contentType.includes('application/json') && !contentType.includes('application/proto')) {
    const text = await resp.text();
    if (text.includes('<!DOCTYPE') || text.includes('<html')) {
      throw new Error(`Backend unreachable - got HTML instead of JSON. Is the server running?`);
    }
    throw new Error(`Unexpected content-type: ${contentType}`);
  }

  return resp.json() as Promise<T>;
}
