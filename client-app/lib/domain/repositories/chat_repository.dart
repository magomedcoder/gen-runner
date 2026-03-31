import 'dart:async';
import 'dart:typed_data';

import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/session_file_download.dart';

abstract interface class ChatRepository {
  Future<bool> checkConnection();

  Stream<ChatStreamChunk> sendMessage(
    int sessionId,
    List<Message> messages,
  );

  Stream<ChatStreamChunk> regenerateAssistantResponse(
    int sessionId,
    int assistantMessageId,
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

  Future<int> putSessionFile({
    required int sessionId,
    required String filename,
    required List<int> content,
    int ttlSeconds = 0,
  });

  Future<SessionFileDownload> getSessionFile({
    required int sessionId,
    required int fileId,
  });

  Future<SpreadsheetApplyResult> applySpreadsheet({
    List<int>? workbookXlsx,
    required String operationsJson,
    String previewSheet,
    String previewRange,
  });

  Future<Uint8List> buildDocx({required String specJson});

  Future<String> applyMarkdownPatch({
    required String baseText,
    required String patchJson,
  });
}
