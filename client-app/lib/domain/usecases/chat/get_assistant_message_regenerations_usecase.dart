import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class GetAssistantMessageRegenerationsUseCase {
  final ChatRepository repository;

  GetAssistantMessageRegenerationsUseCase(this.repository);

  Future<List<AssistantMessageRegeneration>> call({
    required int sessionId,
    required int assistantMessageId,
  }) {
    return repository.getAssistantMessageRegenerations(
      sessionId: sessionId,
      assistantMessageId: assistantMessageId,
    );
  }
}

