import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class SendMessageUseCase {
  final ChatRepository repository;

  SendMessageUseCase(this.repository);

  Stream<String> call(
    String sessionId,
    List<Message> messages, {
    String? model,
  }) {
    return repository.sendMessage(sessionId, messages, model: model);
  }
}
