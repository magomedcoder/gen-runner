import 'dart:async';

import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/data/mappers/message_mapper.dart';
import 'package:gen/data/mappers/session_mapper.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/generated/grpc_pb/chat.pbgrpc.dart' as grpc;

abstract class IChatRemoteDataSource {
  Future<bool> checkConnection();

  Future<List<String>> getModels();

  Stream<String> sendChatMessage(
    String sessionId,
    List<Message> messages, {
    String? model,
  });

  Future<ChatSession> createSession(String title, {String? model});

  Future<ChatSession> getSession(String sessionId);

  Future<List<ChatSession>> getSessions(int page, int pageSize);

  Future<List<Message>> getSessionMessages(
    String sessionId,
    int page,
    int pageSize,
  );

  Future<void> deleteSession(String sessionId);

  Future<ChatSession> updateSessionTitle(String sessionId, String title);

  Future<ChatSession> updateSessionModel(String sessionId, String model);
}

class ChatRemoteDataSource implements IChatRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  ChatRemoteDataSource(this._channelManager, this._authGuard);

  grpc.ChatServiceClient get _client => _channelManager.chatClient;

  @override
  Future<bool> checkConnection() async {
    Logs().d('ChatRemote: checkConnection');
    try {
      final response = await _authGuard.execute(
        () => _client.checkConnection(grpc.Empty()),
      );
      Logs().i('ChatRemote: checkConnection isConnected=${response.isConnected}');
      return response.isConnected;
    } on GrpcError catch (e) {
      if (e.code == StatusCode.unavailable) {
        return false;
      }
      Logs().e('ChatRemote: checkConnection', exception: e);
      throw NetworkFailure('Ошибка подключения');
    } catch (e) {
      Logs().e('ChatRemote: checkConnection', exception: e);
      return false;
    }
  }

  @override
  Future<List<String>> getModels() async {
    Logs().d('ChatRemote: getModels');
    try {
      final response = await _authGuard.execute(
        () => _client.getModels(grpc.Empty()),
      );
      Logs().i('ChatRemote: getModels получено ${response.models.length}');
      return response.models;
    } on GrpcError catch (e) {
      if (e.code == StatusCode.unavailable) {
        throw NetworkFailure('Ошибка подключения gRPC');
      }
      throwGrpcError(e, 'Ошибка gRPC');
    } catch (e) {
      Logs().e('ChatRemote: getModels', exception: e);
      throw ApiFailure('Ошибка получения списка моделей');
    }
  }

  @override
  Stream<String> sendChatMessage(
    String sessionId,
    List<Message> messages, {
    String? model,
  }) async* {
    Logs().d('ChatRemote: sendMessage sessionId=$sessionId');
    try {
      final chatMessages = MessageMapper.listToProto(messages);

      final request = grpc.SendMessageRequest()
        ..sessionId = sessionId
        ..messages.addAll(chatMessages);
      if (model != null && model.isNotEmpty) {
        request.model = model;
      }

      final responseStream = _client.sendMessage(request);

      await for (final response in responseStream) {
        if (response.content.isNotEmpty) {
          yield response.content;
        }

        if (response.done) {
          break;
        }
      }
      Logs().i('ChatRemote: sendMessage завершён');
    } on GrpcError catch (e) {
      if (e.code == StatusCode.deadlineExceeded) {
        throw NetworkFailure('Таймаут запроса gRPC');
      }
      Logs().e('ChatRemote: sendMessage', exception: e);
      throwGrpcError(e, 'Ошибка gRPC');
    } catch (e) {
      Logs().e('ChatRemote: sendMessage', exception: e);
      throw ApiFailure('Ошибка отправки сообщения');
    }
  }

  @override
  Future<ChatSession> createSession(String title, {String? model}) async {
    Logs().d('ChatRemote: createSession title=$title');
    try {
      final request = grpc.CreateSessionRequest(
        title: title,
      );
      if (model != null && model.isNotEmpty) {
        request.model = model;
      }

      final response = await _authGuard.execute(
        () => _client.createSession(request),
      );
      Logs().i('ChatRemote: createSession успешен');
      return SessionMapper.fromProto(response);
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: createSession', exception: e);
      throwGrpcError(e, 'Ошибка gRPC при создании сессии');
    } catch (e) {
      Logs().e('ChatRemote: createSession', exception: e);
      throw ApiFailure('Ошибка создания сессии');
    }
  }

  @override
  Future<ChatSession> getSession(String sessionId) async {
    try {
      final request = grpc.GetSessionRequest(
        sessionId: sessionId
      );

      final response = await _authGuard.execute(
        () => _client.getSession(request),
      );

      return SessionMapper.fromProto(response);
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при получении сессии: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка получения сессии: $e');
    }
  }

  @override
  Future<List<ChatSession>> getSessions(
    int page,
    int pageSize,
  ) async {
    Logs().d('ChatRemote: getSessions page=$page pageSize=$pageSize');
    try {
      final request = grpc.GetSessionsRequest(
        page: page,
        pageSize: pageSize,
      );

      final response = await _authGuard.execute(
        () => _client.getSessions(request),
      );
      Logs().i('ChatRemote: getSessions получено ${response.sessions.length}');
      return SessionMapper.listFromProto(response.sessions);
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getSessions', exception: e);
      throwGrpcError(e, 'Ошибка gRPC при получении списка сессий');
    } catch (e) {
      Logs().e('ChatRemote: getSessions', exception: e);
      throw ApiFailure('Ошибка получения списка сессий');
    }
  }

  @override
  Future<List<Message>> getSessionMessages(
    String sessionId,
    int page,
    int pageSize,
  ) async {
    try {
      final request = grpc.GetSessionMessagesRequest(
        sessionId: sessionId,
        page: page,
        pageSize: pageSize,
      );

      final response = await _authGuard.execute(
        () => _client.getSessionMessages(request),
      );

      return MessageMapper.listFromProto(response.messages);
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при получении сообщений: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка получения сообщений: $e');
    }
  }

  @override
  Future<void> deleteSession(String sessionId) async {
    try {
      final request = grpc.DeleteSessionRequest(
        sessionId: sessionId
      );

      await _authGuard.execute(() => _client.deleteSession(request));
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при удалении сессии: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка удаления сессии: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionTitle(String sessionId, String title) async {
    try {
      final request = grpc.UpdateSessionTitleRequest(
        sessionId: sessionId,
        title: title
      );

      final response = await _authGuard.execute(
        () => _client.updateSessionTitle(request),
      );

      return SessionMapper.fromProto(response);
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при обновлении заголовка: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка обновления заголовка: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionModel(String sessionId, String model) async {
    try {
      final request = grpc.UpdateSessionModelRequest(
        sessionId: sessionId,
        model: model,
      );

      final response = await _authGuard.execute(
        () => _client.updateSessionModel(request),
      );

      return SessionMapper.fromProto(response);
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при обновлении модели сессии: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка обновления модели сессии: $e');
    }
  }
}
