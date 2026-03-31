import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class GetUserMessageEditsUseCase {
  final ChatRepository repository;

  GetUserMessageEditsUseCase(this.repository);

  Future<List<UserMessageEdit>> call({
    required int sessionId,
    required int userMessageId,
  }) {
    return repository.getUserMessageEdits(
      sessionId: sessionId,
      userMessageId: userMessageId,
    );
  }
}
