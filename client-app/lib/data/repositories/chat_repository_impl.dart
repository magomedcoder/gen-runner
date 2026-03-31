import 'dart:async';
import 'dart:typed_data';

import 'package:gen/core/failures.dart';
import 'package:gen/data/data_sources/remote/chat_remote_datasource.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/session_file_download.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class ChatRepositoryImpl implements ChatRepository {
  final IChatRemoteDataSource dataSource;

  ChatRepositoryImpl(this.dataSource);

  @override
  Future<bool> checkConnection() async {
    try {
      return await dataSource.checkConnection();
    } catch (e) {
      throw NetworkFailure('Ошибка проверки подключения: $e');
    }
  }

  @override
  Stream<ChatStreamChunk> sendMessage(
    int sessionId,
    List<Message> messages,
  ) {
    try {
      return dataSource.sendChatMessage(sessionId, messages);
    } catch (e) {
      throw ApiFailure('Ошибка создания потока сообщений: $e');
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
    } catch (e) {
      throw ApiFailure('Ошибка перегенерации: $e');
    }
  }

  @override
  Future<ChatSession> createSession(String title) async {
    try {
      return await dataSource.createSession(title);
    } catch (e) {
      throw ApiFailure('Ошибка создания сессии: $e');
    }
  }

  @override
  Future<ChatSession> getSession(int sessionId) async {
    try {
      return await dataSource.getSession(sessionId);
    } catch (e) {
      throw ApiFailure('Ошибка получения сессии: $e');
    }
  }

  @override
  Future<List<ChatSession>> listSessions(int page, int pageSize) async {
    try {
      return await dataSource.getSessions(page, pageSize);
    } catch (e) {
      throw ApiFailure('Ошибка получения списка сессий: $e');
    }
  }

  @override
  Future<List<Message>> getSessionMessages(
    int sessionId,
    int page,
    int pageSize,
  ) async {
    try {
      return await dataSource.getSessionMessages(sessionId, page, pageSize);
    } catch (e) {
      throw ApiFailure('Ошибка получения сообщений сессии: $e');
    }
  }

  @override
  Future<void> deleteSession(int sessionId) async {
    try {
      await dataSource.deleteSession(sessionId);
    } catch (e) {
      throw ApiFailure('Ошибка удаления сессии: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionTitle(int sessionId, String title) async {
    try {
      return await dataSource.updateSessionTitle(sessionId, title);
    } catch (e) {
      throw ApiFailure('Ошибка обновления заголовка сессии: $e');
    }
  }

  @override
  Future<ChatSessionSettings> getSessionSettings(int sessionId) async {
    try {
      return await dataSource.getSessionSettings(sessionId);
    } catch (e) {
      throw ApiFailure('Ошибка загрузки настроек чата: $e');
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
    } catch (e) {
      throw ApiFailure('Ошибка сохранения настроек чата: $e');
    }
  }

  @override
  Future<String?> getSelectedRunner() async {
    try {
      return await dataSource.getSelectedRunner();
    } catch (e) {
      throw ApiFailure('Ошибка получения выбранного раннера: $e');
    }
  }

  @override
  Future<void> setSelectedRunner(String? runner) async {
    try {
      await dataSource.setSelectedRunner(runner);
    } catch (e) {
      throw ApiFailure('Ошибка сохранения выбранного раннера: $e');
    }
  }

  @override
  Future<String?> getDefaultRunnerModel(String runner) async {
    try {
      return await dataSource.getDefaultRunnerModel(runner);
    } catch (e) {
      throw ApiFailure('Ошибка получения модели по умолчанию: $e');
    }
  }

  @override
  Future<void> setDefaultRunnerModel(String runner, String? model) async {
    try {
      await dataSource.setDefaultRunnerModel(runner, model);
    } catch (e) {
      throw ApiFailure('Ошибка сохранения модели по умолчанию: $e');
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
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка загрузки файла сессии: $e');
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
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка получения файла сессии: $e');
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
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка таблицы: $e');
    }
  }

  @override
  Future<Uint8List> buildDocx({required String specJson}) async {
    try {
      return await dataSource.buildDocx(specJson: specJson);
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка документа Word: $e');
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
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка патча текста: $e');
    }
  }
}
