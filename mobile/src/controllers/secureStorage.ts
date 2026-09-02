import { Platform } from 'react-native';
import AsyncStorage from '@react-native-async-storage/async-storage';
import * as SecureStore from 'expo-secure-store';

/**
 * Secrets go to the Keychain on iOS and the Keystore on Android.
 *
 * On web there is no equivalent, and this project treats web as a
 * development convenience rather than a place to hold an admin password, so
 * the fallback is deliberately loud rather than silent.
 */
const isWeb = Platform.OS === 'web';

export async function setSecret(key: string, value: string): Promise<void> {
  if (isWeb) {
    console.warn(
      `[secureStorage] No hardware-backed store on web; "${key}" is being held in ordinary storage. Do not use a production controller from a web build.`,
    );
    await AsyncStorage.setItem(`insecure:${key}`, value);
    return;
  }
  await SecureStore.setItemAsync(key, value, {
    keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
  });
}

export async function getSecret(key: string): Promise<string | null> {
  if (isWeb) return AsyncStorage.getItem(`insecure:${key}`);
  try {
    return await SecureStore.getItemAsync(key);
  } catch {
    // A restored backup, or a device whose keychain entry did not survive a
    // reinstall. Treat as absent; the operator signs in again.
    return null;
  }
}

export async function deleteSecret(key: string): Promise<void> {
  if (isWeb) {
    await AsyncStorage.removeItem(`insecure:${key}`);
    return;
  }
  try {
    await SecureStore.deleteItemAsync(key);
  } catch {
    // Already gone.
  }
}
