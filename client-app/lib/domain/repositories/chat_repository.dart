import 'dart:async';

import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';

abstract interface class ChatRepository {
  Future<bool> checkConnection();

  Stream<String> sendMessage(
    int sessionId,
    List<Message> messages,
  );

  Future<ChatSession> createSession(String title);

  Future<ChatSession> getSession(int sessionId);

  Future<List<ChatSession>> listSessions(int page, int pageSize);

  Future<List<Message>> getSessionMessages(
    int sessionId,
    int page,
    int pageSize,
  );

  Future<void> deleteSession(int sessionId);

  Future<ChatSession> updateSessionTitle(int sessionId, String title);

  Future<ChatSessionSettings> getSessionSettings(int sessionId);
  Future<ChatSessionSettings> updateSessionSettings({
    required int sessionId,
    required String systemPrompt,
    required List<String> stopSequences,
    required int timeoutSeconds,
    double? temperature,
    int? maxTokens,
    int? topK,
    double? topP,
    required bool jsonMode,
    required String jsonSchema,
    required String toolsJson,
    required String profile,
  });

  Future<String?> getSelectedRunner();
  Future<void> setSelectedRunner(String? runner);
  Future<String?> getDefaultRunnerModel(String runner);
  Future<void> setDefaultRunnerModel(String runner, String? model);
}
