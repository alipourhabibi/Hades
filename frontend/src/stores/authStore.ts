'use client';

import { create } from 'zustand';
import { setToken, clearToken, setUsername, clearUsername } from '../lib/auth';

// The store holds no token.
//
// The session token is in an HttpOnly cookie the browser cannot read, so there
// is nothing to keep here; `username` is what the UI actually renders from.
// setAuth and clearAuth are async because establishing and clearing the cookie
// is a round trip to /api/session.
interface AuthState {
  username: string | null;
  setAuth: (token: string, username: string) => Promise<void>;
  clearAuth: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  username: null,
  setAuth: async (token, username) => {
    await setToken(token);
    setUsername(username);
    set({ username });
  },
  clearAuth: async () => {
    await clearToken();
    clearUsername();
    set({ username: null });
  },
}));
