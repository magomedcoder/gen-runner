import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class RegenerateAssistantUseCase {
  final ChatRepository repository;

  RegenerateAssistantUseCase(this.repository);

  Stream<ChatStreamChunk> call(int sessionId, int assistantMessageId) {
    return repository.regenerateAssistantResponse(sessionId, assistantMessageId);
  }
}
