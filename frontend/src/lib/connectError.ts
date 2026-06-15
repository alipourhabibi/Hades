import { ConnectError, Code } from '@connectrpc/connect';

export function formatError(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export function isNotFound(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.NotFound;
}

export function isUnauthenticated(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.Unauthenticated;
}
