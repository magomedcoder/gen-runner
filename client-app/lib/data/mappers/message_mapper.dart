import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/generated/grpc_pb/chat.pb.dart' as grpc;

abstract class MessageMapper {
  MessageMapper._();

  static DateTime _dateTimeFromUnixSeconds(int seconds) {
    return DateTime.fromMillisecondsSinceEpoch(seconds * 1000);
  }

  static int _dateTimeToUnixSeconds(DateTime dt) {
    return dt.millisecondsSinceEpoch ~/ 1000;
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

  static String _roleToProto(MessageRole role) {
    return role == MessageRole.user ? 'user' : 'assistant';
  }

  static Message fromProto(grpc.ChatMessage proto) {
    final updatedSeconds = proto.updatedAt.toInt();
    return Message(
      id: proto.id.toInt(),
      content: proto.content,
      role: _roleFromProto(proto.role),
      createdAt: _dateTimeFromUnixSeconds(proto.createdAt.toInt()),
      updatedAt: updatedSeconds > 0 ? _dateTimeFromUnixSeconds(updatedSeconds) : null,
      attachmentFileName: proto.hasAttachmentName() ? proto.attachmentName : null,
      attachmentContent: proto.attachmentContent.isNotEmpty ? Uint8List.fromList(proto.attachmentContent) : null,
      attachmentFileId: proto.hasAttachmentFileId() ? proto.attachmentFileId.toInt() : null,
    );
  }

  static grpc.ChatMessage toProto(Message entity) {
    final p = grpc.ChatMessage();
    p.id = Int64(entity.id);
    p.content = entity.content;
    p.role = _roleToProto(entity.role);
    p.createdAt = Int64(_dateTimeToUnixSeconds(entity.createdAt));
    if (entity.updatedAt != null) {
      p.updatedAt = Int64(_dateTimeToUnixSeconds(entity.updatedAt!));
    }

    if (entity.attachmentFileName != null && entity.attachmentFileName!.isNotEmpty) {
      p.attachmentName = entity.attachmentFileName!;
    }

    if (entity.attachmentContent != null && entity.attachmentContent!.isNotEmpty) {
      p.attachmentContent = entity.attachmentContent!;
    }

    final fid = entity.attachmentFileId;
    if (fid != null && fid > 0) {
      p.attachmentFileId = Int64(fid);
    }

    return p;
  }

  static List<Message> listFromProto(List<grpc.ChatMessage> protos) {
    return protos.map(fromProto).toList();
  }

  static List<grpc.ChatMessage> listToProto(List<Message> entities) {
    return entities.map(toProto).toList();
  }
}
