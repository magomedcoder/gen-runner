import 'package:equatable/equatable.dart';

class UserMessageEdit extends Equatable {
  final int id;
  final int messageId;
  final DateTime createdAt;
  final String oldContent;
  final String newContent;

  const UserMessageEdit({
    required this.id,
    required this.messageId,
    required this.createdAt,
    required this.oldContent,
    required this.newContent,
  });

  @override
  List<Object?> get props => [id, messageId, createdAt, oldContent, newContent];
}
