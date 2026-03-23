import 'package:flutter/material.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class ChatAppBarTitle extends StatelessWidget {
  const ChatAppBarTitle({super.key, required this.state, this.compact = false});

  final ChatState state;

  final bool compact;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final currentSession = state.sessions.firstWhere(
      (session) => session.id == state.currentSessionId,
      orElse: () => ChatSession(
        id: 0,
        title: 'Новый чат',
        createdAt: DateTime.now(),
        updatedAt: DateTime.now(),
      ),
    );

    return Padding(
      padding: const EdgeInsets.only(left: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            currentSession.title,
            style: TextStyle(
              fontSize: compact ? 17 : 16,
              fontWeight: FontWeight.w600,
            ),
            overflow: TextOverflow.ellipsis,
            maxLines: 1,
          ),
          if (!state.isConnected)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: Row(
                children: [
                  Icon(
                    Icons.wifi_off,
                    size: 12,
                    color: theme.colorScheme.error,
                  ),
                  const SizedBox(width: 4),
                  Text(
                    'Нет подключения',
                    style: TextStyle(
                      fontSize: 11,
                      color: theme.colorScheme.error,
                    ),
                  ),
                ],
              ),
            ),
        ],
      ),
    );
  }
}
