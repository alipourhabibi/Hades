'use client';
import dynamic from 'next/dynamic';

const AppShell = dynamic(() => import('@/layouts/AppShell'), { ssr: false });

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
