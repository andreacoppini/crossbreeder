import { Platform, useColorScheme } from 'react-native';

/**
 * One palette, two schemes.
 *
 * The colours are deliberately restrained: this is a tool that gets used at
 * the top of a ladder in a plant room, so the work is done by contrast and by
 * the status colours, not by decoration. Status colours are the same three
 * everywhere (up, degraded, down) so an operator reads them without thinking.
 */

export interface Palette {
  background: string;
  /** Cards and grouped rows sitting on `background`. */
  surface: string;
  surfaceElevated: string;
  border: string;
  separator: string;

  text: string;
  textSecondary: string;
  textTertiary: string;

  accent: string;
  accentMuted: string;
  onAccent: string;

  /** Online, healthy, connected. */
  up: string;
  upMuted: string;
  /** Flagged, degraded, warning. */
  warn: string;
  warnMuted: string;
  /** Offline, failed, critical. */
  down: string;
  downMuted: string;
  /** Unknown or not applicable. */
  neutral: string;
  neutralMuted: string;
}

const light: Palette = {
  background: '#F2F2F7',
  surface: '#FFFFFF',
  surfaceElevated: '#FFFFFF',
  border: '#D8D8DE',
  separator: '#E4E4EA',

  text: '#11131A',
  textSecondary: '#5B6070',
  textTertiary: '#8A8F9E',

  accent: '#0B6BCB',
  accentMuted: '#E3EFFB',
  onAccent: '#FFFFFF',

  up: '#1D7A45',
  upMuted: '#E1F3E8',
  warn: '#9A6100',
  warnMuted: '#FBF0DC',
  down: '#B3261E',
  downMuted: '#FBE4E2',
  neutral: '#6A7080',
  neutralMuted: '#EAEBEF',
};

const dark: Palette = {
  background: '#0C0D11',
  surface: '#16181F',
  surfaceElevated: '#1E212A',
  border: '#2C303B',
  separator: '#23262F',

  text: '#F2F3F7',
  textSecondary: '#A2A8B8',
  textTertiary: '#727888',

  accent: '#4F9DF7',
  accentMuted: '#14273D',
  onAccent: '#06121F',

  up: '#4FC37E',
  upMuted: '#12291C',
  warn: '#E0A33C',
  warnMuted: '#2C2110',
  down: '#F2726A',
  downMuted: '#31191A',
  neutral: '#8C93A3',
  neutralMuted: '#22252E',
};

/** 4pt grid. Every gap and inset in the app comes from here. */
export const space = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
} as const;

export const radius = {
  sm: 6,
  md: 10,
  lg: 14,
  pill: 999,
} as const;

export const typography = {
  largeTitle: { fontSize: 32, lineHeight: 38, fontWeight: '700' as const },
  title: { fontSize: 22, lineHeight: 28, fontWeight: '700' as const },
  headline: { fontSize: 17, lineHeight: 22, fontWeight: '600' as const },
  body: { fontSize: 16, lineHeight: 21, fontWeight: '400' as const },
  callout: { fontSize: 15, lineHeight: 20, fontWeight: '400' as const },
  subhead: { fontSize: 14, lineHeight: 19, fontWeight: '500' as const },
  footnote: { fontSize: 13, lineHeight: 17, fontWeight: '400' as const },
  caption: { fontSize: 11, lineHeight: 14, fontWeight: '600' as const },
} as const;

/**
 * A monospaced face for the things that must line up and must not be
 * misread: MAC addresses, IP addresses, passphrases, channel numbers.
 */
export const mono = Platform.select({
  ios: 'Menlo',
  android: 'monospace',
  default: 'monospace',
}) as string;

export interface Theme {
  colors: Palette;
  scheme: 'light' | 'dark';
  space: typeof space;
  radius: typeof radius;
  typography: typeof typography;
  mono: string;
}

export function useTheme(): Theme {
  const scheme = useColorScheme() === 'dark' ? 'dark' : 'light';
  return {
    colors: scheme === 'dark' ? dark : light,
    scheme,
    space,
    radius,
    typography,
    mono,
  };
}

export const palettes = { light, dark };

/** Map a device or client status onto the three status colours. */
export type StatusTone = 'up' | 'warn' | 'down' | 'neutral' | 'accent';

export function toneColors(colors: Palette, tone: StatusTone) {
  switch (tone) {
    case 'up':
      return { fg: colors.up, bg: colors.upMuted };
    case 'warn':
      return { fg: colors.warn, bg: colors.warnMuted };
    case 'down':
      return { fg: colors.down, bg: colors.downMuted };
    case 'accent':
      return { fg: colors.accent, bg: colors.accentMuted };
    default:
      return { fg: colors.neutral, bg: colors.neutralMuted };
  }
}

/** The tone for an AP's status string, as SmartZone spells them. */
export function apStatusTone(status?: string): StatusTone {
  switch (status) {
    case 'Online':
    case 'Connect':
      return 'up';
    case 'Flagged':
    case 'Discovery':
    case 'Provisioned':
    case 'RebootRequired':
      return 'warn';
    case 'Offline':
    case 'Disconnect':
      return 'down';
    default:
      return 'neutral';
  }
}

/** The tone for an alarm or event severity. */
export function severityTone(severity?: string): StatusTone {
  switch (severity) {
    case 'Critical':
    case 'Major':
      return 'down';
    case 'Minor':
    case 'Warning':
      return 'warn';
    case 'Info':
      return 'neutral';
    default:
      return 'neutral';
  }
}
