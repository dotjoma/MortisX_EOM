/**
 * Types mirroring Go licensing services
 */

export interface LicenseClaims {
  license_key: string;
  hardware_id: string;
  issued_at: number;
  expires_at: number;
  allowed_instances: number;
  active_capabilities: string[];
}

export type EntitlementStatus =
  | { valid: LicenseClaims }
  | { grace_period: { remaining_seconds: number; claims: LicenseClaims } }
  | 'expired'
  | 'revoked'
  | 'unlicensed';
