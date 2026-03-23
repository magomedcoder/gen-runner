import 'package:desktop_drop/desktop_drop.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_app_bar_title.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_dialogs.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_input_bar.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_messages_panel.dart';
import 'package:gen/presentation/screens/chat/widgets/sessions_list_header.dart';
import 'package:gen/presentation/screens/chat/widgets/sessions_sidebar.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _scrollController = ScrollController();
  final _scaffoldKey = GlobalKey<ScaffoldState>();
  final _inputBarKey = GlobalKey<ChatInputBarState>();
  bool _isSidebarExpanded = true;
  bool _isDraggingFile = false;
  double get _sidebarWidth => Breakpoints.sidebarDefaultWidth;

  static const double _scrollThreshold = 80.0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      context.read<ChatBloc>().add(ChatStarted());
    });
  }

  void _scrollToBottom() {
    if (!mounted) {
      return;
    }

    if (!_scrollController.hasClients) {
      return;
    }

    final pos = _scrollController.position;
    if (pos.maxScrollExtent - pos.pixels <= _scrollThreshold) {
      _scrollController.animateTo(
        pos.maxScrollExtent,
        duration: const Duration(milliseconds: 300),
        curve: Curves.easeOut,
      );
    }
  }

  void _toggleSidebar() {
    setState(() {
      _isSidebarExpanded = !_isSidebarExpanded;
    });
  }

  void _createNewSession() {
    context.read<ChatBloc>().add(const ChatCreateSession());
  }

  void _selectSession(ChatSession session) {
    context.read<ChatBloc>().add(ChatSelectSession(session.id));
  }

  void _selectSessionAndCloseDrawer(ChatSession session) {
    _selectSession(session);
    if (Breakpoints.useDrawerForSessions(context)) {
      Navigator.of(context).pop();
    }
  }

  void _deleteSession(int sessionId, String sessionTitle) {
    showDeleteSessionDialog(
      context,
      sessionId: sessionId,
      sessionTitle: sessionTitle,
      chatBloc: context.read<ChatBloc>(),
    );
  }

  Future<void> _onFilesDropped(DropDoneDetails details) async {
    setState(() => _isDraggingFile = false);
    if (details.files.isEmpty) {
      return;
    }

    final item = details.files.first;
    if (item is! DropItemFile) {
      return;
    }

    try {
      final bytes = await item.readAsBytes();
      final name = item.name.isNotEmpty
          ? item.name
          : item.path.split(RegExp(r'[/\\]')).last;
      if (!mounted) return;
      _inputBarKey.currentState?.setDroppedFile(
        PlatformFile(name: name, size: bytes.length, bytes: bytes),
      );
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Не удалось прочитать файл')),
        );
      }
    }
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return BlocListener<ChatBloc, ChatState>(
      listenWhen: (previous, current) =>
          previous.currentSessionId != current.currentSessionId,
      listener: (context, state) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (!mounted) {
            return;
          }
          _inputBarKey.currentState?.resetComposer();
        });
      },
      child: BlocListener<ChatBloc, ChatState>(
        listener: (context, state) {
          WidgetsBinding.instance.addPostFrameCallback((_) {
            if (state.messages.isNotEmpty) {
              _scrollToBottom();
            }
          });

          if (state.error != null) {
            WidgetsBinding.instance.addPostFrameCallback((_) {
              if (!mounted) return;
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(
                  content: Text(state.error!),
                  backgroundColor: Theme.of(context).colorScheme.error,
                  behavior: SnackBarBehavior.floating,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(8),
                  ),
                ),
              );
            });
          }
        },
        child: Builder(
          builder: (context) {
            final useDrawer = Breakpoints.useDrawerForSessions(context);
            return Scaffold(
              key: _scaffoldKey,
              drawer: useDrawer
                  ? Drawer(
                      backgroundColor: theme.colorScheme.surfaceContainerLow,
                      child: SafeArea(
                        child: SessionsSidebar(
                          isInDrawer: true,
                          onCreateNewSession: _createNewSession,
                          onSelectSession: _selectSessionAndCloseDrawer,
                          onDeleteSession: _deleteSession,
                        ),
                      ),
                    )
                  : null,
              body: SafeArea(
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (!useDrawer)
                      AnimatedContainer(
                        duration: const Duration(milliseconds: 280),
                        curve: Curves.easeInOutCubic,
                        width: _isSidebarExpanded ? _sidebarWidth : 0,
                        clipBehavior: Clip.hardEdge,
                        decoration: BoxDecoration(
                          color: theme.colorScheme.surfaceContainerLow,
                          border: Border(
                            right: BorderSide(
                              color: theme.dividerColor.withValues(alpha: 0.14),
                              width: 1,
                            ),
                          ),
                        ),
                        child: _isSidebarExpanded
                            ? Column(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  SessionsListHeader(
                                    onToggleCollapse: _toggleSidebar,
                                  ),
                                  Expanded(
                                    child: SessionsSidebar(
                                      onCreateNewSession: _createNewSession,
                                      onSelectSession: _selectSession,
                                      onDeleteSession: _deleteSession,
                                    ),
                                  ),
                                ],
                              )
                            : const SizedBox.shrink(),
                      ),
                    Expanded(
                      child: Material(
                        color: theme.colorScheme.surface,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Container(
                              width: double.infinity,
                              padding: const EdgeInsets.fromLTRB(4, 8, 8, 8),
                              decoration: BoxDecoration(
                                color: theme.colorScheme.surface,
                                border: Border(
                                  bottom: BorderSide(
                                    color: theme.dividerColor.withValues(
                                      alpha: 0.12,
                                    ),
                                  ),
                                ),
                              ),
                              child: Row(
                                crossAxisAlignment: CrossAxisAlignment.center,
                                children: [
                                  if (useDrawer)
                                    IconButton(
                                      icon: const Icon(Icons.menu_rounded),
                                      onPressed: () => _scaffoldKey.currentState
                                          ?.openDrawer(),
                                      tooltip: 'Список чатов',
                                    ),
                                  if (!useDrawer && !_isSidebarExpanded)
                                    IconButton(
                                      icon: const Icon(Icons.menu_rounded),
                                      onPressed: _toggleSidebar,
                                      tooltip: 'Показать список чатов',
                                    ),
                                  Expanded(
                                    child: BlocBuilder<ChatBloc, ChatState>(
                                      builder: (context, state) =>
                                          ChatAppBarTitle(
                                            state: state,
                                            compact: useDrawer,
                                          ),
                                    ),
                                  ),
                                  BlocBuilder<ChatBloc, ChatState>(
                                    builder: (context, state) {
                                      if (state.isLoading &&
                                          !state.isStreaming) {
                                        return const Padding(
                                          padding: EdgeInsets.only(right: 12),
                                          child: SizedBox(
                                            width: 18,
                                            height: 18,
                                            child: CircularProgressIndicator(
                                              strokeWidth: 2,
                                            ),
                                          ),
                                        );
                                      }
                                      return const SizedBox(width: 8);
                                    },
                                  ),
                                ],
                              ),
                            ),
                            Expanded(
                              child: BlocBuilder<ChatBloc, ChatState>(
                                builder: (context, state) {
                                  final canDropFile =
                                      state.isConnected &&
                                      !state.isLoading &&
                                      (state.hasActiveRunners != false);
                                  return ChatMessagesPanel(
                                    state: state,
                                    scrollController: _scrollController,
                                    inputBarKey: _inputBarKey,
                                    isDraggingFile: _isDraggingFile,
                                    canDropFile: canDropFile,
                                    onDragEntered: (_) =>
                                        setState(() => _isDraggingFile = true),
                                    onDragExited: (_) =>
                                        setState(() => _isDraggingFile = false),
                                    onDragDone: _onFilesDropped,
                                  );
                                },
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            );
          },
        ),
      ),
    );
  }
}
