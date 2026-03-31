class AssistantMessageRegeneration {
  final int id;
  final int messageId;
  final DateTime createdAt;
  final String oldContent;
  final String newContent;

  const AssistantMessageRegeneration({
    required this.id,
    required this.messageId,
    required this.createdAt,
    required this.oldContent,
    required this.newContent,
  });
}

