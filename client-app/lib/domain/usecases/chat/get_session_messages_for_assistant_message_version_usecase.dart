import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class GetSessionMessagesForAssistantMessageVersionUseCase {
  final ChatRepository repository;

  GetSessionMessagesForAssistantMessageVersionUseCase(this.repository);

  Future<List<Message>> call({
    required int sessionId,
    required int assistantMessageId,
    required int versionIndex,
  }) {
    return repository.getSessionMessagesForAssistantMessageVersion(
      sessionId: sessionId,
      assistantMessageId: assistantMessageId,
      versionIndex: versionIndex,
    );
  }
}

