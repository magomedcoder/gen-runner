import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/grpc_unavailable.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_bloc.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_event.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_state.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_toast_action.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/chat_runner_issue_notice.dart';

export 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';
export 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_toast_action.dart';

final GlobalKey<NavigatorState> appNavigatorKey = GlobalKey<NavigatorState>();

void showAppTopNotice(
  String message, {
  bool error = false,
  AppTopNoticeLevel? level,
  Duration? duration,
  AppTopNoticeToastAction toastAction = AppTopNoticeToastAction.none,
  bool autoDismiss = true,
}) {
  if (isServerUnreachableToastText(message)) {
    reportAppServerUnreachable();
    return;
  }
  final persist =
      isPersistentServerAvailabilityTopNoticeText(message) ||
      isChatRunnerIssueTopNoticePersistentText(message);
  final effectiveAutoDismiss = autoDismiss && !persist;
  sl<AppTopNoticeBloc>().add(
    AppTopNoticeShow(
      message,
      error: error,
      level: level,
      duration: duration,
      toastAction: toastAction,
      autoDismiss: effectiveAutoDismiss,
    ),
  );
}

void reportAppServerUnreachable() {
  sl<AppTopNoticeBloc>().add(const AppTopNoticeReportUnreachable());
}

void dismissAppTopNoticeToast() {
  sl<AppTopNoticeBloc>().add(const AppTopNoticeDismissToast());
}

class AppTopNoticeOverlay extends StatelessWidget {
  const AppTopNoticeOverlay({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      clipBehavior: Clip.none,
      children: [
        child,
        BlocBuilder<AppTopNoticeBloc, AppTopNoticeState>(
          buildWhen: (prev, next) => prev.toastMessage != next.toastMessage || prev.toastLevel != next.toastLevel || prev.toastAction != next.toastAction || prev.serverLink != next.serverLink || prev.topNoticeCollapsed != next.topNoticeCollapsed || prev.serverOfflineCountdownSeconds != next.serverOfflineCountdownSeconds || prev.serverCheckInFlight != next.serverCheckInFlight,
          builder: (context, state) {
            if (!state.hasVisibleNotice) {
              return const SizedBox.shrink();
            }

            final padding = MediaQuery.paddingOf(context);
            final bloc = context.read<AppTopNoticeBloc>();

            if (state.topNoticeCollapsed) {
              return Align(
                alignment: Alignment.topCenter,
                child: Padding(
                  padding: EdgeInsets.only(top: padding.top + 8),
                  child: _CollapsedNoticeChip(
                    level: state.effectiveNoticeLevel,
                    onExpand: () {
                      bloc.add(const AppTopNoticeSetCollapsed(false));
                    },
                  ),
                ),
              );
            }

            return Positioned(
              left: 0,
              right: 0,
              top: padding.top + 8,
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 480),
                    child: _ExpandedNoticeContent(state: state, bloc: bloc),
                  ),
                ),
              ),
            );
          },
        ),
      ],
    );
  }
}

class _ExpandedNoticeContent extends StatelessWidget {
  const _ExpandedNoticeContent({
    required this.state,
    required this.bloc,
  });

  final AppTopNoticeState state;
  final AppTopNoticeBloc bloc;

  @override
  Widget build(BuildContext context) {
    void collapse() {
      bloc.add(const AppTopNoticeSetCollapsed(true));
    }

    if (state.toastMessage != null) {
      return _NoticeCard(
        message: state.toastMessage!,
        level: state.toastLevel,
        toastAction: state.toastAction,
        dense: false,
        onCollapse: collapse,
      );
    }

    if (state.showServerOfflineBanner) {
      return _ServerOfflineCard(
        checking: state.serverCheckInFlight,
        onCheck: () {
          bloc.add(const AppTopNoticeManualServerCheck());
        },
        onCollapse: collapse,
      );
    }

    return const SizedBox.shrink();
  }
}

class _CollapsedNoticeChip extends StatelessWidget {
  const _CollapsedNoticeChip({
    required this.level,
    required this.onExpand,
  });

  final AppTopNoticeLevel level;
  final VoidCallback onExpand;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final style = _levelStyle(theme, level);

    return Semantics(
      label: 'Показать уведомление',
      button: true,
      child: Material(
        elevation: 4,
        shape: const CircleBorder(),
        color: style.background,
        child: InkWell(
          customBorder: const CircleBorder(),
          onTap: onExpand,
          child: Padding(
            padding: const EdgeInsets.all(10),
            child: Icon(
              style.icon,
              color: style.foreground,
              size: 22,
            ),
          ),
        ),
      ),
    );
  }
}

class _LevelStyle {
  const _LevelStyle({
    required this.background,
    required this.foreground,
    required this.icon,
  });

  final Color background;
  final Color foreground;
  final IconData icon;
}

_LevelStyle _levelStyle(ThemeData theme, AppTopNoticeLevel level) {
  return switch (level) {
    AppTopNoticeLevel.info => _LevelStyle(
      background: const Color(0xFF3D6898),
      foreground: const Color(0xFFFAFAFA),
      icon: Icons.info_outline_sharp,
    ),
    AppTopNoticeLevel.warning => _LevelStyle(
      background: const Color(0xFF8A6329),
      foreground: const Color(0xFFFAFAFA),
      icon: Icons.warning_amber_rounded,
    ),
    AppTopNoticeLevel.error => _LevelStyle(
      background: const Color(0xFF8B1010),
      foreground: const Color(0xFFFAFAFA),
      icon: Icons.error_outline_rounded,
    ),
  };
}

class _NoticeCard extends StatelessWidget {
  const _NoticeCard({
    required this.message,
    required this.level,
    required this.toastAction,
    required this.dense,
    required this.onCollapse,
  });

  final String message;
  final AppTopNoticeLevel level;
  final AppTopNoticeToastAction toastAction;
  final bool dense;
  final VoidCallback onCollapse;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final style = _levelStyle(theme, level);
    final hasAction = toastAction != AppTopNoticeToastAction.none;

    Widget body;
    if (hasAction) {
      body = Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Icon(
              style.icon,
              size: dense ? 18 : 20,
              color: style.foreground,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              message,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: style.foreground,
                fontSize: dense ? 13 : null,
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: _ToastActionButton(
              action: toastAction,
              foregroundColor: style.foreground,
            ),
          ),
          const SizedBox(width: 2),
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: _CollapseIconButton(
              color: style.foreground,
              onPressed: onCollapse,
            ),
          ),
        ],
      );
    } else {
      body = Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Icon(
              style.icon,
              size: dense ? 18 : 20,
              color: style.foreground,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              message,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: style.foreground,
                fontSize: dense ? 13 : null,
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: _CollapseIconButton(
              color: style.foreground,
              onPressed: onCollapse,
            ),
          ),
        ],
      );
    }

    return Material(
      elevation: 3,
      borderRadius: BorderRadius.circular(10),
      color: style.background,
      child: Padding(
        padding: EdgeInsets.fromLTRB(12, dense ? 6 : 8, 4, dense ? 6 : 8),
        child: body,
      ),
    );
  }
}

class _ToastActionButton extends StatelessWidget {
  const _ToastActionButton({
    required this.action,
    required this.foregroundColor,
  });

  final AppTopNoticeToastAction action;
  final Color foregroundColor;

  @override
  Widget build(BuildContext context) {
    if (action != AppTopNoticeToastAction.chatReloadRunners) {
      return const SizedBox.shrink();
    }

    return BlocBuilder<ChatBloc, ChatState>(
      buildWhen: (p, n) => p.runnersStatusRefreshing != n.runnersStatusRefreshing,
      builder: (context, chatState) {
        final busy = chatState.runnersStatusRefreshing;
        return FilledButton.tonal(
          onPressed: busy
            ? null
            : () {
              context.read<ChatBloc>().add(const ChatLoadRunners());
            },
          style: FilledButton.styleFrom(
            visualDensity: VisualDensity.compact,
            minimumSize: Size.zero,
            tapTargetSize: MaterialTapTargetSize.shrinkWrap,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
          ),
          child: busy
            ? SizedBox(
              height: 18,
              width: 18,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: foregroundColor,
              ),
            )
            : Text(
              'Проверить',
              style: TextStyle(color: foregroundColor, fontSize: 13),
            ),
        );
      },
    );
  }
}

class _ServerOfflineCard extends StatelessWidget {
  const _ServerOfflineCard({
    required this.checking,
    required this.onCheck,
    required this.onCollapse,
  });

  final bool checking;
  final VoidCallback onCheck;
  final VoidCallback onCollapse;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final style = _levelStyle(theme, AppTopNoticeLevel.error);

    return Material(
      elevation: 3,
      borderRadius: BorderRadius.circular(10),
      color: style.background,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 8, 4, 8),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.center,
              children: [
                Icon(
                  style.icon,
                  size: 20,
                  color: style.foreground,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'Нет связи с сервером',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: style.foreground,
                      fontSize: 13,
                    ),
                  ),
                ),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    FilledButton.tonal(
                      onPressed: checking ? null : onCheck,
                      style: FilledButton.styleFrom(
                        visualDensity: VisualDensity.compact,
                        padding: const EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 8,
                        ),
                      ),
                      child: checking
                        ? SizedBox(
                          height: 18,
                          width: 18,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: style.foreground,
                          ),
                        )
                        : Text(
                          'Проверить',
                          style: TextStyle(
                            color: style.foreground,
                            fontSize: 13,
                          ),
                        ),
                    ),
                    _CollapseIconButton(
                      color: style.foreground,
                      onPressed: onCollapse,
                    ),
                  ],
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _CollapseIconButton extends StatelessWidget {
  const _CollapseIconButton({
    required this.color,
    required this.onPressed,
  });

  final Color color;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return IconButton(
      visualDensity: VisualDensity.compact,
      tooltip: null,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints(
        minWidth: 32,
        minHeight: 32,
      ),
      style: IconButton.styleFrom(
        foregroundColor: color,
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
      onPressed: onPressed,
      icon: Icon(
        Icons.expand_less_rounded,
        size: 22,
        color: color,
      ),
    );
  }
}
