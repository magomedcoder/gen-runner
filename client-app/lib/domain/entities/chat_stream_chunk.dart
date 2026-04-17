import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/rag_document_preview.dart';

enum ChatStreamChunkKind {
  text,
  toolStatus,
  notice,
  reasoning,
  ragMeta,
  assistantFinal,
}

class AssistantStreamFinalSnapshot extends Equatable {
  const AssistantStreamFinalSnapshot({
    required this.assistantMessageId,
    required this.text,
    required this.reasoning,
  });

  final int assistantMessageId;
  final String text;
  final String reasoning;

  @override
  List<Object?> get props => [assistantMessageId, text, reasoning];
}

class ChatStreamChunk extends Equatable {
  final ChatStreamChunkKind kind;
  final String text;
  final String? toolName;
  final String? ragMode;
  final String? ragSourcesJson;
  final RagSourcesPayloadSnapshot? ragSources;
  final int messageId;
  final AssistantStreamFinalSnapshot? assistantFinal;

  const ChatStreamChunk({
    required this.kind,
    required this.text,
    this.toolName,
    this.ragMode,
    this.ragSourcesJson,
    this.ragSources,
    this.messageId = 0,
    this.assistantFinal,
  });

  @override
  List<Object?> get props => [
    kind,
    text,
    toolName,
    ragMode,
    ragSourcesJson,
    ragSources,
    messageId,
    assistantFinal,
  ];
}
