import 'package:equatable/equatable.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_toast_action.dart';

abstract class AppTopNoticeEvent extends Equatable {
  const AppTopNoticeEvent();

  @override
  List<Object?> get props => [];
}

class AppTopNoticeAuthChanged extends AppTopNoticeEvent {
  const AppTopNoticeAuthChanged(this.auth);

  final AuthState auth;

  @override
  List<Object?> get props => [auth];
}

class AppTopNoticeShow extends AppTopNoticeEvent {
  const AppTopNoticeShow(
    this.message, {
    this.error = false,
    this.level,
    this.duration,
    this.toastAction = AppTopNoticeToastAction.none,
    this.autoDismiss = true,
  });

  final String message;
  final bool error;
  final AppTopNoticeLevel? level;
  final Duration? duration;
  final AppTopNoticeToastAction toastAction;
  final bool autoDismiss;

  @override
  List<Object?> get props => [message, error, level, duration, toastAction, autoDismiss];
}

class AppTopNoticeDismissToast extends AppTopNoticeEvent {
  const AppTopNoticeDismissToast();
}

class AppTopNoticeSetCollapsed extends AppTopNoticeEvent {
  const AppTopNoticeSetCollapsed(this.collapsed);

  final bool collapsed;

  @override
  List<Object?> get props => [collapsed];
}

class AppTopNoticeServerPing extends AppTopNoticeEvent {
  const AppTopNoticeServerPing({this.preserveOfflineCountdown = false});

  final bool preserveOfflineCountdown;

  @override
  List<Object?> get props => [preserveOfflineCountdown];
}

class AppTopNoticeManualServerCheck extends AppTopNoticeEvent {
  const AppTopNoticeManualServerCheck();
}

class AppTopNoticeOfflineCountdownTick extends AppTopNoticeEvent {
  const AppTopNoticeOfflineCountdownTick();
}

class AppTopNoticeReportUnreachable extends AppTopNoticeEvent {
  const AppTopNoticeReportUnreachable();
}
