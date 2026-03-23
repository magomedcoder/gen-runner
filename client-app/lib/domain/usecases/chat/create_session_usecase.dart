import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class CreateSessionUseCase {
  CreateSessionUseCase(this.repository);

  final ChatRepository repository;

  static String defaultTitle([DateTime? at]) {
    final now = at ?? DateTime.now();
    String z2(int n) => n.toString().padLeft(2, '0');
    return '${z2(now.hour)}:${z2(now.minute)}:${z2(now.second)} ${z2(now.day)}.${z2(now.month)}.${now.year}';
  }

  Future<ChatSession> call({String? title, String? model}) async {
    final sessionTitle = title ?? defaultTitle();
    return repository.createSession(sessionTitle, model: model);
  }
}
