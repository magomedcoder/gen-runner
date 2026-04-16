import 'dart:typed_data';

import 'package:equatable/equatable.dart';

enum MessageRole { user, assistant }

class Message extends Equatable {
  final int id;
  final String content;
  final MessageRole role;
  final DateTime createdAt;
  final DateTime? updatedAt;
  final String? attachmentFileName;
  final List<String> attachmentFileNames;
  final Uint8List? attachmentContent;
  final int? attachmentFileId;
  final List<int> attachmentFileIds;
  final String? reasoningContent;
  final bool useFileRag;
  final int fileRagTopK;
  final String fileRagEmbedModel;

  const Message({
    required this.id,
    required this.content,
    required this.role,
    required this.createdAt,
    this.updatedAt,
    this.attachmentFileName,
    this.attachmentFileNames = const [],
    this.attachmentContent,
    this.attachmentFileId,
    this.attachmentFileIds = const [],
    this.reasoningContent,
    this.useFileRag = false,
    this.fileRagTopK = 0,
    this.fileRagEmbedModel = '',
  });

  Map<String, dynamic> toJson() => {
    'role': role == MessageRole.user ? 'user' : 'assistant',
    'content': content,
  };

  @override
  List<Object?> get props => [
    id,
    content,
    role,
    createdAt,
    updatedAt,
    attachmentFileName,
    attachmentFileNames,
    attachmentFileId,
    attachmentFileIds,
    reasoningContent,
    useFileRag,
    fileRagTopK,
    fileRagEmbedModel,
  ];
}
