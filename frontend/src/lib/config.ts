function resolveDomain(): string {
  if (process.env.NEXT_PUBLIC_DOMAIN) return process.env.NEXT_PUBLIC_DOMAIN;
  if (typeof window !== 'undefined') return window.location.host;
  return 'localhost';
}

export const DOMAIN: string = resolveDomain();
