import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/widgets/session_context_menu.dart';
import 'package:gen/presentation/screens/chat/widgets/session_list_tile.dart';
import 'package:gen/presentation/screens/chat/widgets/session_title_edit_dialog.dart';
import 'package:gen/presentation/screens/chat/widgets/sessions_drawer_header.dart';
import 'package:gen/presentation/screens/chat/widgets/sessions_new_chat_footer.dart';
import 'package:gen/presentation/screens/chat/widgets/sessions_sidebar_states.dart';

class SessionsSidebar extends StatefulWidget {
  const SessionsSidebar({
    super.key,
    required this.onCreateNewSession,
    required this.onSelectSession,
    required this.onDeleteSession,
    this.isInDrawer = false,
  });

  final VoidCallback onCreateNewSession;
  final void Function(ChatSession) onSelectSession;
  final void Function(int sessionId, String title) onDeleteSession;
  final bool isInDrawer;

  @override
  State<SessionsSidebar> createState() => _SessionsSidebarState();
}

class _SessionsSidebarState extends State<SessionsSidebar> {
  final ScrollController _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _loadSessions();
    });
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  void _loadSessions() {
    context.read<ChatBloc>().add(ChatLoadSessions());
  }

  void _onEditSession(ChatSession session) {
    showSessionTitleEditDialog(context, session);
  }

  void _onDeleteSession(ChatSession session) {
    widget.onDeleteSession(session.id, session.title);
  }

  void _showSessionContextMenuDesktop(
    ChatSession session,
    TapDownDetails details,
  ) {
    final screenSize = MediaQuery.sizeOf(context);
    final position = RelativeRect.fromLTRB(
      details.globalPosition.dx,
      details.globalPosition.dy,
      screenSize.width - details.globalPosition.dx,
      screenSize.height - details.globalPosition.dy,
    );
    SessionContextMenu.showDesktopMenu(
      context,
      position: position,
      onEdit: () => _onEditSession(session),
      onDelete: () => _onDeleteSession(session),
    );
  }

  void _showSessionContextMenuMobile(ChatSession session) {
    SessionContextMenu.showMobileSheet(
      context,
      onEdit: () => _onEditSession(session),
      onDelete: () => _onDeleteSession(session),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.transparent,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (widget.isInDrawer) const SessionsDrawerHeader(),
          Expanded(
            child: BlocBuilder<ChatBloc, ChatState>(
              builder: (context, state) {
                if (state.isLoading) {
                  return const SessionsSidebarLoadingState();
                }

                if (state.error != null) {
                  return SessionsSidebarErrorState(onRetry: _loadSessions);
                }

                if (state.sessions.isEmpty) {
                  return const SessionsSidebarEmptyState();
                }

                return RefreshIndicator(
                  onRefresh: () async {
                    _loadSessions();
                    await Future.delayed(const Duration(milliseconds: 500));
                  },
                  child: ListView.builder(
                    controller: _scrollController,
                    padding: const EdgeInsets.symmetric(vertical: 8),
                    itemCount: state.sessions.length,
                    itemBuilder: (context, index) {
                      final session = state.sessions[index];
                      final isSelected = state.currentSessionId == session.id;
                      final isDesktop = Breakpoints.isDesktop(context);
                      return SessionListTile(
                        session: session,
                        isSelected: isSelected,
                        isDesktop: isDesktop,
                        onTap: () => widget.onSelectSession(session),
                        onLongPress: () =>
                            _showSessionContextMenuMobile(session),
                        onSecondaryTapDown: isDesktop
                            ? (d) => _showSessionContextMenuDesktop(session, d)
                            : null,
                      );
                    },
                  ),
                );
              },
            ),
          ),
          SessionsNewChatFooter(
            onPressed: widget.onCreateNewSession,
            isInDrawer: widget.isInDrawer,
          ),
        ],
      ),
    );
  }
}
