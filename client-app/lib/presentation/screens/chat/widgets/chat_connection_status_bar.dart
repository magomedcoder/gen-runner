import 'package:flutter/material.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

enum ChatServerConnectionStatus {
  connected,
  connecting,
  syncing,
  disconnected,
}

ChatServerConnectionStatus resolveChatServerConnectionStatus(ChatState state) {
  if (state.isConnected) {
    if (state.isLoading && !state.isStreamingInCurrentSession) {
      return ChatServerConnectionStatus.syncing;
    }

    return ChatServerConnectionStatus.connected;
  }

  if (state.isLoading) {
    return ChatServerConnectionStatus.connecting;
  }

  if (state.hasCompletedInitialConnection) {
    return ChatServerConnectionStatus.disconnected;
  }

  return ChatServerConnectionStatus.connected;
}

class ChatConnectionStatusBar extends StatelessWidget {
  const ChatConnectionStatusBar({
    super.key,
    required this.state,
    this.onRetry,
  });

  final ChatState state;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final status = resolveChatServerConnectionStatus(state);
    if (status == ChatServerConnectionStatus.connected) {
      return const SizedBox.shrink();
    }
    
    return _buildBar(context, status);
  }

  Widget _buildBar(BuildContext context, ChatServerConnectionStatus status) {
    final colors = Theme.of(context).colorScheme;

    late Color backgroundColor;
    late Color foregroundColor;
    late IconData icon;
    late String text;
    var showSpinner = false;

    switch (status) {
      case ChatServerConnectionStatus.connecting:
        backgroundColor = colors.primaryContainer;
        foregroundColor = colors.onPrimaryContainer;
        icon = Icons.sync;
        text = 'Подключение к серверу...';
        showSpinner = true;
        break;
      case ChatServerConnectionStatus.syncing:
        backgroundColor = colors.primaryContainer;
        foregroundColor = colors.onPrimaryContainer;
        icon = Icons.sync;
        text = 'Синхронизация...';
        showSpinner = true;
        break;
      case ChatServerConnectionStatus.disconnected:
        backgroundColor = colors.errorContainer;
        foregroundColor = colors.onErrorContainer;
        icon = Icons.cloud_off;
        text = 'Нет соединения с сервером';
        break;
      case ChatServerConnectionStatus.connected:
        return const SizedBox.shrink();
    }

    return Material(
      color: backgroundColor,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 16),
        child: Row(
          children: [
            Icon(icon, size: 18, color: foregroundColor),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                text,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: foregroundColor,
                ),
              ),
            ),
            if (showSpinner) ...[
              const SizedBox(width: 10),
              SizedBox(
                width: 16,
                height: 16,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(foregroundColor),
                ),
              ),
            ],
            if (status == ChatServerConnectionStatus.disconnected &&
                onRetry != null) ...[
              const SizedBox(width: 8),
              TextButton(
                onPressed: onRetry,
                style: TextButton.styleFrom(
                  foregroundColor: foregroundColor,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                ),
                child: const Text('Повторить'),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
