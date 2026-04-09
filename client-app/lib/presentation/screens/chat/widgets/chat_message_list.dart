import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/widgets/chat_bubble.dart';

String _contentForVersion(Message msg, List<UserMessageEdit> edits, int versionIdx) {
  if (edits.isEmpty) {
    return msg.content;
  }

  if (versionIdx <= 0) {
    return edits.first.oldContent;
  }

  final i = (versionIdx - 1).clamp(0, edits.length - 1);

  return edits[i].newContent;
}

String _assistantContentForVersion(Message msg, List<AssistantMessageRegeneration> regens, int versionIdx) {
  if (regens.isEmpty) {
    return msg.content;
  }

  if (versionIdx <= 0) {
    return regens.first.oldContent;
  }

  final i = (versionIdx - 1).clamp(0, regens.length - 1);

  return regens[i].newContent;
}

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
      itemCount: state.messages.length +
          (state.isStreamingInCurrentSession ? 1 : 0) +
          (state.isLoadingOlderMessages ? 1 : 0),
      itemBuilder: (context, index) {
        if (state.isLoadingOlderMessages && index == 0) {
          return const Padding(
            padding: EdgeInsets.only(bottom: 8),
            child: Center(
              child: SizedBox(
                width: 24,
                height: 24,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            ),
          );
        }
        final offset = state.isLoadingOlderMessages ? 1 : 0;
        final msgIndex = index - offset;
        if (msgIndex < state.messages.length) {
          final msg = state.messages[msgIndex];
          final canRegenerate = !state.isStreamingInCurrentSession &&
              msgIndex == state.messages.length - 1 &&
              msg.role == MessageRole.assistant &&
              msg.id > 0;
          final canEdit =
              !state.isStreamingInCurrentSession && msg.role == MessageRole.user && msg.id > 0;

          final edits = canEdit ? state.editsByMessageId[msg.id] : null;
          final cursor = canEdit ? state.editCursorByMessageId[msg.id] : null;
          final hasEdits = edits != null && edits.isNotEmpty;
          final isEdited = canEdit && (state.editedMessageIds.contains(msg.id) || hasEdits || (msg.updatedAt != null && msg.updatedAt!.millisecondsSinceEpoch != msg.createdAt.millisecondsSinceEpoch));
          final versionsCount = hasEdits ? edits.length + 1 : 1;
          final versionIdx = hasEdits ? (cursor ?? (versionsCount - 1)).clamp(0, versionsCount - 1) : 0;
          final displayMsg = (canEdit && hasEdits)
            ? Message(
              id: msg.id,
              content: _contentForVersion(msg, edits, versionIdx),
              role: msg.role,
              createdAt: msg.createdAt,
              updatedAt: msg.updatedAt,
              attachmentFileName: msg.attachmentFileName,
              attachmentContent: msg.attachmentContent,
              attachmentFileId: msg.attachmentFileId,
              useFileRag: msg.useFileRag,
              fileRagTopK: msg.fileRagTopK,
              fileRagEmbedModel: msg.fileRagEmbedModel,
            )
            : msg;

          final regens = msg.role == MessageRole.assistant ? state.regenerationsByMessageId[msg.id] : null;
          final regenCursor = msg.role == MessageRole.assistant ? state.regenerationCursorByMessageId[msg.id] : null;
          final hasRegens = regens != null && regens.isNotEmpty;
          final regenVersionsCount = hasRegens ? regens.length + 1 : 1;
          final regenVersionIdx = hasRegens
            ? (regenCursor ?? (regenVersionsCount - 1)).clamp(0, regenVersionsCount - 1)
            : 0;
          final displayAssistantMsg = (msg.role == MessageRole.assistant && hasRegens)
            ? Message(
              id: msg.id,
              content: _assistantContentForVersion(msg, regens, regenVersionIdx),
              role: msg.role,
              createdAt: msg.createdAt,
              updatedAt: msg.updatedAt,
              attachmentFileName: msg.attachmentFileName,
              attachmentContent: msg.attachmentContent,
              attachmentFileId: msg.attachmentFileId,
              useFileRag: msg.useFileRag,
              fileRagTopK: msg.fileRagTopK,
              fileRagEmbedModel: msg.fileRagEmbedModel,
            )
            : displayMsg;
          final showAssistantNav = msg.role == MessageRole.assistant && (state.regeneratedAssistantMessageIds.contains(msg.id) || hasRegens);

          final isLastInList = msgIndex == state.messages.length - 1;
          final showContinuePartial = !state.isStreamingInCurrentSession &&
              isLastInList &&
              msg.role == MessageRole.assistant &&
              msg.id > 0 &&
              state.partialAssistantMessageId != null &&
              state.partialAssistantMessageId == msg.id;

          return Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: ChatBubble(
              message: displayAssistantMsg,
              sessionId: state.currentSessionId,
              showContinuePartial: showContinuePartial,
              showEditNav: isEdited || showAssistantNav,
              onRegenerate: canRegenerate
                ? () => context.read<ChatBloc>().add(
                  ChatRegenerateAssistant(msg.id),
                )
                : null,
              onEditSubmit: canEdit
                ? (newText) async {
                  context.read<ChatBloc>().add(ChatEditUserMessageAndContinue(msg.id, newText));
                }
                : null,
              editsTotal: isEdited
                ? versionsCount
                : (showAssistantNav ? regenVersionsCount : null),
              editsIndex: isEdited
                ? versionIdx
                : (showAssistantNav ? regenVersionIdx : null),
              onPrevEdit: isEdited
                ? ((!canEdit || (hasEdits && versionIdx <= 0))
                  ? null
                  : () => context.read<ChatBloc>().add(ChatNavigateUserMessageEdit(msg.id, -1)))
                : (showAssistantNav
                  ? ((hasRegens && regenVersionIdx <= 0)
                    ? null
                    : () => context.read<ChatBloc>().add(ChatNavigateAssistantMessageRegeneration(msg.id, -1)))
                  : null),
              onNextEdit: isEdited
                ? ((!canEdit || (hasEdits && versionIdx >= versionsCount - 1))
                  ? null
                  : () => context.read<ChatBloc>().add(ChatNavigateUserMessageEdit(msg.id, 1)))
                : (showAssistantNav
                  ? ((hasRegens && regenVersionIdx >= regenVersionsCount - 1)
                    ? null
                    : () => context.read<ChatBloc>().add(ChatNavigateAssistantMessageRegeneration(msg.id, 1)))
                    : null),
            ),
          );
        }
        if (state.isStreamingInCurrentSession && msgIndex == state.messages.length) {
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
              showEditNav: false,
              isStreaming: true,
              streamingStatus: state.toolProgressLabel,
              streamingReasoning: state.currentStreamingReasoning,
            ),
          );
        }
        return const SizedBox.shrink();
      },
    );
  }
}
