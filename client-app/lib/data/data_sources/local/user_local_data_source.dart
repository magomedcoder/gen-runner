import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:gen/domain/entities/user.dart';
import 'package:shared_preferences/shared_preferences.dart';

abstract class UserLocalDataSource {
  void saveTokens(String accessToken, String refreshToken);

  void saveUser(User user);

  void clearTokens();

  ThemeMode get themeMode;

  Future<void> setThemeMode(ThemeMode mode);
}

class UserLocalDataSourceImpl implements UserLocalDataSource {
  static const _keyAccessToken = 'gen_access_token';
  static const _keyRefreshToken = 'gen_refresh_token';
  static const _keyUser = 'gen_user';
  static const _keyThemeMode = 'gen_theme_mode';

  SharedPreferences? _prefs;
  String? _accessToken;
  String? _refreshToken;
  User? _user;

  String? get accessToken => _accessToken;

  String? get refreshToken => _refreshToken;

  User? get user => _user;

  bool get hasToken => _accessToken != null && _accessToken!.isNotEmpty;

  @override
  ThemeMode get themeMode {
    final name = _prefs?.getString(_keyThemeMode) ?? 'dark';
    return name == 'light' ? ThemeMode.light : ThemeMode.dark;
  }

  Future<void> init() async {
    _prefs ??= await SharedPreferences.getInstance();
    _accessToken = _prefs!.getString(_keyAccessToken);
    _refreshToken = _prefs!.getString(_keyRefreshToken);
    final userJson = _prefs!.getString(_keyUser);
    if (userJson != null) {
      try {
        _user = User.fromJson(jsonDecode(userJson) as Map<String, dynamic>);
      } catch (_) {
        _user = null;
      }
    } else {
      _user = null;
    }
  }

  @override
  void saveTokens(String accessToken, String refreshToken) {
    _accessToken = accessToken;
    _refreshToken = refreshToken;
    _prefs?.setString(_keyAccessToken, accessToken);
    _prefs?.setString(_keyRefreshToken, refreshToken);
  }

  @override
  void saveUser(User user) {
    _user = user;
    _prefs?.setString(_keyUser, jsonEncode(user.toJson()));
  }

  @override
  void clearTokens() {
    _accessToken = null;
    _refreshToken = null;
    _user = null;
    _prefs?.remove(_keyAccessToken);
    _prefs?.remove(_keyRefreshToken);
    _prefs?.remove(_keyUser);
  }

  @override
  Future<void> setThemeMode(ThemeMode mode) async {
    await _prefs?.setString(_keyThemeMode, mode == ThemeMode.dark ? 'dark' : 'light');
  }
}
