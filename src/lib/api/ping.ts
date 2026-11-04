import { safeInvoke } from './client';

export async function ping(message: string): Promise<string> {
  return safeInvoke<string>('ping', { message });
}
