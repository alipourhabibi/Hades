import type { NextConfig } from 'next';

// Allowed dev origins for HMR WebSocket; add your local proxy hostname here.
const DEV_ORIGINS = (process.env.ALLOWED_DEV_ORIGINS ?? 'example.com').split(',').map(s => s.trim()).filter(Boolean);

// There is deliberately no rewrite for /api/rpc/:path*.
//
// A rewrite is matched before dynamic routes, so it shadowed the route handler
// at src/app/api/rpc/[...path]/route.ts entirely: requests went straight to the
// backend and the handler never ran. That was harmless while the browser sent
// the session token itself, but the token now lives in an HttpOnly cookie that
// only the handler can read and turn into an Authorization header. With the
// rewrite in place every authenticated call arrived unauthenticated, came back
// 401, and rpcFetch signed the user out.
//
// The handler forwards to BACKEND_URL itself, so the rewrite bought nothing.
// It does mean the backend sees the frontend as the peer: deployments that care
// about client IP for rate limiting and audit records need a reverse proxy that
// sets X-Forwarded-For (the handler passes it through) and the frontend's
// address listed in server.trustedProxies.
const nextConfig: NextConfig = {
  output: 'standalone',
  allowedDevOrigins: DEV_ORIGINS,
};

export default nextConfig;
