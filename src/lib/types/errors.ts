/**
 * Structured error format returned from Go backend
 */

export interface AppError {
  code: string;
  message: string;
  details?: string;
}

export function isAppError(err: unknown): err is AppError {
  return typeof err === 'object' && err !== null && 'code' in err && 'message' in err;
}
