import React from 'react';
import { Redirect, Tabs } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';
import { useControllers } from '@/controllers/ControllerProvider';
import { useTheme } from '@/ui/theme';

/**
 * Four tabs and a More.
 *
 * The four are the things an operator opens while standing in front of a
 * problem; everything else — DPSKs, zones, alarms, diagnostics, and switching
 * when it lands — lives one level down under More, where it can grow without
 * the tab bar turning into a menu.
 */
export default function TabsLayout() {
  const t = useTheme();
  const { state } = useControllers();

  if (state !== 'connected') return <Redirect href="/" />;

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
