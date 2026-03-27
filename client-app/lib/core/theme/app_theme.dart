import 'package:flutter/material.dart';

class AppTheme {
  AppTheme._();

  static const Color _accent = Color(0xFF30363D);
  static const Color _background = Color(0xFF121212);
  static const Color _surface = Color(0xFF1E1E1E);
  static const Color _surfaceHigh = Color(0xFF2A2A2A);
  static const Color _text = Color(0xFFE6EDF3);
  static const Color _textMuted = Color(0xFF9FA6AD);
  static const Color _error = Color(0xFFFFB4AB);

  static ThemeData get dark {
    final scheme = ColorScheme.fromSeed(
      seedColor: _accent,
      brightness: Brightness.dark,
    ).copyWith(
      primary: _accent,
      onPrimary: _text,
      primaryContainer: _surfaceHigh,
      onPrimaryContainer: _text,
      secondary: _accent,
      onSecondary: _text,
      secondaryContainer: _surfaceHigh,
      onSecondaryContainer: _text,
      tertiary: _accent,
      surface: _surface,
      onSurface: _text,
      onSurfaceVariant: _textMuted,
      surfaceContainerLow: _surface,
      surfaceContainer: _surface,
      surfaceContainerHigh: _surfaceHigh,
      surfaceContainerHighest: _surfaceHigh,
      outline: _textMuted.withValues(alpha: 0.34),
      outlineVariant: _textMuted.withValues(alpha: 0.2),
      error: _error,
      onError: const Color(0xFF690005),
      errorContainer: const Color(0xFF93000A),
      onErrorContainer: const Color(0xFFFFDAD6),
      surfaceTint: _accent,
    );

    final buttonGlass = Colors.white.withValues(alpha: 0.08);
    final buttonGlassHover = Colors.white.withValues(alpha: 0.12);
    final buttonGlassPressed = Colors.white.withValues(alpha: 0.16);
    final buttonGlassDisabled = Colors.white.withValues(alpha: 0.05);

    final glassButtonStyle = ButtonStyle(
      elevation: const WidgetStatePropertyAll(0),
      shadowColor: const WidgetStatePropertyAll(Colors.transparent),
      surfaceTintColor: const WidgetStatePropertyAll(Colors.transparent),
      backgroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) {
            return buttonGlassDisabled;
        }

        if (states.contains(WidgetState.pressed)) {
            return buttonGlassPressed;
        }

        if (states.contains(WidgetState.hovered)) {
            return buttonGlassHover;
        }

        return buttonGlass;
      }),
      foregroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.disabled)) {
          return scheme.onSurface.withValues(alpha: 0.38);
        }

        return scheme.onSurface;
      }),
      overlayColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.pressed)) {
          return scheme.onSurface.withValues(alpha: 0.06);
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
      scaffoldBackgroundColor: _background,
      dialogTheme: DialogThemeData(
        backgroundColor: scheme.surfaceContainer,
      ),
      filledButtonTheme: FilledButtonThemeData(style: glassButtonStyle),
      elevatedButtonTheme: ElevatedButtonThemeData(style: glassButtonStyle),
      outlinedButtonTheme: OutlinedButtonThemeData(style: outlinedGlassStyle),
      iconButtonTheme: IconButtonThemeData(
        style: IconButton.styleFrom(
          foregroundColor: scheme.onSurfaceVariant,
          disabledForegroundColor: scheme.onSurfaceVariant.withValues(alpha: 0.38),
          hoverColor: buttonGlass,
          highlightColor: buttonGlassPressed,
          focusColor: buttonGlassHover,
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: IconButton.styleFrom(
          foregroundColor: scheme.onSurfaceVariant,
          disabledForegroundColor: scheme.onSurfaceVariant.withValues(alpha: 0.38),
          hoverColor: buttonGlass,
          highlightColor: buttonGlassPressed,
          focusColor: buttonGlassHover,
        ),
      ),
      navigationRailTheme: NavigationRailThemeData(
        backgroundColor: scheme.surfaceContainerLow,
        indicatorColor: buttonGlass,
        selectedIconTheme: IconThemeData(color: scheme.onSurface),
        selectedLabelTextStyle: TextStyle(
          color: scheme.onSurface,
          fontSize: 12,
          fontWeight: FontWeight.w500,
          fontFamily: 'Inter',
        ),
        unselectedIconTheme: IconThemeData(color: scheme.onSurfaceVariant),
        unselectedLabelTextStyle: TextStyle(
          color: scheme.onSurfaceVariant,
          fontSize: 12,
          fontFamily: 'Inter',
        ),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: scheme.surfaceContainerLow,
        indicatorColor: buttonGlass,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        labelTextStyle: WidgetStateProperty.resolveWith((states) {
          final selected = states.contains(WidgetState.selected);
          return TextStyle(
            color: selected ? scheme.onSurface : scheme.onSurfaceVariant,
            fontSize: 12,
            fontWeight: selected ? FontWeight.w500 : FontWeight.normal,
            fontFamily: 'Inter',
          );
        }),
        iconTheme: WidgetStateProperty.resolveWith((states) {
          final selected = states.contains(WidgetState.selected);
          return IconThemeData(
            color: selected ? scheme.onSurface : scheme.onSurfaceVariant,
          );
        }),
      ),
      inputDecorationTheme: InputDecorationTheme(
        floatingLabelStyle: TextStyle(
          color: _textMuted,
        ),
      ),
    );
  }
}
