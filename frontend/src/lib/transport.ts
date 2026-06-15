'use client';

import { createConnectTransport } from '@connectrpc/connect-web';
import { getToken, clearToken } from './auth';

export const transport = createConnectTransport({
  baseUrl: '/',
  interceptors: [
    (next) => async (req) => {
      const token = getToken();
      if (token) {
        req.header.set('Authorization', `Bearer ${token}`);
      }
      try {
        return await next(req);
      } catch (err: unknown) {
        if (
          err &&
          typeof err === 'object' &&
          'code' in err &&
          (err as { code: number }).code === 16 // UNAUTHENTICATED
        ) {
          clearToken();
          window.location.href = '/login';
        }
        throw err;
      }
    },
  ],
});
