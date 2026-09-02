import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { SmartZoneClient, createApi } from '@/api';
import type { SmartZoneApi, Session } from '@/api';
import {
  deleteProfile,
  forgetPassword,
  forgetSession,
  getActiveProfileId,
  getPassword,
  loadProfiles,
  loadSession,
  saveProfile,
  saveSession,
  setActiveProfileId,
  setPassword,
} from './store';
import type { ControllerProfile } from './types';

export type ConnectionState =
  | 'loading' // reading storage
  | 'noControllers'
  | 'locked' // a controller is selected but its password is not available
  | 'connecting'
  | 'connected'
  | 'error';

interface ControllerContextValue {
  state: ConnectionState;
  profiles: ControllerProfile[];
  activeProfile: ControllerProfile | null;
  api: SmartZoneApi | null;
  session: Session | null;
  error: string | null;

  addController(
    profile: Omit<ControllerProfile, 'id' | 'createdAt'> & { id?: string },
    password: string,
  ): Promise<ControllerProfile>;
  updateController(profile: ControllerProfile): Promise<void>;
  removeController(id: string): Promise<void>;
  switchTo(id: string): Promise<void>;
  /** Re-authenticate, optionally with a password the operator just typed. */
  reconnect(password?: string): Promise<void>;
  signOut(options?: { forget?: boolean }): Promise<void>;
}

const ControllerContext = createContext<ControllerContextValue | null>(null);

export function ControllerProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<ConnectionState>('loading');
  const [profiles, setProfiles] = useState<ControllerProfile[]>([]);
  const [activeProfile, setActiveProfile] = useState<ControllerProfile | null>(null);
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [api, setApi] = useState<SmartZoneApi | null>(null);

  /**
   * Guards against a slow connect for controller A landing after the operator
   * has already switched to controller B.
   */
  const connectToken = useRef(0);

  const connect = useCallback(
    async (profile: ControllerProfile, passwordOverride?: string) => {
      const token = ++connectToken.current;
      setState('connecting');
      setError(null);

      const password = passwordOverride ?? (await getPassword(profile.id));
      if (!password) {
        if (connectToken.current !== token) return;
        setApi(null);
        setSession(null);
        setState('locked');
        return;
      }

      const cached = passwordOverride ? null : await loadSession(profile.id);

      const client = new SmartZoneClient({
        endpoint: {
          host: profile.host,
          port: profile.port,
          apiVersion: profile.apiVersion,
        },
        credentials: {
          username: profile.username,
          password,
          domainId: profile.domainId,
        },
        initialSession: cached ?? undefined,
        onSession: (next) => {
          void saveSession(profile.id, next);
          if (connectToken.current === token) setSession(next);
        },
      });

      try {
        const live = cached ?? (await client.login());
        if (connectToken.current !== token) return;

        setApi(createApi(client));
        setSession(live);
        setState('connected');

        const stamped: ControllerProfile = {
          ...profile,
          lastUsedAt: Date.now(),
          controllerVersion: live.controllerVersion ?? profile.controllerVersion,
        };
        setActiveProfile(stamped);
        setProfiles(await saveProfile(stamped));
      } catch (err) {
        if (connectToken.current !== token) return;
        setApi(null);
        setSession(null);
        setState('error');
        setError(describe(err));
      }
    },
    [],
  );

  // Restore the last controller on cold start.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [stored, activeId] = await Promise.all([
        loadProfiles(),
        getActiveProfileId(),
      ]);
      if (cancelled) return;

      setProfiles(stored);
      if (stored.length === 0) {
        setState('noControllers');
        return;
      }

      const profile = stored.find((p) => p.id === activeId) ?? stored[0];
      if (!profile) {
        setState('noControllers');
        return;
      }
      setActiveProfile(profile);
      await setActiveProfileId(profile.id);
      await connect(profile);
    })();
    return () => {
      cancelled = true;
    };
  }, [connect]);

  const addController = useCallback<ControllerContextValue['addController']>(
    async (input, password) => {
      const profile: ControllerProfile = {
        ...input,
        id: input.id ?? `c_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
        createdAt: Date.now(),
      };
      await setPassword(profile.id, password);
      const next = await saveProfile(profile);
      setProfiles(next);
      await setActiveProfileId(profile.id);
      setActiveProfile(profile);
      await connect(profile, password);
      return profile;
    },
    [connect],
  );

  const updateController = useCallback(async (profile: ControllerProfile) => {
    setProfiles(await saveProfile(profile));
    setActiveProfile((current) =>
      current?.id === profile.id ? profile : current,
    );
  }, []);

  const removeController = useCallback(
    async (id: string) => {
      const next = await deleteProfile(id);
      setProfiles(next);
      if (activeProfile?.id !== id) return;

      const replacement = next[0] ?? null;
      setActiveProfile(replacement);
      setApi(null);
      setSession(null);
      if (!replacement) {
        setState('noControllers');
        return;
      }
      await setActiveProfileId(replacement.id);
      await connect(replacement);
    },
    [activeProfile?.id, connect],
  );

  const switchTo = useCallback(
    async (id: string) => {
      const profile = profiles.find((p) => p.id === id);
      if (!profile) return;
      setApi(null);
      setSession(null);
      setActiveProfile(profile);
      await setActiveProfileId(id);
      await connect(profile);
    },
    [connect, profiles],
  );

  const reconnect = useCallback(
    async (password?: string) => {
      if (!activeProfile) return;
      if (password) await setPassword(activeProfile.id, password);
      await forgetSession(activeProfile.id);
      await connect(activeProfile, password);
    },
    [activeProfile, connect],
  );

  const signOut = useCallback<ControllerContextValue['signOut']>(
    async (options) => {
      const profile = activeProfile;
      setApi(null);
      setSession(null);
      if (!profile) return;
      await forgetSession(profile.id);
      if (options?.forget) await forgetPassword(profile.id);
      setState('locked');
    },
    [activeProfile],
  );

  const value = useMemo<ControllerContextValue>(
    () => ({
      state,
      profiles,
      activeProfile,
      api,
      session,
      error,
      addController,
      updateController,
      removeController,
      switchTo,
      reconnect,
      signOut,
    }),
    [
      state,
      profiles,
      activeProfile,
      api,
      session,
      error,
      addController,
      updateController,
      removeController,
      switchTo,
      reconnect,
      signOut,
    ],
  );

  return (
    <ControllerContext.Provider value={value}>
      {children}
    </ControllerContext.Provider>
  );
}

export function useControllers(): ControllerContextValue {
  const ctx = useContext(ControllerContext);
  if (!ctx) {
    throw new Error('useControllers must be used inside a ControllerProvider');
  }
  return ctx;
}

/**
 * The API for the connected controller.
 *
 * Throws when nothing is connected, so screens behind the connected gate can
 * use it without a null check on every line. Screens that render before a
 * connection use `useControllers()` instead.
 */
export function useApi(): SmartZoneApi {
  const { api } = useControllers();
  if (!api) throw new Error('No controller is connected');
  return api;
}

function describe(err: unknown): string {
  if (err && typeof err === 'object' && 'displayMessage' in err) {
    return String((err as { displayMessage: string }).displayMessage);
  }
  return err instanceof Error ? err.message : 'Could not connect.';
}
