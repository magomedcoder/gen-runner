import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class GetSessionMessagesForUserMessageVersionUseCase {
  final ChatRepository repository;

  GetSessionMessagesForUserMessageVersionUseCase(this.repository);

  Future<List<Message>> call({
    required int sessionId,
    required int userMessageId,
    required int versionIndex,
  }) {
    return repository.getSessionMessagesForUserMessageVersion(
      sessionId: sessionId,
      userMessageId: userMessageId,
      versionIndex: versionIndex,
    );
  }
}
