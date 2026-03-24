import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class SendMessageUseCase {
  final ChatRepository repository;

  SendMessageUseCase(this.repository);

  Stream<String> call(
    int sessionId,
    List<Message> messages,
  ) {
    return repository.sendMessage(sessionId, messages);
  }
}
