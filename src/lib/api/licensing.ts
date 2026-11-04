import { safeInvoke } from './client';
import { EntitlementStatus, LicenseClaims } from '../types/licensing';

export async function getEntitlementStatus(): Promise<EntitlementStatus> {
  return safeInvoke<EntitlementStatus>('get_entitlement_status');
}

export async function activateLicense(token: string): Promise<LicenseClaims> {
  return safeInvoke<LicenseClaims>('activate_license', { token });
}

export async function getHardwareId(): Promise<string> {
  return safeInvoke<string>('get_hardware_id');
}
