'use client';
import React, { useState, useEffect, useRef } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Link from 'next/link';
import { getSidebarCollapsed, setSidebarCollapsed, getTheme, setTheme, getRecentModules, getUsername } from '../lib/auth';
import { rpcFetch } from '../lib/rpc';
import { useAuthStore } from '../stores/authStore';
import {
  IconHome, IconSearch, IconGear, IconKey, IconShield,
  IconMenu, IconPlus, IconBell, IconSun, IconMoon, IconLogout, IconLock, IconGlobe,
  IconUser, IconClock, IconBuilding,
} from '../components/icons';
import { HadesLogo, HadesLogoHorizontal } from '../components/icons/HadesLogo';
import Avatar from '../components/ui/Avatar';
import Badge from '../components/ui/Badge';
import Btn from '../components/ui/Button';
import Divider from '../components/ui/Divider';
import NotificationsDrawer from '../components/overlays/NotificationsDrawer';
import ModuleWizard from '../components/overlays/ModuleWizard';

const NAV_ITEMS = [
  { to: '/',                  label: 'Home',          icon: <IconHome size={15}/> },
  { to: '/search',            label: 'Search',        icon: <IconSearch size={15}/> },
];
const BOTTOM_NAV = [
  { to: '/settings/sessions', label: 'Sessions',      icon: <IconShield size={15}/> },
  { to: '/settings/tokens',   label: 'API Tokens',    icon: <IconKey size={15}/> },
  { to: '/settings',          label: 'Settings',      icon: <IconGear size={15}/> },
];

interface NavItemProps {
  to: string;
  label: string;
  icon: React.ReactNode;
  collapsed: boolean;
  active: boolean;
}

const NavItem: React.FC<NavItemProps> = ({ to, label, icon, collapsed, active }) => (
  <Link href={to} title={collapsed ? label : undefined} style={{
    display: 'flex', alignItems: 'center', gap: 10, padding: collapsed ? '10px 14px' : '8px 14px',
    borderRadius: 6, textDecoration: 'none', fontSize: 13, fontWeight: 500, transition: 'all 0.1s',
    color: active ? 'var(--c-accent)' : 'var(--c-fg-muted)',
    background: active ? 'var(--c-accent-bg)' : 'transparent',
    borderLeft: active ? '2px solid var(--c-accent)' : '2px solid transparent',
    justifyContent: collapsed ? 'center' : 'flex-start',
  }}>
    {icon}
    {!collapsed && label}
  </Link>
);

interface UserInfo {
  id: string;
  username: string;
  email: string;
  description?: string;
  url?: string;
  createTime?: string;
}

function fmtDate(ts?: string): string {
  if (!ts) return '-';
  try { return new Date(ts).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }); }
  catch { return ts; }
}

const AppShell: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const router = useRouter();
  const pathname = usePathname();
  const { clearAuth, username } = useAuthStore();
  const [collapsed, setCollapsed] = useState(false);
  const [isDark, setIsDark] = useState(true);
  const [notifOpen, setNotifOpen] = useState(false);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [userPopupOpen, setUserPopupOpen] = useState(false);
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false);
  const [recentModules, setRecentModules] = useState<ReturnType<typeof getRecentModules>>([]);
  const userRowRef = useRef<HTMLDivElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setCollapsed(getSidebarCollapsed());
    setIsDark(getTheme() === 'dark');
    setRecentModules(getRecentModules());
    setIsAuthenticated(!!document.cookie.match(/hades_token=([^;]+)/)?.[1]);
  }, []);

  // Fetch current user on mount
  useEffect(() => {
    const token = document.cookie.match(/hades_token=([^;]+)/)?.[1];
    if (!token) return;
    rpcFetch<{ user: UserInfo }>('/hades.api.authorization.v1.Authorization/UserBySession', {})
      .then(data => { if (data?.user) setUserInfo(data.user); })
      .catch(() => {});
  }, []);

  // Close popup when clicking outside
  useEffect(() => {
    if (!userPopupOpen) return;
    const handler = (e: MouseEvent) => {
      if (
        popupRef.current && !popupRef.current.contains(e.target as Node) &&
        userRowRef.current && !userRowRef.current.contains(e.target as Node)
      ) {
        setUserPopupOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [userPopupOpen]);

  const toggleSidebar = () => {
    const next = !collapsed;
    setCollapsed(next);
    setSidebarCollapsed(next);
  };

  const toggleTheme = () => {
    const next = isDark ? 'light' : 'dark';
    setIsDark(!isDark);
    setTheme(next);
  };

  const doLogout = () => {
    rpcFetch('/hades.api.authentication.v1.AuthenticationService/Logout', {}).catch(() => {});
    clearAuth();
    setIsAuthenticated(false);
    router.push('/login');
  };

  const handleLogout = () => {
    setUserPopupOpen(false);
    setLogoutConfirmOpen(true);
  };

  const displayName = userInfo?.username || username || getUsername() || 'User';
  const displayEmail = userInfo?.email || '';
  const initials = displayName.slice(0, 2).toUpperCase();
  const sidebarW = collapsed ? 52 : 256;

  return (
    <div suppressHydrationWarning style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--c-bg-canvas)' }}>
      {/* ── Sidebar ── */}
      <aside suppressHydrationWarning style={{
        width: sidebarW, flexShrink: 0, background: 'var(--c-bg-default)',
        borderRight: '1px solid var(--c-border)', display: 'flex', flexDirection: 'column',
        transition: 'width 0.15s', overflow: 'hidden',
      }}>
        {/* Logo row */}
        <div style={{ height: 56, display: 'flex', alignItems: 'center', justifyContent: collapsed ? 'center' : 'space-between', padding: collapsed ? '0 10px' : '0 12px 0 14px', borderBottom: '1px solid var(--c-border)', flexShrink: 0 }}>
          {collapsed
            ? <HadesLogo size={28} theme={isDark ? 'dark' : 'light'} id="sidebar-collapsed"/>
            : <HadesLogoHorizontal theme={isDark ? 'dark' : 'light'} iconSize={28}/>
          }
          <button onClick={toggleSidebar} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex', padding: 4, borderRadius: 4, flexShrink: 0 }}>
            <IconMenu size={15}/>
          </button>
        </div>

        {/* Quick search */}
        <div style={{ padding: '8px 8px 0' }}>
          <button
            onClick={() => router.push('/search')}
            title="Search (⌘K)"
            style={{
              width: '100%', display: 'flex', alignItems: 'center',
              gap: collapsed ? 0 : 8,
              padding: collapsed ? '8px 0' : '7px 10px',
              justifyContent: collapsed ? 'center' : 'flex-start',
              background: 'var(--c-bg-subtle)', border: '1px solid var(--c-border)',
              borderRadius: 6, cursor: 'pointer', color: 'var(--c-fg-muted)',
              transition: 'all 0.1s',
            }}
            onMouseEnter={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--c-accent)'; (e.currentTarget as HTMLElement).style.color = 'var(--c-fg)'; }}
            onMouseLeave={e => { (e.currentTarget as HTMLElement).style.borderColor = 'var(--c-border)'; (e.currentTarget as HTMLElement).style.color = 'var(--c-fg-muted)'; }}
          >
            <IconSearch size={14}/>
            {!collapsed && (
              <>
                <span style={{ flex: 1, fontSize: 12, color: 'var(--c-fg-subtle)', textAlign: 'left' }}>Search…</span>
                <span style={{ fontSize: 10, color: 'var(--c-fg-subtle)', background: 'var(--c-bg-overlay)', border: '1px solid var(--c-border)', borderRadius: 3, padding: '1px 4px', fontFamily: "'IBM Plex Mono', monospace" }}>⌘K</span>
              </>
            )}
          </button>
        </div>

        {/* Nav */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '10px 8px', display: 'flex', flexDirection: 'column', gap: 2 }}>
          {!collapsed && <div style={{ fontSize: 10, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.8, padding: '4px 6px 6px', marginTop: 4 }}>Main</div>}
          {NAV_ITEMS.map(item => (
            <NavItem key={item.to} {...item} collapsed={collapsed} active={pathname === item.to}/>
          ))}
          {isAuthenticated && (
            <NavItem
              to={`/${displayName}`}
              label="My Profile"
              icon={<IconUser size={15}/>}
              collapsed={collapsed}
              active={pathname === `/${displayName}`}
            />
          )}
          <NavItem
            to="/search?type=orgs"
            label="Organizations"
            icon={<IconBuilding size={15}/>}
            collapsed={collapsed}
            active={pathname === '/search'}
          />

          {recentModules.length > 0 && !collapsed && (
            <>
              <div style={{ fontSize: 10, fontWeight: 600, color: 'var(--c-fg-subtle)', textTransform: 'uppercase', letterSpacing: 0.8, padding: '8px 6px 6px', marginTop: 4 }}>Recent</div>
              {recentModules.map(m => (
                <Link key={m.fullName} href={`/${m.fullName}`} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '7px 14px', borderRadius: 6, textDecoration: 'none', fontSize: 12, color: 'var(--c-fg-muted)', transition: 'background 0.1s', borderLeft: '2px solid transparent' }}
                  onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'}
                  onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}>
                  <Avatar initials={m.owner.slice(0, 2)} size={18}/>
                  <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontFamily: "'IBM Plex Mono', monospace" }}>{m.fullName}</span>
                  {m.visibility === 'private' ? <IconLock size={10}/> : <IconGlobe size={10}/>}
                </Link>
              ))}
            </>
          )}
        </div>

        {/* Bottom nav (auth-only) */}
        {isAuthenticated && (
          <div style={{ borderTop: '1px solid var(--c-border)', padding: '8px 8px 6px', display: 'flex', flexDirection: 'column', gap: 2 }}>
            {BOTTOM_NAV.map(item => (
              <NavItem key={item.to} {...item} collapsed={collapsed} active={pathname === item.to}/>
            ))}
            <button
              onClick={handleLogout}
              title="Sign out"
              style={{
                display: 'flex', alignItems: 'center', gap: 10,
                padding: collapsed ? '10px 14px' : '8px 14px',
                borderRadius: 6, border: 'none', background: 'none', cursor: 'pointer',
                fontSize: 13, fontWeight: 500, width: '100%',
                color: 'var(--c-danger)',
                justifyContent: collapsed ? 'center' : 'flex-start',
                transition: 'background 0.1s',
                fontFamily: 'inherit',
              }}
              onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-overlay)'}
              onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'none'}
            >
              <IconLogout size={15}/>
              {!collapsed && 'Sign out'}
            </button>
          </div>
        )}

        {/* User row (authenticated) or Login/Signup (guest) */}
        {isAuthenticated ? (
          <div
            ref={userRowRef}
            onClick={() => setUserPopupOpen(v => !v)}
            title={collapsed ? displayName : undefined}
            style={{
              borderTop: '1px solid var(--c-border)',
              padding: collapsed ? '10px 0' : '10px 12px',
              display: 'flex', alignItems: 'center', gap: 10,
              cursor: 'pointer', transition: 'background 0.1s',
              justifyContent: collapsed ? 'center' : 'flex-start',
              background: userPopupOpen ? 'var(--c-bg-overlay)' : 'transparent',
            }}
            onMouseEnter={e => { if (!userPopupOpen) (e.currentTarget as HTMLElement).style.background = 'var(--c-bg-subtle)'; }}
            onMouseLeave={e => { if (!userPopupOpen) (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
          >
            <Avatar initials={initials} size={30}/>
            {!collapsed && (
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--c-fg)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.3 }}>
                  {displayName}
                </div>
                {displayEmail && (
                  <div style={{ fontSize: 11, color: 'var(--c-fg-subtle)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.3, marginTop: 1 }}>
                    {displayEmail}
                  </div>
                )}
              </div>
            )}
          </div>
        ) : (
          <div style={{ borderTop: '1px solid var(--c-border)', padding: '8px 8px 10px', display: 'flex', flexDirection: 'column', gap: 6 }}>
            <Link href="/login" style={{
              display: 'flex', alignItems: 'center', justifyContent: collapsed ? 'center' : 'center',
              gap: 8, padding: '8px 14px', borderRadius: 6, textDecoration: 'none',
              fontSize: 13, fontWeight: 600, color: '#fff',
              background: 'var(--c-accent)', transition: 'opacity 0.1s',
            }}>
              {!collapsed && 'Sign in'}
              {collapsed && <IconUser size={15}/>}
            </Link>
            {!collapsed && (
              <Link href="/signup" style={{
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                gap: 8, padding: '7px 14px', borderRadius: 6, textDecoration: 'none',
                fontSize: 13, fontWeight: 500, color: 'var(--c-fg-muted)',
                border: '1px solid var(--c-border)', background: 'var(--c-bg-subtle)',
              }}>
                Sign up
              </Link>
            )}
          </div>
        )}
      </aside>

      {/* ── User info popup ── */}
      {isAuthenticated && userPopupOpen && (
        <div
          ref={popupRef}
          style={{
            position: 'fixed',
            bottom: 8,
            left: sidebarW + 8,
            width: 280,
            background: 'var(--c-bg-default)',
            border: '1px solid var(--c-border)',
            borderRadius: 10,
            boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
            zIndex: 500,
            overflow: 'hidden',
          }}
        >
          <div style={{ padding: '16px 16px 12px', display: 'flex', alignItems: 'flex-start', gap: 12 }}>
            <Avatar initials={initials} size={40}/>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--c-fg)', marginBottom: 2 }}>{displayName}</div>
              {displayEmail && (
                <div style={{ fontSize: 12, color: 'var(--c-fg-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{displayEmail}</div>
              )}
              {userInfo?.url && (
                <div style={{ fontSize: 11, color: 'var(--c-accent)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{userInfo.url}</div>
              )}
            </div>
          </div>

          {userInfo?.description && (
            <>
              <Divider/>
              <div style={{ padding: '10px 16px', fontSize: 12, color: 'var(--c-fg-muted)', lineHeight: 1.5 }}>
                {userInfo.description}
              </div>
            </>
          )}

          <Divider/>

          <div style={{ padding: '10px 16px', display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--c-fg-muted)' }}>
              <IconUser size={13} style={{ color: 'var(--c-fg-subtle)', flexShrink: 0 }}/>
              <span style={{ fontFamily: "'IBM Plex Mono', monospace", color: 'var(--c-fg)' }}>{displayName}</span>
            </div>
            {userInfo?.createTime && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--c-fg-muted)' }}>
                <IconClock size={13} style={{ color: 'var(--c-fg-subtle)', flexShrink: 0 }}/>
                <span>Member since {fmtDate(userInfo.createTime)}</span>
              </div>
            )}
          </div>

          <Divider/>

          <div style={{ padding: '8px 10px', display: 'flex', gap: 6 }}>
            <Btn
              size="sm"
              style={{ flex: 1, justifyContent: 'center' }}
              onClick={() => { setUserPopupOpen(false); router.push(`/${displayName}`); }}
            >
              View Profile
            </Btn>
            <Btn
              size="sm"
              variant="ghost"
              icon={<IconLogout size={13}/>}
              onClick={handleLogout}
              style={{ color: 'var(--c-danger)' }}
            >
              Sign out
            </Btn>
          </div>
        </div>
      )}

      {/* ── Main area ── */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Topbar */}
        <header suppressHydrationWarning style={{ height: 52, flexShrink: 0, borderBottom: '1px solid var(--c-border)', background: 'var(--c-bg-default)', display: 'flex', alignItems: 'center', padding: '0 20px', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Badge variant="green" style={{ fontSize: 11 }}>buf.build compatible</Badge>
            <Badge variant="default" style={{ fontSize: 11 }}>v0.1.0</Badge>
          </div>
          <div style={{ flex: 1 }}/>
          {isAuthenticated && <Btn variant="primary" size="sm" icon={<IconPlus size={13}/>} onClick={() => setWizardOpen(true)}>New Module</Btn>}
          <button onClick={() => router.push('/search')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex', padding: 4 }}>
            <IconSearch size={16}/>
          </button>
          <button onClick={() => setNotifOpen(true)} style={{ position: 'relative', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex', padding: 4 }}>
            <IconBell size={16}/>
          </button>
          <button onClick={toggleTheme} title="Toggle theme" style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--c-fg-muted)', display: 'flex', padding: 4 }}>
            {isDark ? <IconSun size={16}/> : <IconMoon size={16}/>}
          </button>
          {isAuthenticated && (
            <>
              <div style={{ width: 1, height: 20, background: 'var(--c-border)' }}/>
              <button onClick={() => router.push(`/${displayName}`)} style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex' }}>
                <Avatar initials={initials} size={28}/>
              </button>
            </>
          )}
        </header>

        {/* Page content */}
        <main style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column' }}>
          {children}
        </main>
      </div>

      {/* Overlays */}
      {notifOpen && <NotificationsDrawer onClose={() => setNotifOpen(false)}/>}
      {wizardOpen && <ModuleWizard onClose={() => setWizardOpen(false)} onCreated={(owner, name) => { setWizardOpen(false); router.push(`/${owner}/${name}`); }}/>}

      {/* Logout confirmation dialog */}
      {logoutConfirmOpen && (
        <div
          style={{
            position: 'fixed', inset: 0, zIndex: 1000,
            background: 'rgba(0,0,0,0.5)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}
          onClick={() => setLogoutConfirmOpen(false)}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{
              background: 'var(--c-bg-default)',
              border: '1px solid var(--c-border)',
              borderRadius: 10,
              padding: '24px',
              width: 340,
              boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
            }}
          >
            <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--c-fg)', marginBottom: 8 }}>Sign out</div>
            <div style={{ fontSize: 13, color: 'var(--c-fg-muted)', marginBottom: 20, lineHeight: 1.5 }}>
              Are you sure you want to sign out?
            </div>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Btn size="sm" variant="ghost" onClick={() => setLogoutConfirmOpen(false)}>Cancel</Btn>
              <Btn
                size="sm"
                onClick={doLogout}
                style={{ background: 'var(--c-danger)', color: '#fff', border: 'none' }}
              >
                Sign out
              </Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default AppShell;
