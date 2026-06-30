import type { NextConfig } from 'next';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:50051';

// Allowed dev origins for HMR WebSocket; add your local proxy hostname here.
const DEV_ORIGINS = (process.env.ALLOWED_DEV_ORIGINS ?? 'example.com').split(',').map(s => s.trim()).filter(Boolean);

const nextConfig: NextConfig = {
  output: 'standalone',
  allowedDevOrigins: DEV_ORIGINS,
  async rewrites() {
    return [
      {
        source: '/api/rpc/:path*',
        destination: `${BACKEND_URL}/:path*`,
      },
    ];
  },
};

export default nextConfig;
