import 'dart:async';

import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/grpc_unavailable.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/domain/usecases/chat/connect_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_event.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_state.dart';

const Duration _serverPollInterval = Duration(seconds: 45);

class AppTopNoticeBloc extends Bloc<AppTopNoticeEvent, AppTopNoticeState> {
  AppTopNoticeBloc({
    required ConnectUseCase connectUseCase,
    required AuthBloc authBloc,
  })  : _connect = connectUseCase,
      _authBloc = authBloc,
      super(const AppTopNoticeState()) {
    on<AppTopNoticeShow>(_onShow);
    on<AppTopNoticeDismissToast>(_onDismissToast);
    on<AppTopNoticeSetCollapsed>(_onSetCollapsed);
    on<AppTopNoticeServerPing>(_onServerPing);
    on<AppTopNoticeManualServerCheck>(_onManualServerCheck);
    on<AppTopNoticeOfflineCountdownTick>(_onOfflineCountdownTick);
    on<AppTopNoticeReportUnreachable>(_onReportUnreachable);
    on<AppTopNoticeAuthChanged>(_onAuthChanged);

    _authSubscription = _authBloc.stream.listen(
      (auth) => add(AppTopNoticeAuthChanged(auth)),
    );
    add(AppTopNoticeAuthChanged(_authBloc.state));
  }

  final ConnectUseCase _connect;
  final AuthBloc _authBloc;

  StreamSubscription<AuthState>? _authSubscription;
  Timer? _toastTimer;
  Timer? _serverPollTimer;
  Timer? _offlineCountdownTimer;
  bool _serverPingInFlight = false;

  @override
  Future<void> close() {
    _toastTimer?.cancel();
    _serverPollTimer?.cancel();
    _offlineCountdownTimer?.cancel();
    _authSubscription?.cancel();
    return super.close();
  }

  void _cancelOfflineCountdownTimer() {
    _offlineCountdownTimer?.cancel();
    _offlineCountdownTimer = null;
  }

  void _restartOfflineCountdown(Emitter<AppTopNoticeState> emit) {
    _cancelOfflineCountdownTimer();
    emit(state.copyWith(serverOfflineCountdownSeconds: 15));
    _offlineCountdownTimer = Timer.periodic(
      const Duration(seconds: 1),
      (_) => add(const AppTopNoticeOfflineCountdownTick()),
    );
  }

  void _syncOfflineCountdownAfterUnreachable(
    Emitter<AppTopNoticeState> emit, {
    required bool preserveExistingCountdown,
  }) {
    if (state.toastMessage != null) {
      _cancelOfflineCountdownTimer();
      emit(state.copyWith(clearServerOfflineCountdown: true));
      return;
    }

    final hasRunning = state.serverOfflineCountdownSeconds != null || _offlineCountdownTimer != null;

    if (preserveExistingCountdown && hasRunning) {
      return;
    }

    _restartOfflineCountdown(emit);
  }

  void _stopOfflineCountdownUi(Emitter<AppTopNoticeState> emit) {
    _cancelOfflineCountdownTimer();
    emit(state.copyWith(clearServerOfflineCountdown: true));
  }

  void _onAuthChanged(
    AppTopNoticeAuthChanged event,
    Emitter<AppTopNoticeState> emit,
  ) {
    final auth = event.auth;
    _toastTimer?.cancel();
    _toastTimer = null;
    _serverPollTimer?.cancel();
    _serverPollTimer = null;
    _cancelOfflineCountdownTimer();

    if (!auth.isAuthenticated) {
      emit(
        state.copyWith(
          clearToast: auth.error == null,
          serverLink: AppTopNoticeServerLink.unknown,
          topNoticeCollapsed: false,
          clearServerOfflineCountdown: true,
          serverCheckInFlight: false,
        ),
      );
      return;
    }

    add(const AppTopNoticeServerPing());
    _serverPollTimer = Timer.periodic(_serverPollInterval, (_) {
      add(const AppTopNoticeServerPing(preserveOfflineCountdown: true));
    });
  }

  AppTopNoticeLevel _resolveLevel(AppTopNoticeShow event) {
    if (event.level != null) {
      return event.level!;
    }

    return event.error ? AppTopNoticeLevel.error : AppTopNoticeLevel.info;
  }

  Future<void> _onShow(
    AppTopNoticeShow event,
    Emitter<AppTopNoticeState> emit,
  ) async {
    if (isServerUnreachableToastText(event.message)) {
      add(const AppTopNoticeReportUnreachable());
      return;
    }
    _toastTimer?.cancel();
    _toastTimer = null;
    _cancelOfflineCountdownTimer();
    final level = _resolveLevel(event);

    final Duration? dismissAfter;
    if (!event.autoDismiss) {
      dismissAfter = null;
    } else if (event.duration != null) {
      dismissAfter = event.duration!.inMilliseconds > 0 ? event.duration : null;
    } else {
      dismissAfter = const Duration(seconds: 4);
    }

    emit(
      state.copyWith(
        toastMessage: event.message,
        toastLevel: level,
        toastAction: event.toastAction,
        clearToast: false,
        topNoticeCollapsed: false,
        clearServerOfflineCountdown: true,
      ),
    );

    if (dismissAfter != null) {
      _toastTimer = Timer(dismissAfter, () {
        if (!isClosed) {
          add(const AppTopNoticeDismissToast());
        }
      });
    }
  }

  void _onDismissToast(
    AppTopNoticeDismissToast event,
    Emitter<AppTopNoticeState> emit,
  ) {
    _toastTimer?.cancel();
    _toastTimer = null;
    emit(state.copyWith(clearToast: true));

    if (state.serverLink == AppTopNoticeServerLink.unreachable) {
      _syncOfflineCountdownAfterUnreachable(emit, preserveExistingCountdown: false);
    }
  }

  void _onSetCollapsed(
    AppTopNoticeSetCollapsed event,
    Emitter<AppTopNoticeState> emit,
  ) {
    if (!state.hasVisibleNotice) {
      return;
    }
    emit(state.copyWith(topNoticeCollapsed: event.collapsed));
  }

  void _onManualServerCheck(
    AppTopNoticeManualServerCheck event,
    Emitter<AppTopNoticeState> emit,
  ) {
    if (state.serverCheckInFlight || _serverPingInFlight) {
      return;
    }
    _cancelOfflineCountdownTimer();
    emit(state.copyWith(clearServerOfflineCountdown: true));
    add(const AppTopNoticeServerPing());
  }

  void _onReportUnreachable(
    AppTopNoticeReportUnreachable event,
    Emitter<AppTopNoticeState> emit,
  ) {
    if (!_authBloc.state.isAuthenticated) {
      return;
    }
    _toastTimer?.cancel();
    _toastTimer = null;
    emit(
      state.copyWith(
        clearToast: true,
        serverLink: AppTopNoticeServerLink.unreachable,
        serverCheckInFlight: false,
      ),
    );
    _syncOfflineCountdownAfterUnreachable(emit, preserveExistingCountdown: false);
  }

  void _onOfflineCountdownTick(
    AppTopNoticeOfflineCountdownTick event,
    Emitter<AppTopNoticeState> emit,
  ) {
    if (state.serverCheckInFlight || _serverPingInFlight) {
      return;
    }

    final sec = state.serverOfflineCountdownSeconds;
    if (sec == null) {
      _cancelOfflineCountdownTimer();
      return;
    }

    if (sec <= 1) {
      _cancelOfflineCountdownTimer();
      emit(state.copyWith(clearServerOfflineCountdown: true));
      add(const AppTopNoticeServerPing());
      return;
    }

    emit(state.copyWith(serverOfflineCountdownSeconds: sec - 1));
  }

  Future<void> _onServerPing(
    AppTopNoticeServerPing event,
    Emitter<AppTopNoticeState> emit,
  ) async {
    if (!_authBloc.state.isAuthenticated) {
      return;
    }

    if (_serverPingInFlight) {
      return;
    }

    _serverPingInFlight = true;
    emit(state.copyWith(serverCheckInFlight: true));

    try {
      final ok = await _connect();
      if (isClosed) {
        return;
      }

      emit(
        state.copyWith(
          serverLink: ok
              ? AppTopNoticeServerLink.reachable
              : AppTopNoticeServerLink.unreachable,
          serverCheckInFlight: false,
        ),
      );

      if (ok) {
        _stopOfflineCountdownUi(emit);
      } else {
        _syncOfflineCountdownAfterUnreachable(
          emit,
          preserveExistingCountdown: event.preserveOfflineCountdown,
        );
      }
    } catch (e, st) {
      Logs().w('AppTopNoticeBloc: server ping failed', exception: e, stackTrace: st);
      if (!isClosed) {
        emit(
          state.copyWith(
            serverLink: AppTopNoticeServerLink.unreachable,
            serverCheckInFlight: false,
          ),
        );
        _syncOfflineCountdownAfterUnreachable(
          emit,
          preserveExistingCountdown: event.preserveOfflineCountdown,
        );
      }
    } finally {
      _serverPingInFlight = false;
    }
  }
}
