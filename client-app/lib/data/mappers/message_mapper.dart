import 'dart:typed_data';

import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/generated/grpc_pb/chat.pb.dart' as grpc;

abstract class MessageMapper {
  MessageMapper._();

  static DateTime _dateTimeFromUnixSeconds(int seconds) {
    return DateTime.fromMillisecondsSinceEpoch(seconds * 1000);
  }

  static MessageRole _roleFromProto(String role) {
    switch (role.trim().toLowerCase()) {
      case 'user':
        return MessageRole.user;
      case 'assistant':
        return MessageRole.assistant;
      default:
        return MessageRole.user;
    }
  }

  static Message fromProto(grpc.ChatMessage proto) {
    final updatedSeconds = proto.updatedAt.toInt();
    final role = _roleFromProto(proto.role);
    var content = proto.content;
    String? reasoningFromTags;
    if (role == MessageRole.assistant) {
      final peeled = RedactedThinkingSplit.peel(content);
      content = peeled.$1;
      reasoningFromTags = peeled.$2;
    }
    return Message(
      id: proto.id.toInt(),
      content: content,
      role: role,
      createdAt: _dateTimeFromUnixSeconds(proto.createdAt.toInt()),
      updatedAt: updatedSeconds > 0
          ? _dateTimeFromUnixSeconds(updatedSeconds)
          : null,
      attachmentFileName: proto.hasAttachmentName()
          ? proto.attachmentName
          : null,
      attachmentFileNames: proto.hasAttachmentName()
          ? [proto.attachmentName]
          : const [],
      attachmentContent: proto.attachmentContent.isNotEmpty
          ? Uint8List.fromList(proto.attachmentContent)
          : null,
      attachmentFileId: proto.hasAttachmentFileId()
          ? proto.attachmentFileId.toInt()
          : null,
      attachmentFileIds: proto.hasAttachmentFileId()
          ? [proto.attachmentFileId.toInt()]
          : const [],
      reasoningContent: reasoningFromTags,
      useFileRag: false,
      fileRagTopK: 0,
      fileRagEmbedModel: '',
    );
  }

  static List<Message> listFromProto(List<grpc.ChatMessage> protos) {
    return protos.map(fromProto).toList();
  }
}
