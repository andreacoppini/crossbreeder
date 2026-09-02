import React from 'react';
import {
  ActivityIndicator,
  Pressable,
  type RefreshControlProps,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
  type StyleProp,
  type TextStyle,
  type ViewStyle,
} from 'react-native';
import * as Haptics from 'expo-haptics';
import { useTheme, toneColors, type StatusTone } from './theme';

/**
 * The shared vocabulary of the app. Screens are assembled from these rather
 * than from raw `View`s, which is what keeps a list of APs and a list of
 * DPSKs looking like they belong to the same product.
 */

export function Screen({
  children,
  scroll,
  refreshControl,
  contentStyle,
}: {
  children: React.ReactNode;
  scroll?: boolean;
  refreshControl?: React.ReactElement<RefreshControlProps>;
  contentStyle?: StyleProp<ViewStyle>;
}) {
  const t = useTheme();
  const base: ViewStyle = { flex: 1, backgroundColor: t.colors.background };
  if (!scroll) return <View style={[base, contentStyle]}>{children}</View>;
  return (
    <ScrollView
      style={base}
      contentContainerStyle={[{ padding: t.space.lg, gap: t.space.md }, contentStyle]}
      refreshControl={refreshControl}
      keyboardShouldPersistTaps="handled"
    >
      {children}
    </ScrollView>
  );
}

export function Card({
  children,
  style,
  padded = true,
}: {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  padded?: boolean;
}) {
  const t = useTheme();
  return (
    <View
      style={[
        {
          backgroundColor: t.colors.surface,
          borderRadius: t.radius.lg,
          borderWidth: StyleSheet.hairlineWidth,
          borderColor: t.colors.border,
          overflow: 'hidden',
        },
        padded && { padding: t.space.lg },
        style,
      ]}
    >
      {children}
    </View>
  );
}

type TypeVariant = keyof ReturnType<typeof useTheme>['typography'];

export function Label({
  children,
  variant = 'body',
  color,
  tone,
  numberOfLines,
  mono: useMono,
  style,
}: {
  children: React.ReactNode;
  variant?: TypeVariant;
  color?: string;
  tone?: StatusTone;
  numberOfLines?: number;
  mono?: boolean;
  style?: StyleProp<TextStyle>;
}) {
  const t = useTheme();
  const toned = tone ? toneColors(t.colors, tone).fg : undefined;
  return (
    <Text
      numberOfLines={numberOfLines}
      style={[
        t.typography[variant],
        { color: color ?? toned ?? t.colors.text },
        useMono && { fontFamily: t.mono },
        style,
      ]}
    >
      {children}
    </Text>
  );
}

export function Muted({
  children,
  variant = 'footnote',
  numberOfLines,
}: {
  children: React.ReactNode;
  variant?: TypeVariant;
  numberOfLines?: number;
}) {
  const t = useTheme();
  return (
    <Label variant={variant} color={t.colors.textSecondary} numberOfLines={numberOfLines}>
      {children}
    </Label>
  );
}

/** A coloured status pill. The app's main carrier of state at a glance. */
export function Pill({
  label,
  tone = 'neutral',
  compact,
}: {
  label: string;
  tone?: StatusTone;
  compact?: boolean;
}) {
  const t = useTheme();
  const { fg, bg } = toneColors(t.colors, tone);
  return (
    <View
      style={{
        backgroundColor: bg,
        paddingHorizontal: compact ? t.space.sm : t.space.md,
        paddingVertical: compact ? 2 : 4,
        borderRadius: t.radius.pill,
        alignSelf: 'flex-start',
      }}
    >
      <Text
        style={[
          t.typography.caption,
          { color: fg, letterSpacing: 0.4, textTransform: 'uppercase' },
        ]}
      >
        {label}
      </Text>
    </View>
  );
}

/** A tappable row, the backbone of every list in the app. */
export function Row({
  title,
  subtitle,
  detail,
  right,
  left,
  onPress,
  tone,
  destructive,
  disabled,
}: {
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  detail?: React.ReactNode;
  right?: React.ReactNode;
  left?: React.ReactNode;
  onPress?: () => void;
  tone?: StatusTone;
  destructive?: boolean;
  disabled?: boolean;
}) {
  const t = useTheme();
  const body = (
    <View
      style={{
        flexDirection: 'row',
        alignItems: 'center',
        gap: t.space.md,
        paddingHorizontal: t.space.lg,
        paddingVertical: t.space.md,
        opacity: disabled ? 0.45 : 1,
      }}
    >
      {tone ? (
        <View
          style={{
            width: 8,
            height: 8,
            borderRadius: 4,
            backgroundColor: toneColors(t.colors, tone).fg,
          }}
        />
      ) : null}
      {left}
      <View style={{ flex: 1, gap: 2 }}>
        {typeof title === 'string' ? (
          <Label
            variant="headline"
            numberOfLines={1}
            color={destructive ? t.colors.down : undefined}
          >
            {title}
          </Label>
        ) : (
          title
        )}
        {typeof subtitle === 'string' ? (
          <Muted numberOfLines={1}>{subtitle}</Muted>
        ) : (
          subtitle
        )}
        {detail}
      </View>
      {right}
    </View>
  );

  if (!onPress || disabled) return body;
  return (
    <Pressable
      onPress={onPress}
      android_ripple={{ color: t.colors.separator }}
      style={({ pressed }) => ({
        backgroundColor: pressed ? t.colors.separator : 'transparent',
      })}
    >
      {body}
    </Pressable>
  );
}

export function Divider() {
  const t = useTheme();
  return (
    <View
      style={{
        height: StyleSheet.hairlineWidth,
        backgroundColor: t.colors.separator,
        marginLeft: t.space.lg,
      }}
    />
  );
}

/** A grouped list, iOS-style: one card, hairlines between rows. */
export function Group({
  header,
  footer,
  children,
}: {
  header?: string;
  footer?: string;
  children: React.ReactNode;
}) {
  const t = useTheme();
  const items = React.Children.toArray(children).filter(Boolean);
  return (
    <View style={{ gap: t.space.sm }}>
      {header ? (
        <Text
          style={[
            t.typography.caption,
            {
              color: t.colors.textSecondary,
              marginLeft: t.space.md,
              letterSpacing: 0.6,
              textTransform: 'uppercase',
            },
          ]}
        >
          {header}
        </Text>
      ) : null}
      <Card padded={false}>
        {items.map((child, i) => (
          <View key={i}>
            {i > 0 ? <Divider /> : null}
            {child}
          </View>
        ))}
      </Card>
      {footer ? <Muted>{footer}</Muted> : null}
    </View>
  );
}

export function Button({
  title,
  onPress,
  variant = 'primary',
  disabled,
  loading,
  haptic = true,
  style,
}: {
  title: string;
  onPress?: () => void;
  variant?: 'primary' | 'secondary' | 'destructive' | 'plain';
  disabled?: boolean;
  loading?: boolean;
  haptic?: boolean;
  style?: StyleProp<ViewStyle>;
}) {
  const t = useTheme();
  const inactive = disabled || loading;

  const scheme = {
    primary: { bg: t.colors.accent, fg: t.colors.onAccent, border: 'transparent' },
    secondary: {
      bg: t.colors.surface,
      fg: t.colors.accent,
      border: t.colors.border,
    },
    destructive: { bg: t.colors.down, fg: '#FFFFFF', border: 'transparent' },
    plain: { bg: 'transparent', fg: t.colors.accent, border: 'transparent' },
  }[variant];

  return (
    <Pressable
      disabled={inactive}
      onPress={() => {
        if (haptic) void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
        onPress?.();
      }}
      style={({ pressed }) => [
        {
          backgroundColor: scheme.bg,
          borderColor: scheme.border,
          borderWidth: variant === 'secondary' ? StyleSheet.hairlineWidth : 0,
          paddingVertical: 13,
          paddingHorizontal: t.space.lg,
          borderRadius: t.radius.md,
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'row',
          gap: t.space.sm,
          opacity: inactive ? 0.5 : pressed ? 0.8 : 1,
        },
        style,
      ]}
    >
      {loading ? <ActivityIndicator size="small" color={scheme.fg} /> : null}
      <Text style={[t.typography.headline, { color: scheme.fg }]}>{title}</Text>
    </Pressable>
  );
}

export function Field({
  label,
  value,
  onChangeText,
  placeholder,
  secure,
  keyboardType,
  autoCapitalize = 'none',
  autoCorrect = false,
  hint,
  error,
  editable = true,
  mono: useMono,
  onSubmitEditing,
  returnKeyType,
}: {
  label: string;
  value: string;
  onChangeText: (next: string) => void;
  placeholder?: string;
  secure?: boolean;
  keyboardType?: React.ComponentProps<typeof TextInput>['keyboardType'];
  autoCapitalize?: React.ComponentProps<typeof TextInput>['autoCapitalize'];
  autoCorrect?: boolean;
  hint?: string;
  error?: string | null;
  editable?: boolean;
  mono?: boolean;
  onSubmitEditing?: () => void;
  returnKeyType?: React.ComponentProps<typeof TextInput>['returnKeyType'];
}) {
  const t = useTheme();
  return (
    <View style={{ gap: t.space.xs }}>
      <Text style={[t.typography.subhead, { color: t.colors.textSecondary }]}>
        {label}
      </Text>
      <TextInput
        value={value}
        onChangeText={onChangeText}
        placeholder={placeholder}
        placeholderTextColor={t.colors.textTertiary}
        secureTextEntry={secure}
        keyboardType={keyboardType}
        autoCapitalize={autoCapitalize}
        autoCorrect={autoCorrect}
        editable={editable}
        onSubmitEditing={onSubmitEditing}
        returnKeyType={returnKeyType}
        style={[
          t.typography.body,
          {
            color: t.colors.text,
            backgroundColor: t.colors.surface,
            borderWidth: StyleSheet.hairlineWidth,
            borderColor: error ? t.colors.down : t.colors.border,
            borderRadius: t.radius.md,
            paddingHorizontal: t.space.md,
            paddingVertical: 11,
            opacity: editable ? 1 : 0.6,
          },
          useMono && { fontFamily: t.mono },
        ]}
      />
      {error ? (
        <Label variant="footnote" tone="down">
          {error}
        </Label>
      ) : hint ? (
        <Muted>{hint}</Muted>
      ) : null}
    </View>
  );
}

/** A labelled value, for detail screens. */
export function Stat({
  label,
  value,
  tone,
  mono: useMono,
}: {
  label: string;
  value: React.ReactNode;
  tone?: StatusTone;
  mono?: boolean;
}) {
  const t = useTheme();
  return (
    <View
      style={{
        flexDirection: 'row',
        alignItems: 'baseline',
        justifyContent: 'space-between',
        gap: t.space.md,
        paddingHorizontal: t.space.lg,
        paddingVertical: 10,
      }}
    >
      <Muted variant="callout">{label}</Muted>
      {typeof value === 'string' || typeof value === 'number' ? (
        <Label variant="callout" tone={tone} mono={useMono} numberOfLines={1}>
          {value}
        </Label>
      ) : (
        value
      )}
    </View>
  );
}

/** A big number with a caption, for the dashboard. */
export function Metric({
  value,
  caption,
  tone = 'neutral',
  onPress,
}: {
  value: React.ReactNode;
  caption: string;
  tone?: StatusTone;
  onPress?: () => void;
}) {
  const t = useTheme();
  const { fg } = toneColors(t.colors, tone);
  const inner = (
    <View style={{ gap: 2, paddingVertical: t.space.sm }}>
      <Text style={[t.typography.largeTitle, { color: fg }]}>{value}</Text>
      <Text style={[t.typography.footnote, { color: t.colors.textSecondary }]}>
        {caption}
      </Text>
    </View>
  );
  if (!onPress) return inner;
  return (
    <Pressable onPress={onPress} style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}>
      {inner}
    </Pressable>
  );
}

export function Loading({ label }: { label?: string }) {
  const t = useTheme();
  return (
    <View
      style={{
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        gap: t.space.md,
        padding: t.space.xl,
      }}
    >
      <ActivityIndicator color={t.colors.accent} />
      {label ? <Muted>{label}</Muted> : null}
    </View>
  );
}

export function EmptyState({
  title,
  message,
  action,
}: {
  title: string;
  message?: string;
  action?: React.ReactNode;
}) {
  const t = useTheme();
  return (
    <View
      style={{
        alignItems: 'center',
        justifyContent: 'center',
        gap: t.space.sm,
        padding: t.space.xl,
      }}
    >
      <Label variant="headline">{title}</Label>
      {message ? (
        <Text
          style={[
            t.typography.callout,
            { color: t.colors.textSecondary, textAlign: 'center' },
          ]}
        >
          {message}
        </Text>
      ) : null}
      {action ? <View style={{ marginTop: t.space.sm }}>{action}</View> : null}
    </View>
  );
}

/** An error the operator can act on, rather than a red string. */
export function ErrorState({
  message,
  onRetry,
  hint,
}: {
  message: string;
  onRetry?: () => void;
  hint?: string;
}) {
  const t = useTheme();
  return (
    <Card style={{ gap: t.space.md }}>
      <Label variant="headline" tone="down">
        Something went wrong
      </Label>
      <Label variant="callout">{message}</Label>
      {hint ? <Muted>{hint}</Muted> : null}
      {onRetry ? <Button title="Try again" variant="secondary" onPress={onRetry} /> : null}
    </Card>
  );
}

/** A horizontal set of filter chips. */
export function ChipBar<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string; tone?: StatusTone }[];
  value: T;
  onChange: (next: T) => void;
}) {
  const t = useTheme();
  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={{ gap: t.space.sm, paddingHorizontal: t.space.lg }}
    >
      {options.map((option) => {
        const active = option.value === value;
        const tone = option.tone ? toneColors(t.colors, option.tone) : null;
        return (
          <Pressable
            key={option.value}
            onPress={() => {
              void Haptics.selectionAsync();
              onChange(option.value);
            }}
            style={{
              paddingHorizontal: t.space.md,
              paddingVertical: 7,
              borderRadius: t.radius.pill,
              backgroundColor: active
                ? (tone?.fg ?? t.colors.accent)
                : t.colors.surface,
              borderWidth: StyleSheet.hairlineWidth,
              borderColor: active ? 'transparent' : t.colors.border,
            }}
          >
            <Text
              style={[
                t.typography.subhead,
                { color: active ? t.colors.onAccent : t.colors.textSecondary },
              ]}
            >
              {option.label}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}
