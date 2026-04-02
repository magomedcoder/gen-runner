import 'dart:async';
import 'dart:typed_data';

import 'package:gen/core/failures.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/data/data_sources/remote/chat_remote_datasource.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/session_file_download.dart';
import 'package:gen/domain/entities/session_messages_page.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class ChatRepositoryImpl implements ChatRepository {
  final IChatRemoteDataSource dataSource;

  ChatRepositoryImpl(this.dataSource);

  @override
  Future<bool> checkConnection() async {
    try {
      return await dataSource.checkConnection();
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: checkConnection', exception: e, stackTrace: st);
      throw NetworkFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка проверки подключения'),
      );
    }
  }

  @override
  Stream<ChatStreamChunk> sendMessage(
    int sessionId,
    Message message,
  ) {
    try {
      return dataSource.sendChatMessage(sessionId, message);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: sendMessage stream', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка создания потока сообщений'),
      );
    }
  }

  @override
  Stream<ChatStreamChunk> regenerateAssistantResponse(
    int sessionId,
    int assistantMessageId,
  ) {
    try {
      return dataSource.regenerateAssistantResponse(
        sessionId,
        assistantMessageId,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: regenerate', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка перегенерации'));
    }
  }

  @override
  Stream<ChatStreamChunk> continueAssistantResponse(
    int sessionId,
    int assistantMessageId,
  ) {
    try {
      return dataSource.continueAssistantResponse(
        sessionId,
        assistantMessageId,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: continueAssistant', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка продолжения ответа'),
      );
    }
  }

  @override
  Stream<ChatStreamChunk> editUserMessageAndContinue(
    int sessionId,
    int userMessageId,
    String newContent,
  ) {
    try {
      return dataSource.editUserMessageAndContinue(
        sessionId,
        userMessageId,
        newContent,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: editUserMessage', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка редактирования сообщения'),
      );
    }
  }

  @override
  Future<List<UserMessageEdit>> getUserMessageEdits({
    required int sessionId,
    required int userMessageId,
  }) async {
    try {
      return await dataSource.getUserMessageEdits(
        sessionId: sessionId,
        userMessageId: userMessageId,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getUserMessageEdits', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки истории правок'),
      );
    }
  }

  @override
  Future<List<Message>> getSessionMessagesForUserMessageVersion({
    required int sessionId,
    required int userMessageId,
    required int versionIndex,
  }) async {
    try {
      return await dataSource.getSessionMessagesForUserMessageVersion(
        sessionId: sessionId,
        userMessageId: userMessageId,
        versionIndex: versionIndex,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSessionMessagesForUserMessageVersion', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки ветки сообщений'),
      );
    }
  }

  @override
  Future<List<AssistantMessageRegeneration>> getAssistantMessageRegenerations({
    required int sessionId,
    required int assistantMessageId,
  }) async {
    try {
      return await dataSource.getAssistantMessageRegenerations(
        sessionId: sessionId,
        assistantMessageId: assistantMessageId,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getAssistantMessageRegenerations', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки истории перегенераций'),
      );
    }
  }

  @override
  Future<List<Message>> getSessionMessagesForAssistantMessageVersion({
    required int sessionId,
    required int assistantMessageId,
    required int versionIndex,
  }) async {
    try {
      return await dataSource.getSessionMessagesForAssistantMessageVersion(
        sessionId: sessionId,
        assistantMessageId: assistantMessageId,
        versionIndex: versionIndex,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSessionMessagesForAssistantMessageVersion', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки версии ответа'),
      );
    }
  }

  @override
  Future<ChatSession> createSession(String title) async {
    try {
      return await dataSource.createSession(title);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: createSession', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка создания сессии'));
    }
  }

  @override
  Future<ChatSession> getSession(int sessionId) async {
    try {
      return await dataSource.getSession(sessionId);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSession', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка получения сессии'));
    }
  }

  @override
  Future<List<ChatSession>> listSessions(int page, int pageSize) async {
    try {
      return await dataSource.getSessions(page, pageSize);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: listSessions', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения списка сессий'),
      );
    }
  }

  @override
  Future<SessionMessagesPage> getSessionMessagesPage({
    required int sessionId,
    int beforeMessageId = 0,
    int pageSize = 40,
  }) async {
    try {
      return await dataSource.getSessionMessagesPage(
        sessionId: sessionId,
        beforeMessageId: beforeMessageId,
        pageSize: pageSize,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSessionMessagesPage', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения сообщений сессии'),
      );
    }
  }

  @override
  Future<void> deleteSession(int sessionId) async {
    try {
      await dataSource.deleteSession(sessionId);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: deleteSession', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка удаления сессии'));
    }
  }

  @override
  Future<ChatSession> updateSessionTitle(int sessionId, String title) async {
    try {
      return await dataSource.updateSessionTitle(sessionId, title);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: updateSessionTitle', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка обновления заголовка сессии'),
      );
    }
  }

  @override
  Future<ChatSessionSettings> getSessionSettings(int sessionId) async {
    try {
      return await dataSource.getSessionSettings(sessionId);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSessionSettings', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки настроек чата'),
      );
    }
  }

  @override
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
  }) async {
    try {
      return await dataSource.updateSessionSettings(
        sessionId: sessionId,
        systemPrompt: systemPrompt,
        stopSequences: stopSequences,
        timeoutSeconds: timeoutSeconds,
        temperature: temperature,
        topK: topK,
        topP: topP,
        jsonMode: jsonMode,
        jsonSchema: jsonSchema,
        toolsJson: toolsJson,
        profile: profile,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: updateSessionSettings', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка сохранения настроек чата'),
      );
    }
  }

  @override
  Future<String?> getSelectedRunner() async {
    try {
      return await dataSource.getSelectedRunner();
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSelectedRunner', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения выбранного раннера'),
      );
    }
  }

  @override
  Future<void> setSelectedRunner(String? runner) async {
    try {
      await dataSource.setSelectedRunner(runner);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: setSelectedRunner', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка сохранения выбранного раннера'),
      );
    }
  }

  @override
  Future<String?> getDefaultRunnerModel(String runner) async {
    try {
      return await dataSource.getDefaultRunnerModel(runner);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getDefaultRunnerModel', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения модели по умолчанию'),
      );
    }
  }

  @override
  Future<void> setDefaultRunnerModel(String runner, String? model) async {
    try {
      await dataSource.setDefaultRunnerModel(runner, model);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: setDefaultRunnerModel', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка сохранения модели по умолчанию'),
      );
    }
  }

  @override
  Future<int> putSessionFile({
    required int sessionId,
    required String filename,
    required List<int> content,
    int ttlSeconds = 0,
  }) async {
    try {
      return await dataSource.putSessionFile(
        sessionId: sessionId,
        filename: filename,
        content: content,
        ttlSeconds: ttlSeconds,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: putSessionFile', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка загрузки файла сессии'),
      );
    }
  }

  @override
  Future<SessionFileDownload> getSessionFile({
    required int sessionId,
    required int fileId,
  }) async {
    try {
      return await dataSource.getSessionFile(
        sessionId: sessionId,
        fileId: fileId,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: getSessionFile', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения файла сессии'),
      );
    }
  }

  @override
  Future<SpreadsheetApplyResult> applySpreadsheet({
    List<int>? workbookXlsx,
    required String operationsJson,
    String previewSheet = '',
    String previewRange = '',
  }) async {
    try {
      return await dataSource.applySpreadsheet(
        workbookXlsx: workbookXlsx,
        operationsJson: operationsJson,
        previewSheet: previewSheet,
        previewRange: previewRange,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: applySpreadsheet', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка таблицы'));
    }
  }

  @override
  Future<Uint8List> buildDocx({required String specJson}) async {
    try {
      return await dataSource.buildDocx(specJson: specJson);
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: buildDocx', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка документа Word'));
    }
  }

  @override
  Future<String> applyMarkdownPatch({
    required String baseText,
    required String patchJson,
  }) async {
    try {
      return await dataSource.applyMarkdownPatch(
        baseText: baseText,
        patchJson: patchJson,
      );
    } catch (e, st) {
      if (e is Failure) rethrow;
      Logs().e('ChatRepository: applyMarkdownPatch', exception: e, stackTrace: st);
      throw ApiFailure(userSafeErrorMessage(e, fallback: 'Ошибка патча текста'));
    }
  }
}
