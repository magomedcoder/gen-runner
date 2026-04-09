import 'package:equatable/equatable.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_toast_action.dart';

enum AppTopNoticeServerLink {
  unknown,
  reachable,
  unreachable,
}

class AppTopNoticeState extends Equatable {
  const AppTopNoticeState({
    this.toastMessage,
    this.toastLevel = AppTopNoticeLevel.info,
    this.toastAction = AppTopNoticeToastAction.none,
    this.serverLink = AppTopNoticeServerLink.unknown,
    this.topNoticeCollapsed = false,
    this.serverOfflineCountdownSeconds,
    this.serverCheckInFlight = false,
  });

  final String? toastMessage;
  final AppTopNoticeLevel toastLevel;
  final AppTopNoticeToastAction toastAction;
  final AppTopNoticeServerLink serverLink;
  final bool topNoticeCollapsed;
  final int? serverOfflineCountdownSeconds;

  final bool serverCheckInFlight;

  bool get showServerOfflineBanner => toastMessage == null && serverLink == AppTopNoticeServerLink.unreachable;

  bool get hasVisibleNotice {
    if (toastMessage != null) {
      return true;
    }

    return showServerOfflineBanner;
  }

  AppTopNoticeLevel get _serverLevel => showServerOfflineBanner ? AppTopNoticeLevel.error : AppTopNoticeLevel.info;

  AppTopNoticeLevel get effectiveNoticeLevel {
    final toastL = toastMessage != null ? toastLevel : null;
    final serverL = showServerOfflineBanner ? _serverLevel : null;
    final parts = [toastL, serverL].whereType<AppTopNoticeLevel>();
    if (parts.isEmpty) {
      return AppTopNoticeLevel.info;
    }

    return parts.reduce(
      (a, b) => a.weight >= b.weight ? a : b,
    );
  }

  AppTopNoticeState copyWith({
    String? toastMessage,
    bool clearToast = false,
    AppTopNoticeLevel? toastLevel,
    AppTopNoticeToastAction? toastAction,
    AppTopNoticeServerLink? serverLink,
    bool? topNoticeCollapsed,
    int? serverOfflineCountdownSeconds,
    bool clearServerOfflineCountdown = false,
    bool? serverCheckInFlight,
  }) {
    return AppTopNoticeState(
      toastMessage: clearToast ? null : (toastMessage ?? this.toastMessage),
      toastLevel: clearToast
        ? AppTopNoticeLevel.info
        : (toastLevel ?? this.toastLevel),
      toastAction: clearToast
        ? AppTopNoticeToastAction.none
        : (toastAction ?? this.toastAction),
      serverLink: serverLink ?? this.serverLink,
      topNoticeCollapsed: topNoticeCollapsed ?? this.topNoticeCollapsed,
      serverOfflineCountdownSeconds: clearServerOfflineCountdown
        ? null
        : (serverOfflineCountdownSeconds ?? this.serverOfflineCountdownSeconds),
      serverCheckInFlight: serverCheckInFlight ?? this.serverCheckInFlight,
    );
  }

  @override
  List<Object?> get props => [
    toastMessage,
    toastLevel,
    toastAction,
    serverLink,
    topNoticeCollapsed,
    serverOfflineCountdownSeconds,
    serverCheckInFlight,
  ];
}
