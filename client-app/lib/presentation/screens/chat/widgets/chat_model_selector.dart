import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class ChatRunnerSelector extends StatelessWidget {
  const ChatRunnerSelector({super.key, required this.state});

  final ChatState state;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final runners = state.runners;
    final selected = state.selectedRunner;
    final isEnabled = state.isConnected && !state.isLoading;

    if (runners.isEmpty) {
      return Tooltip(
        message: 'Раннеры не загружены',
        child: Icon(
          Icons.dns_outlined,
          size: 20,
          color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
        ),
      );
    }

    return PopupMenuButton<String>(
      enabled: isEnabled,
      tooltip: 'Выбор раннера',
      initialValue: selected ?? runners.first,
      padding: const EdgeInsets.all(4),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
      child: Icon(
        Icons.dns_outlined,
        size: 20,
        color: isEnabled
          ? theme.colorScheme.primary
          : theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.6),
      ),
      onOpened: () {
        if (state.runners.isEmpty) {
          context.read<ChatBloc>().add(const ChatLoadRunners());
        }
      },
      itemBuilder: (context) => [
        for (final runner in runners)
          PopupMenuItem<String>(
            value: runner,
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    state.runnerNames[runner] ?? runner,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (runner == (selected ?? runners.first))
                  Icon(
                    Icons.check_rounded,
                    size: 18,
                    color: theme.colorScheme.primary,
                  ),
              ],
            ),
          ),
      ],
      onSelected: (value) {
        context.read<ChatBloc>().add(ChatSelectRunner(value));
      },
    );
  }
}
