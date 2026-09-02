import React from 'react';
import { StatusBar } from 'expo-status-bar';
import { Stack, type ErrorBoundaryProps } from 'expo-router';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ControllerProvider } from '@/controllers/ControllerProvider';
import { ErrorState, Screen } from '@/ui/components';
import { useTheme } from '@/ui/theme';

/**
 * Defaults chosen for a phone on someone else's network: two retries at most,
 * no refetch storm when the app returns to the foreground, and cached data
 * kept long enough that walking between rooms does not empty every screen.
 */
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 15_000,
      gcTime: 10 * 60_000,
    },
    mutations: { retry: 0 },
  },
});

function RootStack() {
  const t = useTheme();
  return (
    <>
      <StatusBar style={t.scheme === 'dark' ? 'light' : 'dark'} />
      <Stack
        screenOptions={{
          headerStyle: { backgroundColor: t.colors.background },
          headerTintColor: t.colors.accent,
          headerTitleStyle: { color: t.colors.text },
          headerShadowVisible: false,
          contentStyle: { backgroundColor: t.colors.background },
        }}
      >
        <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
        <Stack.Screen name="connect" options={{ headerShown: false }} />
        <Stack.Screen
          name="dpsk/generate"
          options={{ presentation: 'modal', title: 'Generate keys' }}
        />
      </Stack>
    </>
  );
}

export default function RootLayout() {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <QueryClientProvider client={queryClient}>
          <ControllerProvider>
            <RootStack />
          </ControllerProvider>
        </QueryClientProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}

/**
 * expo-router renders this instead of unmounting the tree when a screen
 * throws. Without it a crash is a white screen, which tells the operator
 * nothing and cannot be reported.
 */
export function ErrorBoundary({ error, retry }: ErrorBoundaryProps) {
  return (
    <SafeAreaProvider>
      <Screen scroll>
        <ErrorState
          message={error.message}
          hint="This is a fault in the app rather than in the controller. Going back and trying again usually works."
          onRetry={() => void retry()}
        />
      </Screen>
    </SafeAreaProvider>
  );
}
