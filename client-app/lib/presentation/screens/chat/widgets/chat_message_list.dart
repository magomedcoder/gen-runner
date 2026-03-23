import 'package:flutter/material.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/message.dart';
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
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: ChatBubble(message: state.messages[index]),
          );
        }
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: ChatBubble(
            message: Message(
              id: -1,
              content: state.currentStreamingText ?? '',
              role: MessageRole.assistant,
              createdAt: DateTime.now(),
            ),
            isStreaming: true,
          ),
        );
      },
    );
  }
}
