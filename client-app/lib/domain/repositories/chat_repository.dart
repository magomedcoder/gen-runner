import 'dart:async';
import 'dart:typed_data';

import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/session_file_download.dart';
import 'package:gen/domain/entities/session_messages_page.dart';

abstract interface class ChatRepository {
  Future<bool> checkConnection();

  Stream<ChatStreamChunk> sendMessage(
    int sessionId,
    Message message,
  );

  Stream<ChatStreamChunk> regenerateAssistantResponse(
    int sessionId,
    int assistantMessageId,
  );

  Stream<ChatStreamChunk> continueAssistantResponse(
    int sessionId,
    int assistantMessageId,
  );

  Stream<ChatStreamChunk> editUserMessageAndContinue(
    int sessionId,
    int userMessageId,
    String newContent,
  );

  Future<List<UserMessageEdit>> getUserMessageEdits({
    required int sessionId,
    required int userMessageId,
  });

  Future<List<Message>> getSessionMessagesForUserMessageVersion({
    required int sessionId,
    required int userMessageId,
    required int versionIndex,
  });

  Future<List<AssistantMessageRegeneration>> getAssistantMessageRegenerations({
    required int sessionId,
    required int assistantMessageId,
  });

  Future<List<Message>> getSessionMessagesForAssistantMessageVersion({
    required int sessionId,
    required int assistantMessageId,
    required int versionIndex,
  });

  Future<ChatSession> createSession(String title);

  Future<ChatSession> getSession(int sessionId);

  Future<List<ChatSession>> listSessions(int page, int pageSize);

  Future<SessionMessagesPage> getSessionMessagesPage({
    required int sessionId,
    int beforeMessageId = 0,
    int pageSize = 40,
  });

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
