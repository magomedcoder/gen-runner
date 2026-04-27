import 'dart:async';

import 'package:desktop_drop/desktop_drop.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/chat_runner_issue_notice.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_app_bar_title.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_dialogs.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_input_bar.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_messages_panel.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_session_settings_button.dart';
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

  static const Duration _loadOlderDebounce = Duration(milliseconds: 320);

  Timer? _loadOlderDebounceTimer;
  ChatState? _prevStateForScroll;

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onChatScroll);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      context.read<ChatBloc>().add(ChatStarted());
    });
  }

  void _onChatScroll() {
    if (!_scrollController.hasClients) {
      return;
    }
    final bloc = context.read<ChatBloc>();
    final s = bloc.state;
    if (s.currentSessionId == null ||
        !s.hasMoreOlderMessages ||
        s.isLoadingOlderMessages ||
        s.messages.isEmpty) {
      return;
    }

    final pos = _scrollController.position;
    if (pos.maxScrollExtent - pos.pixels > 120) {
      return;
    }

    _loadOlderDebounceTimer?.cancel();
    _loadOlderDebounceTimer = Timer(_loadOlderDebounce, () {
      if (!mounted) {
        return;
      }
      final cur = context.read<ChatBloc>().state;
      if (cur.currentSessionId == null ||
          !cur.hasMoreOlderMessages ||
          cur.isLoadingOlderMessages ||
          cur.messages.isEmpty) {
        return;
      }
      context.read<ChatBloc>().add(const ChatLoadOlderMessages());
    });
  }

  void _scrollLatestIntoView() {
    if (!mounted) {
      return;
    }

    void go() {
      if (!mounted || !_scrollController.hasClients) {
        return;
      }

      final pos = _scrollController.position;
      final t = pos.minScrollExtent;
      if (t.isFinite) {
        _scrollController.jumpTo(t.clamp(pos.minScrollExtent, pos.maxScrollExtent));
      }
    }

    WidgetsBinding.instance.addPostFrameCallback((_) {
      go();
      WidgetsBinding.instance.addPostFrameCallback((_) => go());
    });
  }

  void _toggleSidebar() {
    setState(() {
      _isSidebarExpanded = !_isSidebarExpanded;
    });
  }

  void _createNewSession() {
    context.read<ChatBloc>().add(const ChatCreateSession());
  }

  void _createNewSessionAndCloseDrawer() {
    _createNewSession();
    if (Breakpoints.useDrawerForSessions(context)) {
      Navigator.of(context).pop();
    }
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

    final droppedFiles = <PlatformFile>[];
    var readFailed = 0;
    for (final item in details.files) {
      if (item is! DropItemFile) {
        continue;
      }

      try {
        final bytes = await item.readAsBytes();
        final name = item.name.isNotEmpty
            ? item.name
            : item.path.split(RegExp(r'[/\\]')).last;
        droppedFiles.add(
          PlatformFile(name: name, size: bytes.length, bytes: bytes),
        );
      } catch (_) {
        readFailed++;
      }
    }

    if (!mounted) {
      return;
    }

    if (droppedFiles.isNotEmpty || readFailed > 0) {
      _inputBarKey.currentState?.setDroppedFiles(
        droppedFiles,
        readFailedBeforeValidation: readFailed,
      );
    }
  }

  void _preserveScrollAfterPrepend() {
    if (!_scrollController.hasClients) {
      return;
    }
    final pos = _scrollController.position;
    final oldMax = pos.maxScrollExtent;
    final oldPixels = pos.pixels;
    final pinnedToLatest = oldPixels <= 1.0 && oldMax > 0;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_scrollController.hasClients) {
        return;
      }
      final p = _scrollController.position;
      final newMax = p.maxScrollExtent;
      final delta = newMax - oldMax;
      if (pinnedToLatest) {
        _scrollController.jumpTo(p.minScrollExtent.clamp(p.minScrollExtent, newMax));
        return;
      }

      _scrollController.jumpTo((oldPixels + delta).clamp(0.0, newMax));
    });
  }

  void _handleScrollOnStateChange(ChatState prev, ChatState curr) {
    final prepended =
        prev.messages.isNotEmpty &&
        curr.messages.length > prev.messages.length &&
        curr.messages.isNotEmpty &&
        curr.messages.first.id != prev.messages.first.id;
    if (prepended) {
      _preserveScrollAfterPrepend();
      return;
    }

    if (prev.currentSessionId != curr.currentSessionId) {
      if (mounted &&
          (curr.messages.isNotEmpty || curr.isStreamingInCurrentSession)) {
        _scrollLatestIntoView();
      }
      return;
    }

    void tryScrollLatestIntoView() {
      if (!mounted) {
        return;
      }
      if (curr.messages.isEmpty && !curr.isStreamingInCurrentSession) {
        return;
      }
      _scrollLatestIntoView();
    }

    if (curr.isStreamingInCurrentSession && (prev.currentStreamingText != curr.currentStreamingText || prev.currentStreamingReasoning != curr.currentStreamingReasoning || prev.toolProgressLabel != curr.toolProgressLabel || (!prev.isStreamingInCurrentSession && curr.isStreamingInCurrentSession))) {
      tryScrollLatestIntoView();
      return;
    }

    if (prev.isStreamingInCurrentSession && !curr.isStreamingInCurrentSession) {
      tryScrollLatestIntoView();
      return;
    }

    if (curr.messages.length > prev.messages.length) {
      if (prev.messages.isEmpty && curr.messages.isNotEmpty && !curr.isLoading) {
        tryScrollLatestIntoView();
        return;
      }
      if (prev.messages.isNotEmpty &&
          curr.messages.last.id != prev.messages.last.id &&
          curr.messages.first.id == prev.messages.first.id) {
        tryScrollLatestIntoView();
      }
    }
  }

  @override
  void dispose() {
    _loadOlderDebounceTimer?.cancel();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return BlocListener<ChatBloc, ChatState>(
      listenWhen: (previous, current) => previous.currentSessionId != current.currentSessionId,
      listener: (context, state) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (!mounted) {
            return;
          }
          _inputBarKey.currentState?.resetComposer();
          _loadOlderDebounceTimer?.cancel();
        });
      },
      child: BlocListener<ChatBloc, ChatState>(
        listenWhen: (previous, current) => previous != current,
        listener: (context, state) {
          final prev = _prevStateForScroll;
          WidgetsBinding.instance.addPostFrameCallback((_) {
            if (!mounted) {
              return;
            }
            if (prev != null) {
              _handleScrollOnStateChange(prev, state);
            } else if (state.messages.isNotEmpty && state.currentSessionId != null && !state.isLoading) {
              _scrollLatestIntoView();
            }
          });
          _prevStateForScroll = state;
        },
        child: BlocListener<ChatBloc, ChatState>(
          listenWhen: shouldEmitChatRunnerIssueNotice,
          listener: (context, state) {
            final msg = chatRunnerIssueNoticeMessage(state);
            if (msg == null) {
              dismissAppTopNoticeToast();
              return;
            }
            showAppTopNotice(
              msg,
              level: chatRunnerIssueNoticeLevel(state),
              toastAction: AppTopNoticeToastAction.chatReloadRunners,
            );
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
                            onCreateNewSession: _createNewSessionAndCloseDrawer,
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
                                color: theme.dividerColor.withValues(
                                  alpha: 0.14,
                                ),
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
                          color: theme.scaffoldBackgroundColor,
                          child: BlocBuilder<ChatBloc, ChatState>(
                            builder: (context, state) {
                              final immersiveEmpty = state.isEmptyChatComposer;
                              final canDropFile = state.isConnected && !state.isLoading && (state.hasActiveRunners != false);
                              final topPad = MediaQuery.paddingOf(context).top;
                              return Column(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  if (!immersiveEmpty)
                                    Container(
                                      width: double.infinity,
                                      padding: const EdgeInsets.fromLTRB(
                                        4,
                                        8,
                                        8,
                                        8,
                                      ),
                                      decoration: BoxDecoration(
                                        color: theme.colorScheme.surface,
                                        border: Border(
                                          bottom: BorderSide(
                                            color: theme.dividerColor.withValues(alpha: 0.12),
                                          ),
                                        ),
                                      ),
                                      child: Row(
                                        crossAxisAlignment: CrossAxisAlignment.center,
                                        children: [
                                          if (useDrawer)
                                            IconButton(
                                              icon: const Icon(
                                                Icons.menu_rounded,
                                              ),
                                              onPressed: () => _scaffoldKey
                                                  .currentState
                                                  ?.openDrawer(),
                                              tooltip: 'Список чатов',
                                            ),
                                          if (!useDrawer && !_isSidebarExpanded)
                                            IconButton(
                                              icon: const Icon(
                                                Icons.menu_rounded,
                                              ),
                                              onPressed: _toggleSidebar,
                                              tooltip: 'Показать список чатов',
                                            ),
                                          Expanded(
                                            child: BlocBuilder<ChatBloc, ChatState>(
                                                  builder: (context, state) =>ChatAppBarTitle(
                                                        state: state,
                                                        compact: useDrawer,
                                                      ),
                                                ),
                                          ),
                                          BlocBuilder<ChatBloc, ChatState>(
                                            builder: (context, state) {
                                              return ChatSessionSettingsButton(
                                                state: state,
                                              );
                                            },
                                          ),
                                          const SizedBox(width: 8),
                                          BlocBuilder<ChatBloc, ChatState>(
                                            builder: (context, state) {
                                              if (state.isLoading && !state.isStreamingInCurrentSession) {
                                                return const Padding(
                                                  padding: EdgeInsets.only(
                                                    right: 12,
                                                  ),
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
                                    child: Stack(
                                      clipBehavior: Clip.none,
                                      children: [
                                        ChatMessagesPanel(
                                          state: state,
                                          scrollController: _scrollController,
                                          inputBarKey: _inputBarKey,
                                          immersiveEmptyChat: immersiveEmpty,
                                          isDraggingFile: _isDraggingFile,
                                          canDropFile: canDropFile,
                                          onDragEntered: (_) => setState(() => _isDraggingFile = true),
                                          onDragExited: (_) => setState(() => _isDraggingFile = false),
                                          onDragDone: _onFilesDropped,
                                          onDismissRagDocumentPreview: () => context.read<ChatBloc>().add(
                                            const ChatDismissRagDocumentPreview(),
                                          ),
                                        ),
                                        if (immersiveEmpty && useDrawer)
                                          Positioned(
                                            top: topPad + 4,
                                            left: 4,
                                            child: IconButton(
                                              icon: const Icon(
                                                Icons.menu_rounded,
                                              ),
                                              onPressed: () => _scaffoldKey
                                                  .currentState
                                                  ?.openDrawer(),
                                              tooltip: 'Список чатов',
                                            ),
                                          ),
                                        if (immersiveEmpty &&
                                            !useDrawer &&
                                            !_isSidebarExpanded)
                                          Positioned(
                                            top: topPad + 4,
                                            left: 4,
                                            child: IconButton(
                                              icon: const Icon(
                                                Icons.menu_rounded,
                                              ),
                                              onPressed: _toggleSidebar,
                                              tooltip: 'Показать список чатов',
                                            ),
                                          ),
                                      ],
                                    ),
                                  ),
                                ],
                              );
                            },
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
      ),
    );
  }
}
