import React from 'react';
import { Redirect } from 'expo-router';
import { useControllers } from '@/controllers/ControllerProvider';
import { Loading, Screen } from '@/ui/components';

/**
 * The gate. Everything past here can assume a controller is connected, which
 * is what lets the rest of the app use `useApi()` without a null check on
 * every screen.
 */
export default function Index() {
  const { state } = useControllers();

  switch (state) {
    case 'loading':
      return (
        <Screen>
          <Loading label="Opening your controllers" />
        </Screen>
      );
    case 'noControllers':
      return <Redirect href="/connect" />;
    case 'connected':
      return <Redirect href="/(tabs)" />;
    case 'locked':
    case 'error':
    case 'connecting':
    default:
      return <Redirect href="/connect/signin" />;
  }
}
