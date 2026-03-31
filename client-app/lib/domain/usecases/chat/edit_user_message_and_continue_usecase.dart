import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class EditUserMessageAndContinueUseCase {
  final ChatRepository repository;

  EditUserMessageAndContinueUseCase(this.repository);

  Stream<ChatStreamChunk> call(
    int sessionId,
    int userMessageId,
    String newContent,
  ) {
    return repository.editUserMessageAndContinue(
      sessionId,
      userMessageId,
      newContent,
    );
  }
}
