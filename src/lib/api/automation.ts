import { safeInvoke } from './client';
import { AutomationProfile } from '../types/automation';

export async function getAutomationProfile(botId: string): Promise<AutomationProfile> {
  return safeInvoke<AutomationProfile>('get_automation_profile', { botId });
}

export async function saveAutomationProfile(profile: AutomationProfile): Promise<void> {
  return safeInvoke<void>('save_automation_profile', { profile });
}
