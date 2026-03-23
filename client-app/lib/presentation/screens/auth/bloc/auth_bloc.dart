import 'dart:async';

import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/app_version.dart' as app_version;
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/jwt_util.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/data/data_sources/local/user_local_data_source.dart';
import 'package:gen/domain/usecases/auth/login_usecase.dart';
import 'package:gen/domain/usecases/auth/logout_usecase.dart';
import 'package:gen/domain/usecases/auth/refresh_token_usecase.dart';
import 'package:gen/generated/grpc_pb/auth.pb.dart' as auth_pb;
import 'package:gen/presentation/screens/auth/bloc/auth_event.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';

class AuthBloc extends Bloc<AuthEvent, AuthState> {
  final LoginUseCase loginUseCase;
  final RefreshTokenUseCase refreshTokenUseCase;
  final LogoutUseCase logoutUseCase;
  final UserLocalDataSourceImpl tokenStorage;
  final GrpcChannelManager channelManager;
  final AuthGuard authGuard;

  Timer? _backgroundRefreshTimer;

  AuthBloc({
    required this.loginUseCase,
    required this.refreshTokenUseCase,
    required this.logoutUseCase,
    required this.tokenStorage,
    required this.channelManager,
    required this.authGuard,
  }) : super(const AuthState()) {
    authGuard.setOnSessionExpired(() => add(const AuthLogoutRequested()));
    on<AuthLoginRequested>(_onLoginRequested);
    on<AuthRefreshTokenRequested>(_onRefreshTokenRequested);
    on<AuthRefreshTokenInBackground>(_onRefreshTokenInBackground);
    on<AuthLogoutRequested>(_onLogoutRequested);
    on<AuthClearError>(_onClearError);
    on<AuthCheckRequested>(_onCheckRequested);
    on<AuthClearNeedsUpdate>(_onClearNeedsUpdate);
  }

  @override
  void onTransition(Transition<AuthEvent, AuthState> transition) {
    super.onTransition(transition);
    if (transition.nextState.isAuthenticated) {
      _startBackgroundRefreshTimer();
    } else {
      _cancelBackgroundRefreshTimer();
    }
  }

  @override
  Future<void> close() {
    _cancelBackgroundRefreshTimer();
    return super.close();
  }

  void _startBackgroundRefreshTimer() {
    _cancelBackgroundRefreshTimer();
    _backgroundRefreshTimer = Timer.periodic(
      backgroundRefreshCheckInterval,
      (_) {
        final expiry = getAccessTokenExpiry(tokenStorage.accessToken);
        if (expiry == null) return;
        final now = DateTime.now();
        if (expiry.difference(now) <= accessTokenRefreshThreshold) {
          Logs().d('AuthBloc: время access-токена подходит к концу - фоновый рефреш');
          add(const AuthRefreshTokenInBackground());
        }
      },
    );
  }

  void _cancelBackgroundRefreshTimer() {
    _backgroundRefreshTimer?.cancel();
    _backgroundRefreshTimer = null;
  }

  Future<void> _onCheckRequested(
    AuthCheckRequested event,
    Emitter<AuthState> emit,
  ) async {
    Logs().d('AuthBloc: проверка сессии');
    emit(state.copyWith(isLoading: true, error: null));

    final refreshToken = tokenStorage.refreshToken;
    if (refreshToken == null || refreshToken.isEmpty) {
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: false,
          user: null,
          error: null,
        ),
      );
      return;
    }

    final versionOk = await _checkVersion(channelManager);
    if (!versionOk) {
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: false,
          user: null,
          error: null,
          needsUpdate: true,
        ),
      );
      return;
    }

    const maxAttempts = 3;
    const retryDelay = Duration(milliseconds: 1500);

    Object? lastError;
    var wasUnauthorized = false;

    for (var attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        final tokens = await refreshTokenUseCase(refreshToken);
        tokenStorage.saveTokens(tokens.accessToken, tokens.refreshToken);
        final user = tokenStorage.user;

        if (user == null) {
          tokenStorage.clearTokens();
          emit(
            state.copyWith(
              isLoading: false,
              isAuthenticated: false,
              user: null,
              error: null,
            ),
          );
          return;
        }

        Logs().i('AuthBloc: сессия восстановлена');
        emit(
          state.copyWith(
            isLoading: false,
            isAuthenticated: true,
            user: user,
            error: null,
          ),
        );
        return;
      } catch (e) {
        lastError = e;
        wasUnauthorized = e is UnauthorizedFailure;
        if (wasUnauthorized) {
          Logs().w('AuthBloc: refresh не авторизован');
          break;
        }
        Logs().e('AuthBloc: ошибка проверки сессии', exception: e);
        if (attempt < maxAttempts) {
          await Future<void>.delayed(retryDelay);
        }
      }
    }

    if (wasUnauthorized) {
      tokenStorage.clearTokens();
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: false,
          user: null,
          error: null,
        ),
      );
    } else {
      final user = tokenStorage.user;
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: true,
          user: user,
          error: lastError?.toString().replaceAll('Exception: ', ''),
        ),
      );
    }
  }

  Future<void> _onLoginRequested(
    AuthLoginRequested event,
    Emitter<AuthState> emit,
  ) async {
    Logs().i('AuthBloc: вход username=${event.username}');
    emit(state.copyWith(isLoading: true, error: null));

    try {
      await channelManager.setServer(event.host, event.port);
      final versionOk = await _checkVersion(channelManager);
      if (!versionOk) {
        emit(
          state.copyWith(
            isLoading: false,
            isAuthenticated: false,
            error: null,
            needsUpdate: true,
          ),
        );
        return;
      }
      final result = await loginUseCase(event.username, event.password);

      Logs().i('AuthBloc: вход выполнен успешно');
      tokenStorage.saveTokens(
        result.tokens.accessToken,
        result.tokens.refreshToken,
      );
      tokenStorage.saveUser(result.user);

      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: true,
          user: result.user,
          error: null,
        ),
      );
    } catch (e) {
      if (e is UnauthorizedFailure) {
        Logs().w('AuthBloc: неверные учётные данные');
        tokenStorage.clearTokens();
        emit(
          state.copyWith(
            isLoading: false,
            isAuthenticated: false,
            user: null,
            error: e.toString().replaceAll('Exception: ', ''),
          ),
        );
      } else {
        Logs().e('AuthBloc: ошибка входа', exception: e);
        emit(
          state.copyWith(
            isLoading: false,
            error: e.toString().replaceAll('Exception: ', ''),
          ),
        );
      }
    }
  }

  Future<void> _onRefreshTokenRequested(
    AuthRefreshTokenRequested event,
    Emitter<AuthState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      final tokens = await refreshTokenUseCase(event.refreshToken);

      tokenStorage.saveTokens(
        tokens.accessToken,
        tokens.refreshToken,
      );

      emit(state.copyWith(isLoading: false, error: null));
    } catch (e) {
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: false,
          user: null,
          error: e.toString().replaceAll('Exception: ', ''),
        ),
      );
    }
  }

  Future<void> _onRefreshTokenInBackground(
    AuthRefreshTokenInBackground event,
    Emitter<AuthState> emit,
  ) async {
    final refreshToken = tokenStorage.refreshToken;
    if (refreshToken == null || refreshToken.isEmpty) return;

    try {
      final tokens = await refreshTokenUseCase(refreshToken);
      tokenStorage.saveTokens(tokens.accessToken, tokens.refreshToken);
      Logs().d('AuthBloc: фоновый рефреш токена выполнен');
    } catch (e) {
      if (e is UnauthorizedFailure) {
        Logs().w('AuthBloc: недействительный refresh token при фоновом рефреше');
        tokenStorage.clearTokens();
        emit(
          state.copyWith(
            isLoading: false,
            isAuthenticated: false,
            user: null,
            error: null,
          ),
        );
      }
    }
  }

  Future<void> _onLogoutRequested(
    AuthLogoutRequested event,
    Emitter<AuthState> emit,
  ) async {
    Logs().i('AuthBloc: выход');
    emit(state.copyWith(isLoading: true, error: null));

    try {
      await logoutUseCase();
    } catch (e) {
      Logs().w('AuthBloc: ошибка при выходе', exception: e);
    } finally {
      tokenStorage.clearTokens();
      emit(
        state.copyWith(
          isLoading: false,
          isAuthenticated: false,
          user: null,
          error: null,
        ),
      );
    }
  }

  void _onClearError(AuthClearError event, Emitter<AuthState> emit) {
    emit(state.copyWith(error: null));
  }

  void _onClearNeedsUpdate(AuthClearNeedsUpdate event, Emitter<AuthState> emit) {
    emit(state.copyWith(needsUpdate: false));
  }

  Future<bool> _checkVersion(GrpcChannelManager channelManager) async {
    try {
      final request = auth_pb.CheckVersionRequest(
          clientBuild: app_version.appBuildNumber
      );
      final response = await channelManager.authClientForVersionCheck.checkVersion(request);
      return response.compatible;
    } catch (e) {
      Logs().w('AuthBloc: ошибка проверки версии', exception: e);
      return true;
    }
  }
}
