import 'dart:math' as math;

import 'package:desktop_drop/desktop_drop.dart';
import 'package:flutter/material.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_drop_overlay.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_input_bar.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_message_list.dart';
import 'package:gen/presentation/screens/chat/widgets/rag_ingestion_status_banner.dart';

class ChatMessagesPanel extends StatelessWidget {
  const ChatMessagesPanel({
    super.key,
    required this.state,
    required this.scrollController,
    required this.inputBarKey,
    required this.immersiveEmptyChat,
    required this.isDraggingFile,
    required this.canDropFile,
    required this.onDragEntered,
    required this.onDragExited,
    required this.onDragDone,
  });

  final ChatState state;
  final ScrollController scrollController;
  final GlobalKey<ChatInputBarState> inputBarKey;
  final bool immersiveEmptyChat;
  final bool isDraggingFile;
  final bool canDropFile;
  final void Function(DropEventDetails details) onDragEntered;
  final void Function(DropEventDetails details) onDragExited;
  final Future<void> Function(DropDoneDetails details) onDragDone;

  @override
  Widget build(BuildContext context) {
    if (state.isLoading && state.messages.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }

    if (immersiveEmptyChat) {
      final maxComposer = math.min(720.0, MediaQuery.sizeOf(context).width - 32);
      return DropTarget(
        onDragEntered: canDropFile ? onDragEntered : null,
        onDragExited: canDropFile ? onDragExited : null,
        onDragDone: canDropFile ? onDragDone : null,
        enable: canDropFile,
        child: Stack(
          children: [
            Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (state.ragIngestionUi != null)
                  RagIngestionStatusBanner(ui: state.ragIngestionUi!),
                Expanded(
                  child: Center(
                    child: SingleChildScrollView(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
                      child: ConstrainedBox(
                        constraints: BoxConstraints(maxWidth: maxComposer),
                        child: ChatInputBar(
                          key: inputBarKey,
                          isEnabled: canDropFile,
                          roundedCard: true,
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            ),
            if (isDraggingFile) const ChatDropOverlay(),
          ],
        ),
      );
    }

    return DropTarget(
      onDragEntered: canDropFile ? onDragEntered : null,
      onDragExited: canDropFile ? onDragExited : null,
      onDragDone: canDropFile ? onDragDone : null,
      enable: canDropFile,
      child: Stack(
        children: [
          Column(
            children: [
              Expanded(
                child: ChatMessageList(
                  scrollController: scrollController,
                  state: state,
                ),
              ),
              if (state.ragIngestionUi != null)
                RagIngestionStatusBanner(ui: state.ragIngestionUi!),
              ChatInputBar(key: inputBarKey, isEnabled: canDropFile),
            ],
          ),
          if (isDraggingFile) const ChatDropOverlay(),
        ],
      ),
    );
  }
}
