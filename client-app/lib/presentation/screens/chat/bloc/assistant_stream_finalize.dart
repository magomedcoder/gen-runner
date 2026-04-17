import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/presentation/screens/chat/bloc/assistant_chat_stream.dart';

class AssistantStreamFinalized {
  const AssistantStreamFinalized({
    required this.content,
    this.reasoningContent,
  });

  final String content;
  final String? reasoningContent;
}

AssistantStreamFinalized? finalizeAssistantStreamAccum(AssistantStreamAccum acc) {
  if (!RedactedThinkingSplit.hasAssistantPayload(
    acc.rawAssistantText,
    acc.nativeReasoning,
  )) {
    return null;
  }
  final peeled = RedactedThinkingSplit.peel(acc.rawAssistantText);
  final combined = RedactedThinkingSplit.combine(
    acc.nativeReasoning,
    peeled.$2,
  );
  return AssistantStreamFinalized(
    content: peeled.$1,
    reasoningContent: combined.isEmpty ? null : combined,
  );
}

Message assistantMessageFromStreamFinal({
  required AssistantStreamFinalized fin,
  required int streamingAssistantMessageId,
  required int fallbackMessageId,
}) {
  final id = streamingAssistantMessageId > 0
      ? streamingAssistantMessageId
      : fallbackMessageId;
  return Message(
    id: id,
    content: fin.content,
    role: MessageRole.assistant,
    createdAt: DateTime.now(),
    reasoningContent: fin.reasoningContent,
  );
}

void seedContinueAccumFromAssistantMessage(
  AssistantStreamAccum acc,
  Message previousAssistant,
) {
  acc.rawAssistantText = previousAssistant.content;
  acc.nativeReasoning = previousAssistant.reasoningContent ?? '';
  final initialPeeled = RedactedThinkingSplit.peel(acc.rawAssistantText);
  acc.nativeReasoning = RedactedThinkingSplit.combine(
    acc.nativeReasoning,
    initialPeeled.$2,
  );
  acc.rawAssistantText = initialPeeled.$1;
}
