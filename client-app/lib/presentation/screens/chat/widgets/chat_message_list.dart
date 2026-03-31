import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/widgets/chat_bubble.dart';

class ChatMessageList extends StatelessWidget {
  const ChatMessageList({
    super.key,
    required this.scrollController,
    required this.state,
  });

  final ScrollController scrollController;
  final ChatState state;

  @override
  Widget build(BuildContext context) {
    final horizontalPadding = Breakpoints.isMobile(context) ? 12.0 : 16.0;
    return ListView.builder(
      controller: scrollController,
      padding: EdgeInsets.symmetric(
        vertical: 16,
        horizontal: horizontalPadding,
      ),
      itemCount: state.messages.length + (state.isStreaming ? 1 : 0),
      itemBuilder: (context, index) {
        if (index < state.messages.length) {
          final msg = state.messages[index];
          final canRegenerate = !state.isStreaming &&
              index == state.messages.length - 1 &&
              msg.role == MessageRole.assistant &&
              msg.id > 0;
          return Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: ChatBubble(
              message: msg,
              sessionId: state.currentSessionId,
              onRegenerate: canRegenerate
                  ? () => context.read<ChatBloc>().add(
                        ChatRegenerateAssistant(msg.id),
                      )
                  : null,
            ),
          );
        }
        return Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: ChatBubble(
            message: Message(
              id: -1,
              content: state.currentStreamingText ?? '',
              role: MessageRole.assistant,
              createdAt: DateTime.now(),
            ),
            sessionId: state.currentSessionId,
            isStreaming: true,
            streamingStatus: state.toolProgressLabel,
          ),
        );
      },
    );
  }
}
