import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class ChatModelSelector extends StatelessWidget {
  const ChatModelSelector({super.key, required this.state});

  final ChatState state;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final models = state.models;
    final selected = state.selectedModel;
    final isEnabled = state.isConnected && !state.isLoading;

    if (models.isEmpty) {
      return Tooltip(
        message: 'Модели не загружены',
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            color: theme.colorScheme.surfaceContainerLow,
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.smart_toy_outlined,
                size: 14,
                color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
              ),
              const SizedBox(width: 5),
              Text(
                'Модель',
                style: TextStyle(
                  fontSize: 11,
                  color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
                ),
              ),
            ],
          ),
        ),
      );
    }

    return PopupMenuButton<String>(
      enabled: isEnabled,
      tooltip: 'Выбор модели',
      padding: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
        decoration: BoxDecoration(
          color: theme.colorScheme.surfaceContainerLow,
          borderRadius: BorderRadius.circular(6),
          border: Border.all(
            color: theme.colorScheme.outline.withValues(alpha: 0.2),
            width: 1,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.smart_toy_outlined,
              size: 14,
              color: isEnabled
                  ? theme.colorScheme.primary
                  : theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
            ),
            const SizedBox(width: 5),
            Text(
              selected ?? models.first,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w500,
                color: isEnabled
                    ? theme.colorScheme.onSurface
                    : theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(width: 2),
            Icon(
              Icons.keyboard_arrow_down_rounded,
              color: theme.colorScheme.onSurfaceVariant,
              size: 16,
            ),
          ],
        ),
      ),
      onOpened: () {
        if (state.models.isEmpty) {
          context.read<ChatBloc>().add(const ChatLoadModels());
        }
      },
      itemBuilder: (context) => [
        for (final model in models)
          PopupMenuItem<String>(value: model, child: Text(model)),
      ],
      onSelected: (value) {
        context.read<ChatBloc>().add(ChatSelectModel(value));
      },
    );
  }
}
