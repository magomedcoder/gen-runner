import 'dart:async';

import 'package:gen/core/failures.dart';
import 'package:gen/data/data_sources/local/session_model_local_data_source.dart';
import 'package:gen/data/data_sources/remote/chat_remote_datasource.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class ChatRepositoryImpl implements ChatRepository {
  final IChatRemoteDataSource dataSource;
  final SessionModelLocalDataSource localDataSource;

  ChatRepositoryImpl(this.dataSource, this.localDataSource);

  @override
  Future<bool> checkConnection() async {
    try {
      return await dataSource.checkConnection();
    } catch (e) {
      throw NetworkFailure('Ошибка проверки подключения: $e');
    }
  }

  @override
  Future<List<String>> getModels() async {
    try {
      return await dataSource.getModels();
    } catch (e) {
      throw ApiFailure('Ошибка получения списка моделей: $e');
    }
  }

  @override
  Stream<String> sendMessage(
    String sessionId,
    List<Message> messages, {
    String? model,
  }) {
    try {
      return dataSource.sendChatMessage(
        sessionId,
        messages,
        model: model,
      );
    } catch (e) {
      throw ApiFailure('Ошибка создания потока сообщений: $e');
    }
  }

  @override
  Future<ChatSession> createSession(String title, {String? model}) async {
    try {
      return await dataSource.createSession(title, model: model);
    } catch (e) {
      throw ApiFailure('Ошибка создания сессии: $e');
    }
  }

  @override
  Future<ChatSession> getSession(String sessionId) async {
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
    String sessionId,
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
  Future<void> deleteSession(String sessionId) async {
    try {
      await dataSource.deleteSession(sessionId);
    } catch (e) {
      throw ApiFailure('Ошибка удаления сессии: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionTitle(String sessionId, String title) async {
    try {
      return await dataSource.updateSessionTitle(sessionId, title);
    } catch (e) {
      throw ApiFailure('Ошибка обновления заголовка сессии: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionModel(String sessionId, String model) async {
    try {
      return await dataSource.updateSessionModel(sessionId, model);
    } catch (e) {
      throw ApiFailure('Ошибка обновления модели сессии: $e');
    }
  }

  @override
  Future<String?> getSessionModel(String sessionId) async {
    try {
      return await localDataSource.getSessionModel(sessionId);
    } catch (e) {
      return null;
    }
  }

  @override
  Future<void> setSessionModel(String sessionId, String model) async {
    try {
      await localDataSource.setSessionModel(sessionId, model);
    } catch (e) {
      throw ApiFailure('Ошибка сохранения модели сессии: $e');
    }
  }
}
