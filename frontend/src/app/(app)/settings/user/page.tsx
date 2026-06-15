'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function SettingsUserRedirect() {
  const router = useRouter();
  useEffect(() => { router.replace('/settings/tokens'); }, [router]);
  return null;
}
