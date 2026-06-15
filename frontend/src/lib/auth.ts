export const TOKEN_KEY = 'hades_token';
export const USERNAME_KEY = 'hades_username';
export const THEME_KEY = 'hades_theme';
export const SIDEBAR_KEY = 'hades_sidebar_collapsed';
export const RECENT_KEY = 'hades_recent_modules';

// ── Cookie helpers (client-side) ──────────────────────────────────────────────

export function getCookieValue(name: string): string | null {
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

export function setCookie(name: string, value: string, days = 30, httpOnly = false): void {
  if (typeof document === 'undefined') return;
  const expires = new Date(Date.now() + days * 864e5).toUTCString();
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax${httpOnly ? '; HttpOnly' : ''}`;
}

export function deleteCookie(name: string): void {
  if (typeof document === 'undefined') return;
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
}

// ── Token ─────────────────────────────────────────────────────────────────────

export function getToken(): string | null {
  return getCookieValue(TOKEN_KEY);
}

export function setToken(t: string): void {
  setCookie(TOKEN_KEY, t, 30);
}

export function clearToken(): void {
  deleteCookie(TOKEN_KEY);
}

// ── Username ──────────────────────────────────────────────────────────────────

export function getUsername(): string | null {
  return getCookieValue(USERNAME_KEY);
}

export function setUsername(u: string): void {
  setCookie(USERNAME_KEY, u, 30);
}

export function clearUsername(): void {
  deleteCookie(USERNAME_KEY);
}

// ── Theme ─────────────────────────────────────────────────────────────────────

export function getTheme(): 'dark' | 'light' {
  return (getCookieValue(THEME_KEY) as 'dark' | 'light') || 'dark';
}

export function setTheme(t: 'dark' | 'light'): void {
  setCookie(THEME_KEY, t, 365);
  if (typeof document !== 'undefined') {
    if (t === 'light') {
      document.documentElement.classList.add('light');
    } else {
      document.documentElement.classList.remove('light');
    }
  }
}

// ── Sidebar ───────────────────────────────────────────────────────────────────

export function getSidebarCollapsed(): boolean {
  if (typeof localStorage === 'undefined') return false;
  return localStorage.getItem(SIDEBAR_KEY) === 'true';
}

export function setSidebarCollapsed(v: boolean): void {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(SIDEBAR_KEY, String(v));
}

// ── Recent modules ────────────────────────────────────────────────────────────

export interface RecentModule {
  owner: string;
  name: string;
  fullName: string;
  visibility: string;
}

export function getRecentModules(): RecentModule[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
  } catch {
    return [];
  }
}

export function addRecentModule(m: RecentModule): void {
  if (typeof localStorage === 'undefined') return;
  const existing = getRecentModules().filter(r => r.fullName !== m.fullName);
  localStorage.setItem(RECENT_KEY, JSON.stringify([m, ...existing].slice(0, 4)));
}
