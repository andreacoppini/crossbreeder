import AsyncStorage from '@react-native-async-storage/async-storage';
import type { Session } from '@/api';
import { deleteSecret, getSecret, setSecret } from './secureStorage';
import type { ControllerProfile } from './types';

const PROFILES_KEY = 'smartzone.controllers.v1';
const ACTIVE_KEY = 'smartzone.controllers.active';

const passwordKey = (id: string) => `smartzone.pw.${id}`;
const sessionKey = (id: string) => `smartzone.session.${id}`;

export async function loadProfiles(): Promise<ControllerProfile[]> {
  const raw = await AsyncStorage.getItem(PROFILES_KEY);
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as ControllerProfile[]) : [];
  } catch {
    // Corrupt storage should not brick the app; the operator re-adds.
    return [];
  }
}

async function writeProfiles(profiles: ControllerProfile[]): Promise<void> {
  await AsyncStorage.setItem(PROFILES_KEY, JSON.stringify(profiles));
}

export async function saveProfile(
  profile: ControllerProfile,
): Promise<ControllerProfile[]> {
  const profiles = await loadProfiles();
  const index = profiles.findIndex((p) => p.id === profile.id);
  if (index >= 0) profiles[index] = profile;
  else profiles.push(profile);
  await writeProfiles(profiles);
  return profiles;
}

/** Remove a controller and every secret held for it. */
export async function deleteProfile(id: string): Promise<ControllerProfile[]> {
  const profiles = (await loadProfiles()).filter((p) => p.id !== id);
  await writeProfiles(profiles);
  await Promise.all([
    deleteSecret(passwordKey(id)),
    deleteSecret(sessionKey(id)),
  ]);
  const active = await getActiveProfileId();
  if (active === id) await setActiveProfileId(profiles[0]?.id ?? null);
  return profiles;
}

export async function getActiveProfileId(): Promise<string | null> {
  return AsyncStorage.getItem(ACTIVE_KEY);
}

export async function setActiveProfileId(id: string | null): Promise<void> {
  if (id) await AsyncStorage.setItem(ACTIVE_KEY, id);
  else await AsyncStorage.removeItem(ACTIVE_KEY);
}

export function setPassword(id: string, password: string): Promise<void> {
  return setSecret(passwordKey(id), password);
}

export function getPassword(id: string): Promise<string | null> {
  return getSecret(passwordKey(id));
}

export function forgetPassword(id: string): Promise<void> {
  return deleteSecret(passwordKey(id));
}

/**
 * Cache the service ticket so reopening the app does not cost a login.
 *
 * SmartZone expires tickets at 24 hours; anything older is discarded on read
 * rather than sent, so the first call after a long gap is a clean login
 * instead of a 401 and a retry.
 */
const TICKET_MAX_AGE_MS = 23 * 60 * 60 * 1000;

export async function saveSession(id: string, session: Session): Promise<void> {
  await setSecret(sessionKey(id), JSON.stringify(session));
}

export async function loadSession(id: string): Promise<Session | null> {
  const raw = await getSecret(sessionKey(id));
  if (!raw) return null;
  try {
    const session = JSON.parse(raw) as Session;
    if (!session.serviceTicket) return null;
    if (Date.now() - session.issuedAt > TICKET_MAX_AGE_MS) return null;
    return session;
  } catch {
    return null;
  }
}

export function forgetSession(id: string): Promise<void> {
  return deleteSecret(sessionKey(id));
}

/** Stable id for a new profile, without pulling in a uuid dependency. */
export function newProfileId(): string {
  return `c_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}
