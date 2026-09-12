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

// setCookie writes a NON-SENSITIVE preference cookie from the browser.
//
// It cannot set HttpOnly: browsers ignore that attribute on cookies written
// through document.cookie. The previous signature took an `httpOnly` parameter
// and appended the flag, which read as protection and provided none. The
// session token is not written here at all any more; it is set by the server
// through /api/session. See setToken.
export function setCookie(name: string, value: string, days = 30): void {
  if (typeof document === 'undefined') return;
  const expires = new Date(Date.now() + days * 864e5).toUTCString();
  const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax${secure}`;
}

export function deleteCookie(name: string): void {
  if (typeof document === 'undefined') return;
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
}

// ── Session token ─────────────────────────────────────────────────────────────
//
// The session token lives in an HttpOnly, SameSite=Lax cookie set by the server
// at /api/session, and is attached to backend calls by the /api/rpc proxy. It is
// deliberately not readable from JavaScript: it used to be an ordinary
// document.cookie value, so any XSS anywhere in the app could read it directly.
//
// There is therefore no getToken. Use isSignedIn to decide what to render;
// authorization is the server's answer, not the browser's.

export async function setToken(t: string): Promise<void> {
  const resp = await fetch('/api/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: t }),
    credentials: 'same-origin',
  });
  if (!resp.ok) throw new Error('Could not establish the session');
}

export async function clearToken(): Promise<void> {
  await fetch('/api/session', { method: 'DELETE', credentials: 'same-origin' });
}

// isSignedIn reports whether this browser believes it has a session.
//
// It reads the username cookie, which is a display value rather than a
// credential. A stale true answer costs a redirect after the first 401; a
// readable token would cost the token.
export function isSignedIn(): boolean {
  return getUsername() !== null;
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
