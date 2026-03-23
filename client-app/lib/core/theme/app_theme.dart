import 'package:flutter/material.dart';

class AppTheme {
  AppTheme._();

  static const Color _surface = Color(0xFF232831);
  static const Color _surfaceLow = Color(0xFF282E36);
  static const Color _surfaceMid = Color(0xFF2C333C);
  static const Color _surfaceHigh = Color(0xFF363E48);
  static const Color _surfaceHighest = Color(0xFF414953);

  static const Color _primaryMuted = Color(0xFF8FA3B2);
  static const Color _secondaryMuted = Color(0xFF7D8E9C);

  static const Color _buttonGlass = Color(0x14FFFFFF);
  static const Color _buttonGlassHover = Color(0x1EFFFFFF);
  static const Color _buttonGlassPressed = Color(0x26FFFFFF);
  static const Color _buttonGlassDisabled = Color(0x0CFFFFFF);

  static ThemeData get dark {
    final scheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFF5F7382),
      brightness: Brightness.dark,
    ).copyWith(
        primary: _primaryMuted,
        onPrimary: const Color(0xFF0D0F12),
        primaryContainer: const Color(0xFF2A3138),
        onPrimaryContainer: const Color(0xFFD1DADF),
        secondary: _secondaryMuted,
        onSecondary: const Color(0xFF0D0F12),
        secondaryContainer: const Color(0xFF262C33),
        onSecondaryContainer: const Color(0xFFC8D0D8),
        tertiary: const Color(0xFF6F808E),
        surface: _surface,
        onSurface: const Color(0xFFE0E4E9),
        onSurfaceVariant: const Color(0xFFA8B0BA),
        surfaceContainerLow: _surfaceLow,
        surfaceContainer: _surfaceMid,
        surfaceContainerHigh: _surfaceHigh,
        surfaceContainerHighest: _surfaceHighest,
        outline: const Color(0xFF4A5159),
        outlineVariant: const Color(0xFF343A42),
        error: const Color(0xFFD88A91),
        onError: const Color(0xFF1A0F10),
        errorContainer: const Color(0xFF3D2A2C),
        onErrorContainer: const Color(0xFFFFD6D8),
        surfaceTint: _primaryMuted,
      );

    final glassButtonStyle = ButtonStyle(
      elevation: const WidgetStatePropertyAll(0),
      shadowColor: const WidgetStatePropertyAll(Colors.transparent),
      surfaceTintColor: const WidgetStatePropertyAll(Colors.transparent),
      backgroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) {
            return _buttonGlassDisabled;
        }

        if (states.contains(WidgetState.pressed)) {
            return _buttonGlassPressed;
        }

        if (states.contains(WidgetState.hovered)) {
            return _buttonGlassHover;
        }

        return _buttonGlass;
      }),
      foregroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) {
          return scheme.onSurface.withValues(alpha: 0.38);
        }

        return scheme.onSurface;
      }),
      overlayColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.pressed)) {
          return Colors.white.withValues(alpha: 0.06);
        }

        return Colors.transparent;
      }),
    );

    final outlinedGlassStyle = glassButtonStyle.copyWith(
      side: WidgetStatePropertyAll(
        BorderSide(color: scheme.outline.withValues(alpha: 0.28)),
      ),
    );

    return ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      fontFamily: 'Inter',
      scaffoldBackgroundColor: _surface,
      dialogTheme: DialogThemeData(
        backgroundColor: _surfaceHigh,
      ),
      filledButtonTheme: FilledButtonThemeData(style: glassButtonStyle),
      elevatedButtonTheme: ElevatedButtonThemeData(style: glassButtonStyle),
      outlinedButtonTheme: OutlinedButtonThemeData(style: outlinedGlassStyle),
      iconButtonTheme: IconButtonThemeData(
        style: IconButton.styleFrom(
          foregroundColor: scheme.onSurfaceVariant,
          disabledForegroundColor: scheme.onSurfaceVariant.withValues(alpha: 0.38),
          hoverColor: _buttonGlass,
          highlightColor: _buttonGlassPressed,
          focusColor: _buttonGlassHover,
        ),
      ),
    );
  }
}
