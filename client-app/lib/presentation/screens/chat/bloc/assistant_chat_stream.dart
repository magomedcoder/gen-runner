import 'dart:async';

import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/rag_document_preview.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class AssistantStreamAccum {
  AssistantStreamAccum({
    this.rawAssistantText = '',
    this.nativeReasoning = '',
  });

  String rawAssistantText;
  String nativeReasoning;
}

StreamSubscription<ChatStreamChunk> subscribeAssistantChatStream(
  Stream<ChatStreamChunk> stream, {
  required void Function(ChatStreamChunk chunk) onChunk,
  required Completer<bool> completer,
}) {
  return stream.listen(
    onChunk,
    onDone: () {
      if (!completer.isCompleted) {
        completer.complete(false);
      }
    },
    onError: (Object e, StackTrace st) {
      if (!completer.isCompleted) {
        completer.completeError(e, st);
      }
    },
    cancelOnError: false,
  );
}

void handleAssistantChatStreamChunk({
  required ChatStreamChunk chunk,
  required Emitter<ChatState> emit,
  required ChatState Function() currentState,
  required bool Function() isCurrentSession,
  required AssistantStreamAccum acc,
  required void Function(int messageId) onAssistantMessageId,
}) {
  if (chunk.kind == ChatStreamChunkKind.assistantFinal) {
    final snap = chunk.assistantFinal;
    if (snap != null) {
      acc.rawAssistantText = snap.text;
      acc.nativeReasoning = snap.reasoning;
      if (snap.assistantMessageId > 0) {
        onAssistantMessageId(snap.assistantMessageId);
      }
      if (isCurrentSession()) {
        final peeled = RedactedThinkingSplit.peel(acc.rawAssistantText);
        final combined = RedactedThinkingSplit.combine(
          acc.nativeReasoning,
          peeled.$2,
        );
        emit(
          currentState().copyWith(
            currentStreamingText: peeled.$1,
            currentStreamingReasoning: combined.isEmpty ? null : combined,
            clearToolProgress: true,
          ),
        );
      }
    }
    return;
  }
  if (chunk.kind == ChatStreamChunkKind.toolStatus) {
    final line = chunk.text.trim().isNotEmpty
        ? chunk.text
        : (chunk.toolName ?? 'инструмент');
    if (isCurrentSession()) {
      emit(currentState().copyWith(toolProgressLabel: line));
    }
    return;
  }
  if (chunk.kind == ChatStreamChunkKind.ragMeta) {
    if (isCurrentSession()) {
      final preview = RagDocumentPreview.tryParse(
        summary: chunk.text,
        sourcesJson: chunk.ragSourcesJson,
        modeFromStream: chunk.ragMode,
        ragSources: chunk.ragSources,
      );
      if (preview != null) {
        emit(
          currentState().copyWith(
            ragDocumentPreview: preview,
            clearToolProgress: true,
          ),
        );
      }
    }
    return;
  }
  if (chunk.kind == ChatStreamChunkKind.notice) {
    final t = chunk.text.trim();
    if (t.isNotEmpty && isCurrentSession()) {
      emit(currentState().copyWith(streamNotice: t));
    }
    return;
  }
  if (chunk.kind == ChatStreamChunkKind.reasoning) {
    if (chunk.messageId > 0) {
      onAssistantMessageId(chunk.messageId);
    }
    acc.nativeReasoning += chunk.text;
    if (isCurrentSession()) {
      final peeled = RedactedThinkingSplit.peel(acc.rawAssistantText);
      final combined = RedactedThinkingSplit.combine(
        acc.nativeReasoning,
        peeled.$2,
      );
      emit(
        currentState().copyWith(
          currentStreamingText: peeled.$1,
          currentStreamingReasoning: combined.isEmpty ? null : combined,
          clearToolProgress: true,
        ),
      );
    }
    return;
  }

  if (chunk.messageId > 0) {
    onAssistantMessageId(chunk.messageId);
  }

  acc.rawAssistantText += chunk.text;
  final peeled = RedactedThinkingSplit.peel(acc.rawAssistantText);
  final combined = RedactedThinkingSplit.combine(
    acc.nativeReasoning,
    peeled.$2,
  );
  emit(
    currentState().copyWith(
      currentStreamingText: peeled.$1,
      currentStreamingReasoning: combined.isEmpty ? null : combined,
      clearToolProgress: true,
    ),
  );
}
