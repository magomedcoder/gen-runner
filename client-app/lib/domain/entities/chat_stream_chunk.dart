import 'package:equatable/equatable.dart';

enum ChatStreamChunkKind { text, toolStatus, notice }

class ChatStreamChunk extends Equatable {
  final ChatStreamChunkKind kind;
  final String text;
  final String? toolName;
  final int messageId;

  const ChatStreamChunk({
    required this.kind,
    required this.text,
    this.toolName,
    this.messageId = 0,
  });

  @override
  List<Object?> get props => [kind, text, toolName, messageId];
}
