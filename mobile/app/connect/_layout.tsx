import React from 'react';
import { Stack } from 'expo-router';
import { useTheme } from '@/ui/theme';

export default function ConnectLayout() {
  const t = useTheme();
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: t.colors.background },
        headerTintColor: t.colors.accent,
        headerTitleStyle: { color: t.colors.text },
        headerShadowVisible: false,
        contentStyle: { backgroundColor: t.colors.background },
      }}
    >
      <Stack.Screen name="index" options={{ title: 'Controllers' }} />
      <Stack.Screen name="add" options={{ title: 'Add a controller' }} />
      <Stack.Screen
        name="scan"
        options={{ title: 'Scan', presentation: 'modal' }}
      />
      <Stack.Screen name="signin" options={{ title: 'Sign in' }} />
    </Stack>
  );
}
