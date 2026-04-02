import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/core/ui/app_top_notice_controller.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_connection_status_bar.dart';

class AppRootTopChrome extends StatelessWidget {
  const AppRootTopChrome({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SafeArea(
          bottom: false,
          left: false,
          right: false,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              BlocBuilder<ChatBloc, ChatState>(
                builder: (context, state) {
                  return ChatConnectionStatusBar(
                    state: state,
                    onRetry: () => context.read<ChatBloc>().add(const ChatStarted()),
                  );
                },
              ),
              AnimatedBuilder(
                animation: sl<AppTopNoticeController>(),
                builder: (context, _) {
                  final controller = sl<AppTopNoticeController>();
                  final entry = controller.current;
                  return AnimatedSwitcher(
                    duration: const Duration(milliseconds: 280),
                    switchInCurve: Curves.easeOutCubic,
                    switchOutCurve: Curves.easeInCubic,
                    layoutBuilder: (currentChild, previousChildren) {
                      return Stack(
                        alignment: Alignment.topCenter,
                        clipBehavior: Clip.none,
                        children: <Widget>[
                          ...previousChildren,
                          ?currentChild,
                        ],
                      );
                    },
                    transitionBuilder: (child, animation) {
                      final offset = Tween<Offset>(
                        begin: const Offset(0, -1),
                        end: Offset.zero,
                      ).animate(
                        CurvedAnimation(
                          parent: animation,
                          curve: Curves.easeOutCubic,
                        ),
                      );
                      return ClipRect(
                        child: SlideTransition(
                          position: offset,
                          child: child,
                        ),
                      );
                    },
                    child: entry == null
                      ? const SizedBox(
                        key: ValueKey<String>('app-top-notice-empty'),
                        width: double.infinity,
                      )
                      : _NoticeBanner(
                        key: ValueKey<int>(entry.id),
                        entry: entry,
                        onDismiss: controller.dismissCurrent,
                      ),
                  );
                },
              ),
            ],
          ),
        ),
        Expanded(child: child),
      ],
    );
  }
}

class _NoticeBanner extends StatelessWidget {
  const _NoticeBanner({
    super.key,
    required this.entry,
    required this.onDismiss,
  });

  final AppTopNoticeEntry entry;
  final VoidCallback onDismiss;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final bg = entry.error ? scheme.errorContainer : scheme.secondaryContainer;
    final fg = entry.error ? scheme.onErrorContainer : scheme.onSecondaryContainer;

    return Material(
      color: bg,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(
              entry.error ? Icons.error_outline : Icons.info_outline,
              size: 20,
              color: fg,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                entry.message,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: fg,
                  height: 1.35,
                ),
              ),
            ),
            Semantics(
              label: 'Закрыть',
              button: true,
              child: Material(
                color: Colors.transparent,
                child: InkWell(
                  onTap: onDismiss,
                  customBorder: const CircleBorder(),
                  child: Padding(
                    padding: const EdgeInsets.all(8),
                    child: Icon(Icons.close_rounded, size: 20, color: fg),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
