'use client';

import { create } from 'zustand';
import { setToken, clearToken, setUsername, clearUsername, getUsername } from '../lib/auth';

interface AuthState {
  token: string | null;
  username: string | null;
  setAuth: (token: string, username: string) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  username: null,
  setAuth: (token, username) => {
    setToken(token);
    setUsername(username);
    set({ token, username });
  },
  clearAuth: () => {
    clearToken();
    clearUsername();
    set({ token: null, username: null });
  },
}));
