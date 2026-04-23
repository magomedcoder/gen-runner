import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/domain/entities/mcp_server_entity.dart';
import 'package:gen/domain/repositories/runners_repository.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

bool chatMcpSheetCanOpen(ChatState state) {
  return state.currentSessionId == null || state.sessionSettings != null;
}

bool chatMcpEffectiveOn(ChatState state) {
  if (state.currentSessionId == null) {
    return state.draftMcpServerIds.isNotEmpty;
  }
  return (state.sessionSettings?.mcpServerIds ?? const []).isNotEmpty;
}

class ChatMcpMenuButton extends StatefulWidget {
  const ChatMcpMenuButton({super.key, required this.isEnabled});

  final bool isEnabled;

  @override
  State<ChatMcpMenuButton> createState() => _ChatMcpMenuButtonState();
}

class _ChatMcpMenuButtonState extends State<ChatMcpMenuButton> {
  List<McpServerEntity> _servers = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _loadServers());
  }

  Future<void> _loadServers() async {
    try {
      final list = await di.sl<RunnersRepository>().listUserMcpServers();
      if (!mounted) {
        return;
      }
      final filtered = list.where((s) => s.enabled).toList();
      filtered.sort((a, b) {
        final an = a.name.trim().isNotEmpty ? a.name : '#${a.id}';
        final bn = b.name.trim().isNotEmpty ? b.name : '#${b.id}';
        return an.toLowerCase().compareTo(bn.toLowerCase());
      });
      setState(() {
        _servers = filtered;
        _loading = false;
      });
    } catch (_) {
      if (mounted) {
        setState(() => _loading = false);
      }
    }
  }

  List<int> _currentIds(ChatState state) {
    if (state.currentSessionId == null) {
      return state.draftMcpServerIds;
    }
    return state.sessionSettings?.mcpServerIds ?? const [];
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return BlocBuilder<ChatBloc, ChatState>(
      builder: (context, state) {
        final canOpen = widget.isEnabled && chatMcpSheetCanOpen(state);
        final mcpOn = chatMcpEffectiveOn(state);
        final curIds = _currentIds(state);

        Color mcpIconColor;
        if (!canOpen) {
          mcpIconColor =
              theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.38);
        } else if (mcpOn) {
          final primary = theme.colorScheme.primary;
          mcpIconColor = widget.isEnabled
              ? theme.colorScheme.onSurfaceVariant
              : primary.withValues(alpha: 0.48);
        } else {
          mcpIconColor =
              theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.4);
        }

        PopupMenuItem<String> menuItem(
          String value,
          String label, {
          bool checked = false,
        }) {
          return PopupMenuItem<String>(
            value: value,
            child: Row(
              children: [
                SizedBox(
                  width: 22,
                  child: checked
                      ? Icon(
                          Icons.check_rounded,
                          size: 18,
                          color: theme.colorScheme.primary,
                        )
                      : null,
                ),
                Expanded(child: Text(label)),
              ],
            ),
          );
        }

        return PopupMenuButton<String>(
          tooltip: 'MCP: серверы и инструменты',
          enabled: canOpen,
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: mcpOn && canOpen
                ? DecoratedBox(
                    decoration: BoxDecoration(
                      color: theme.colorScheme.primary.withValues(
                        alpha: 0.16,
                      ),
                      shape: BoxShape.circle,
                    ),
                    child: SizedBox(
                      width: 26,
                      height: 26,
                      child: Center(
                        child: Icon(
                          Icons.extension,
                          color: mcpIconColor,
                          size: 22,
                        ),
                      ),
                    ),
                  )
                : Icon(
                    Icons.extension_outlined,
                    color: mcpIconColor,
                    size: 22,
                  ),
          ),
          onSelected: (v) {
            final bloc = context.read<ChatBloc>();
            final curState = bloc.state;
            final ids = List<int>.from(_currentIds(curState));
            final id = int.tryParse(v);
            if (id == null) {
              return;
            }
            if (ids.contains(id)) {
              ids.remove(id);
            } else {
              ids.add(id);
            }
            bloc.add(ChatSetMcp(serverIds: ids));
          },
          itemBuilder: (ctx) {
            if (_loading) {
              return [
                PopupMenuItem<String>(
                  enabled: false,
                  child: Row(
                    children: [
                      SizedBox(
                        width: 22,
                        height: 22,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: theme.colorScheme.primary,
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          'Загрузка серверов...',
                          style: theme.textTheme.bodyMedium,
                        ),
                      ),
                    ],
                  ),
                ),
              ];
            }
            final items = <PopupMenuEntry<String>>[];
            if (_servers.isEmpty) {
              items.add(
                PopupMenuItem<String>(
                  enabled: false,
                  child: Text(
                    'Нет доступных серверов',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
              );
              return items;
            }
            for (final s in _servers) {
              final label =
                  s.name.trim().isNotEmpty ? s.name : '#${s.id}';
              items.add(
                menuItem(
                  '${s.id}',
                  label,
                  checked: curIds.contains(s.id),
                ),
              );
            }
            return items;
          },
        );
      },
    );
  }
}
