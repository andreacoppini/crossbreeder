import React from 'react';
import { Tabs } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';
import { ConnectedOnly } from '@/controllers/ConnectedOnly';
import { useTheme } from '@/ui/theme';

/**
 * Four tabs and a More.
 *
 * The four are the things an operator opens while standing in front of a
 * problem; everything else — DPSKs, zones, alarms, diagnostics, and switching
 * when it lands — lives one level down under More, where it can grow without
 * the tab bar turning into a menu.
 *
 * This layout is also the gate on being connected, and it is the *only* gate:
 * a route group adds no path segment, so this group's `index` is the app's
 * `/`. An `app/index.tsx` beside it would claim the same route, and two
 * screens redirecting to each other's idea of `/` is an app that hangs on
 * launch. There is deliberately no such file.
 *
 * While a connection is being established this renders in place rather than
 * redirecting, so a cold start does not flash the controller list on its way
 * to the overview.
 */
export default function TabsLayout() {
  return (
    <ConnectedOnly>
      <ConnectedTabs />
    </ConnectedOnly>
  );
}

function ConnectedTabs() {
  const t = useTheme();

  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: t.colors.accent,
        tabBarInactiveTintColor: t.colors.textTertiary,
        tabBarStyle: {
          backgroundColor: t.colors.surface,
          borderTopColor: t.colors.separator,
        },
        headerStyle: { backgroundColor: t.colors.background },
        headerTitleStyle: { color: t.colors.text },
        headerTintColor: t.colors.accent,
        headerShadowVisible: false,
        sceneStyle: { backgroundColor: t.colors.background },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: 'Overview',
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="speedometer-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="aps"
        options={{
          title: 'APs',
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="hardware-chip-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="wlans"
        options={{
          title: 'WLANs',
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="wifi-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="clients"
        options={{
          title: 'Clients',
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="phone-portrait-outline" color={color} size={size} />
          ),
        }}
      />
      <Tabs.Screen
        name="more"
        options={{
          title: 'More',
          tabBarIcon: ({ color, size }) => (
            <Ionicons name="ellipsis-horizontal" color={color} size={size} />
          ),
        }}
      />
    </Tabs>
  );
}
