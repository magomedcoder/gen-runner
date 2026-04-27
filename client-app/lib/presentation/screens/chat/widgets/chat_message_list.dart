import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/core/tool_chain_partition.dart';
import 'package:gen/presentation/widgets/chat_bubble.dart';
import 'package:gen/presentation/widgets/consolidated_tool_round_bubble.dart';
import 'package:gen/presentation/widgets/tool_results_batch_bubble.dart';

String _contentForVersion(
  Message msg,
  List<UserMessageEdit> edits,
  int versionIdx,
) {
  if (edits.isEmpty) {
    return msg.content;
  }

  if (versionIdx <= 0) {
    return edits.first.oldContent;
  }

  final i = (versionIdx - 1).clamp(0, edits.length - 1);

  return edits[i].newContent;
}

String _assistantContentForVersion(
  Message msg,
  List<AssistantMessageRegeneration> regens,
  int versionIdx,
) {
  if (regens.isEmpty) {
    return msg.content;
  }

  if (versionIdx <= 0) {
    return regens.first.oldContent;
  }

  final i = (versionIdx - 1).clamp(0, regens.length - 1);

  return regens[i].newContent;
}

sealed class _ChatListItem {
  const _ChatListItem();
}

class _ChatListSingle extends _ChatListItem {
  const _ChatListSingle(this.messageIndex);
  final int messageIndex;
}

class _ChatListToolBatch extends _ChatListItem {
  const _ChatListToolBatch(this.start, this.end);
  final int start;
  final int end;
}

class _ChatListToolChain extends _ChatListItem {
  const _ChatListToolChain(this.chain);
  final ToolChainIndices chain;
}

List<_ChatListItem> _buildChatListItems(List<Message> messages) {
  final parts = partitionMessagesForToolChainUi(messages);
  final raw = <_ChatListItem>[];
  for (final p in parts) {
    switch (p) {
      case PartitionSingle(:final index):
        raw.add(_ChatListSingle(index));
      case PartitionToolChain(:final chain):
        raw.add(_ChatListToolChain(chain));
    }
  }

  return _coalesceConsecutiveToolSingles(raw, messages);
}

List<_ChatListItem> _coalesceConsecutiveToolSingles(
  List<_ChatListItem> raw,
  List<Message> msgs,
) {
  final out = <_ChatListItem>[];
  var i = 0;
  while (i < raw.length) {
    final it = raw[i];
    if (it is _ChatListSingle && msgs[it.messageIndex].role == MessageRole.tool) {
      final start = it.messageIndex;
      var end = start;
      var j = i + 1;
      while (j < raw.length) {
        final nx = raw[j];
        if (nx is! _ChatListSingle) {
          break;
        }

        if (msgs[nx.messageIndex].role != MessageRole.tool) {
          break;
        }

        if (nx.messageIndex != end + 1) {
          break;
        }

        end = nx.messageIndex;
        j++;
      }

      if (start == end) {
        out.add(it);
        i++;
      } else {
        out.add(_ChatListToolBatch(start, end));
        i = j;
      }

      continue;
    }

    out.add(it);
    i++;
  }

  return out;
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
    final listItems = _buildChatListItems(state.messages);
    final n = listItems.length;
    final hasStream = state.isStreamingInCurrentSession;
    final hasOlder = state.isLoadingOlderMessages;
    final childCount = n + (hasStream ? 1 : 0) + (hasOlder ? 1 : 0);

    Widget rowForListIndex(int rowIndex) {
      final item = listItems[rowIndex];
      return switch (item) {
        _ChatListToolBatch(:final start, :final end) => Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: ToolResultsBatchBubble(
            tools: state.messages.sublist(start, end + 1).toList(growable: false),
          ),
        ),
        final _ChatListToolChain c => _toolChainRow(context, state, c.chain),
        _ChatListSingle(:final messageIndex) => _messageRow(context, state, messageIndex),
      };
    }

    Widget streamingRow() {
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
          ragPreviewBySessionFile: state.ragPreviewBySessionFile,
          showEditNav: false,
          isStreaming: true,
          streamingStatus: state.toolProgressLabel,
          streamingReasoning: state.currentStreamingReasoning,
        ),
      );
    }

    const olderSpinner = Padding(
      padding: EdgeInsets.only(bottom: 8),
      child: Center(
        child: SizedBox(
          width: 24,
          height: 24,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
      ),
    );

    return ListView.builder(
      reverse: true,
      controller: scrollController,
      padding: EdgeInsets.symmetric(
        vertical: 16,
        horizontal: horizontalPadding,
      ),
      itemCount: childCount,
      itemBuilder: (context, index) {
        if (hasStream && index == 0) {
          return streamingRow();
        }

        final streamOff = hasStream ? 1 : 0;
        final i = index - streamOff;
        if (i < n) {
          final rowIndex = n - 1 - i;
          return rowForListIndex(rowIndex);
        }

        if (hasOlder && index == childCount - 1) {
          return olderSpinner;
        }

        return const SizedBox.shrink();
      },
    );
  }
}

Widget _toolChainRow(
  BuildContext context,
  ChatState state,
  ToolChainIndices chain,
) {
  final segments = chain.segments.map((s) {
    final lead = state.messages[s.leadIndex];
    final tools = state.messages.sublist(s.toolStart, s.toolEnd + 1).toList(growable: false);
    return ToolChainSegmentView(leadAssistant: lead, toolMessages: tools);
  }).toList();

  final tailIdx = chain.finalAssistantIndex;
  final tail = tailIdx != null ? state.messages[tailIdx] : null;

  final lastIdx = tailIdx ?? (chain.segments.isEmpty ? -1 : chain.segments.last.toolEnd);
  final canRegenerate = !state.isStreamingInCurrentSession &&
      lastIdx >= 0 &&
      lastIdx == state.messages.length - 1 &&
      tail != null &&
      tail.role == MessageRole.assistant &&
      tail.id > 0;

  if (tail == null) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: ConsolidatedToolRoundBubble(
        segments: segments,
      ),
    );
  }

  final msg = tail;
  final msgIndex = tailIdx!;
  final regens = state.regenerationsByMessageId[msg.id];
  final regenCursor = state.regenerationCursorByMessageId[msg.id];
  final hasRegens = regens != null && regens.isNotEmpty;
  final regenVersionsCount = hasRegens ? regens.length + 1 : 1;
  final regenVersionIdx = hasRegens
      ? (regenCursor ?? (regenVersionsCount - 1)).clamp(0, regenVersionsCount - 1)
      : 0;
  final displayTail = hasRegens
      ? Message(
          id: msg.id,
          content: _assistantContentForVersion(
            msg,
            regens,
            regenVersionIdx,
          ),
          role: msg.role,
          createdAt: msg.createdAt,
          updatedAt: msg.updatedAt,
          attachmentFileName: msg.attachmentFileName,
          attachmentFileNames: msg.attachmentFileNames,
          attachmentMime: msg.attachmentMime,
          attachmentContent: msg.attachmentContent,
          attachmentFileId: msg.attachmentFileId,
          attachmentFileIds: msg.attachmentFileIds,
          reasoningContent: msg.reasoningContent,
          toolCallId: msg.toolCallId,
          toolName: msg.toolName,
          toolCallsJson: msg.toolCallsJson,
          useFileRag: msg.useFileRag,
          fileRagTopK: msg.fileRagTopK,
          fileRagEmbedModel: msg.fileRagEmbedModel,
        )
      : msg;

  final showAssistantNav = state.regeneratedAssistantMessageIds.contains(msg.id) || hasRegens;

  final showContinuePartial = !state.isStreamingInCurrentSession &&
      msgIndex == state.messages.length - 1 &&
      msg.role == MessageRole.assistant &&
      msg.id > 0 &&
      state.partialAssistantMessageId != null &&
      state.partialAssistantMessageId == msg.id;

  return Padding(
    padding: const EdgeInsets.only(bottom: 8),
    child: ConsolidatedToolRoundBubble(
      segments: segments,
      finalAssistant: displayTail,
      showContinuePartial: showContinuePartial,
      onContinueAssistant: showContinuePartial && msg.id > 0
        ? () => context.read<ChatBloc>().add(ChatContinueAssistant(msg.id))
        : null,
      onRegenerate: canRegenerate
        ? () => context.read<ChatBloc>().add(
          ChatRegenerateAssistant(msg.id),
        )
        : null,
      showEditNav: showAssistantNav,
      editsTotal: showAssistantNav ? regenVersionsCount : null,
      editsIndex: showAssistantNav ? regenVersionIdx : null,
      onPrevEdit: showAssistantNav
        ? ((hasRegens && regenVersionIdx <= 0)
          ? null
          : () => context.read<ChatBloc>().add(
            ChatNavigateAssistantMessageRegeneration(
              msg.id,
              -1,
            ),
          ))
        : null,
      onNextEdit: showAssistantNav
        ? ((hasRegens && regenVersionIdx >= regenVersionsCount - 1)
          ? null
          : () => context.read<ChatBloc>().add(
            ChatNavigateAssistantMessageRegeneration(
              msg.id,
              1,
            ),
          ))
        : null,
    ),
  );
}

Widget _messageRow(
  BuildContext context,
  ChatState state,
  int msgIndex,
) {
  final msg = state.messages[msgIndex];
  final canRegenerate = !state.isStreamingInCurrentSession &&
      msgIndex == state.messages.length - 1 &&
      msg.role == MessageRole.assistant &&
      msg.id > 0;
  final canEdit = !state.isStreamingInCurrentSession && msg.role == MessageRole.user && msg.id > 0;

  final edits = canEdit ? state.editsByMessageId[msg.id] : null;
  final cursor = canEdit ? state.editCursorByMessageId[msg.id] : null;
  final hasEdits = edits != null && edits.isNotEmpty;
  final isEdited = canEdit && (state.editedMessageIds.contains(msg.id) || hasEdits || (msg.updatedAt != null && msg.updatedAt!.millisecondsSinceEpoch != msg.createdAt.millisecondsSinceEpoch));
  final versionsCount = hasEdits ? edits.length + 1 : 1;
  final versionIdx = hasEdits
      ? (cursor ?? (versionsCount - 1)).clamp(0, versionsCount - 1)
      : 0;
  final displayMsg = (canEdit && hasEdits)
    ? Message(
      id: msg.id,
      content: _contentForVersion(msg, edits, versionIdx),
      role: msg.role,
      createdAt: msg.createdAt,
      updatedAt: msg.updatedAt,
      attachmentFileName: msg.attachmentFileName,
      attachmentFileNames: msg.attachmentFileNames,
      attachmentMime: msg.attachmentMime,
      attachmentContent: msg.attachmentContent,
      attachmentFileId: msg.attachmentFileId,
      attachmentFileIds: msg.attachmentFileIds,
      reasoningContent: msg.reasoningContent,
      toolCallId: msg.toolCallId,
      toolName: msg.toolName,
      toolCallsJson: msg.toolCallsJson,
      useFileRag: msg.useFileRag,
      fileRagTopK: msg.fileRagTopK,
      fileRagEmbedModel: msg.fileRagEmbedModel,
    )
    : msg;

  final regens = msg.role == MessageRole.assistant
      ? state.regenerationsByMessageId[msg.id]
      : null;
  final regenCursor = msg.role == MessageRole.assistant
      ? state.regenerationCursorByMessageId[msg.id]
      : null;
  final hasRegens = regens != null && regens.isNotEmpty;
  final regenVersionsCount = hasRegens ? regens.length + 1 : 1;
  final regenVersionIdx = hasRegens
      ? (regenCursor ?? (regenVersionsCount - 1)).clamp(0, regenVersionsCount - 1)
      : 0;
  final displayAssistantMsg = (msg.role == MessageRole.assistant && hasRegens)
    ? Message(
      id: msg.id,
      content: _assistantContentForVersion(
        msg,
        regens,
        regenVersionIdx,
      ),
      role: msg.role,
      createdAt: msg.createdAt,
      updatedAt: msg.updatedAt,
      attachmentFileName: msg.attachmentFileName,
      attachmentFileNames: msg.attachmentFileNames,
      attachmentMime: msg.attachmentMime,
      attachmentContent: msg.attachmentContent,
      attachmentFileId: msg.attachmentFileId,
      attachmentFileIds: msg.attachmentFileIds,
      reasoningContent: msg.reasoningContent,
      toolCallId: msg.toolCallId,
      toolName: msg.toolName,
      toolCallsJson: msg.toolCallsJson,
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
      ragPreviewBySessionFile: state.ragPreviewBySessionFile,
      showContinuePartial: showContinuePartial,
      showEditNav: isEdited || showAssistantNav,
      onRegenerate: canRegenerate
        ? () => context.read<ChatBloc>().add(ChatRegenerateAssistant(msg.id))
        : null,
      onEditSubmit: canEdit ? (newText) async {
          context.read<ChatBloc>().add(ChatEditUserMessageAndContinue(msg.id, newText));
        } : null,
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
          : () => context.read<ChatBloc>().add(
            ChatNavigateAssistantMessageRegeneration(msg.id, -1),
          ))
        : null),
      onNextEdit: isEdited
        ? ((!canEdit || (hasEdits && versionIdx >= versionsCount - 1))
          ? null
          : () => context.read<ChatBloc>().add(
            ChatNavigateUserMessageEdit(msg.id, 1),
          ))
          : (showAssistantNav ? ((hasRegens && regenVersionIdx >= regenVersionsCount - 1)
            ? null
            : () => context.read<ChatBloc>().add(
              ChatNavigateAssistantMessageRegeneration(
                msg.id,
                1,
              ),
            ))
          : null),
    ),
  );
}
