import React from 'react';
import { Redirect } from 'expo-router';
import { Loading, Screen } from '@/ui/components';
import { useControllers } from './ControllerProvider';

/**
 * Gate for anything that needs a live connection.
 *
 * `useApi()` throws when nothing is connected — that is its contract, and it
 * is what lets screens behind the gate call it without a null check on every
 * line. The catch is that a screen reached by a deep link, or by a cold start
 * straight onto a detail route, mounts *before* the provider has finished
 * connecting. Without this wrapper that throw takes the whole tree down and
 * the operator gets a white screen.
 *
 * The tab layout is one such gate; this is the same rule for the routes that
 * live outside the tabs.
 */
export function ConnectedOnly({ children }: { children: React.ReactNode }) {
  const { state } = useControllers();

  if (state === 'loading' || state === 'connecting') {
    return (
      <Screen>
        <Loading label="Connecting to your controller" />
      </Screen>
    );
  }
  if (state === 'noControllers') return <Redirect href="/connect" />;
  if (state !== 'connected') return <Redirect href="/connect/signin" />;

  return <>{children}</>;
}
