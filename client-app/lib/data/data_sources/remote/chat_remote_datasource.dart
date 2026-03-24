import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/data/mappers/message_mapper.dart';
import 'package:gen/data/mappers/session_mapper.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/generated/grpc_pb/common.pb.dart' as common;
import 'package:gen/generated/grpc_pb/chat.pbgrpc.dart' as grpc;

Message? _lastUserMessage(List<Message> messages) {
  for (var i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role == MessageRole.user) {
      return messages[i];
    }
  }
  return null;
}

abstract class IChatRemoteDataSource {
  Future<bool> checkConnection();

  Stream<String> sendChatMessage(
    int sessionId,
    List<Message> messages,
  );

  Future<ChatSession> createSession(String title);

  Future<ChatSession> getSession(int sessionId);

  Future<List<ChatSession>> getSessions(int page, int pageSize);

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

class ChatRemoteDataSource implements IChatRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  ChatRemoteDataSource(this._channelManager, this._authGuard);

  grpc.ChatServiceClient get _client => _channelManager.chatClient;

  @override
  Future<bool> checkConnection() async {
    Logs().d('ChatRemote: checkConnection');
    try {
      final response = await _client.checkConnection(common.Empty());
      Logs().i(
        'ChatRemote: checkConnection isConnected=${response.isConnected}',
      );
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
  Stream<String> sendChatMessage(
    int sessionId,
    List<Message> messages,
  ) {
    Logs().d('ChatRemote: sendMessage sessionId=$sessionId');
    final controller = StreamController<String>();
    StreamSubscription<grpc.ChatResponse>? streamSubscription;

    Future<void> closeWithError(Object error, [StackTrace? st]) async {
      if (!controller.isClosed) {
        controller.addError(error, st);
      }
      if (!controller.isClosed) {
        await controller.close();
      }
    }

    () async {
      try {
        final lastUser = _lastUserMessage(messages);
        if (lastUser == null) {
          Logs().w('ChatRemote: sendMessage нет сообщения с role=user');
          throw ApiFailure('Нет пользовательского сообщения для отправки');
        }

        final chatMessages = MessageMapper.listToProto([lastUser]);

        final request = grpc.SendMessageRequest()
          ..sessionId = Int64(sessionId)
          ..messages.addAll(chatMessages);
        final responseStream = _client.sendMessage(request);
        streamSubscription = responseStream.listen(
          (response) {
            if (controller.isClosed) {
              return;
            }
            if (response.content.isNotEmpty) {
              controller.add(response.content);
            }
            if (response.done) {
              Logs().i('ChatRemote: sendMessage завершён');
              controller.close();
            }
          },
          onError: (Object e, StackTrace st) async {
            if (e is GrpcError && e.code == StatusCode.deadlineExceeded) {
              await closeWithError(NetworkFailure('Таймаут запроса gRPC'), st);
              return;
            }
            if (e is GrpcError) {
              Logs().e('ChatRemote: sendMessage', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else {
                await closeWithError(NetworkFailure('Ошибка gRPC'), st);
              }
              return;
            }
            await closeWithError(ApiFailure('Ошибка отправки сообщения'), st);
          },
          onDone: () async {
            if (!controller.isClosed) {
              Logs().i('ChatRemote: sendMessage завершён');
              await controller.close();
            }
          },
          cancelOnError: true,
        );
      } on GrpcError catch (e) {
        if (e.code == StatusCode.deadlineExceeded) {
          await closeWithError(NetworkFailure('Таймаут запроса gRPC'));
          return;
        }
        Logs().e('ChatRemote: sendMessage', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else {
          await closeWithError(NetworkFailure('Ошибка gRPC'));
        }
      } on Failure catch (e, st) {
        await closeWithError(e, st);
      } catch (e, st) {
        Logs().e('ChatRemote: sendMessage', exception: e);
        await closeWithError(ApiFailure('Ошибка отправки сообщения'), st);
      }
    }();

    controller.onCancel = () async {
      Logs().d('ChatRemote: sendMessage отменён клиентом');
      await streamSubscription?.cancel();
      streamSubscription = null;
    };

    return controller.stream;
  }

  @override
  Future<ChatSession> createSession(String title) async {
    Logs().d('ChatRemote: createSession title=$title');
    try {
      final request = grpc.CreateSessionRequest(title: title);

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
  Future<ChatSession> getSession(int sessionId) async {
    try {
      final request = grpc.GetSessionRequest(sessionId: Int64(sessionId));

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
  Future<List<ChatSession>> getSessions(int page, int pageSize) async {
    Logs().d('ChatRemote: getSessions page=$page pageSize=$pageSize');
    try {
      final request = grpc.GetSessionsRequest(page: page, pageSize: pageSize);

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
    int sessionId,
    int page,
    int pageSize,
  ) async {
    try {
      final request = grpc.GetSessionMessagesRequest(
        sessionId: Int64(sessionId),
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
  Future<void> deleteSession(int sessionId) async {
    try {
      final request = grpc.DeleteSessionRequest(sessionId: Int64(sessionId));

      await _authGuard.execute(() => _client.deleteSession(request));
    } on GrpcError catch (e) {
      throwGrpcError(e, 'Ошибка gRPC при удалении сессии: ${e.message}');
    } catch (e) {
      throw ApiFailure('Ошибка удаления сессии: $e');
    }
  }

  @override
  Future<ChatSession> updateSessionTitle(int sessionId, String title) async {
    try {
      final request = grpc.UpdateSessionTitleRequest(
        sessionId: Int64(sessionId),
        title: title,
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
  Future<String?> getSelectedRunner() async {
    final response = await _authGuard.execute(
      () => _client.getSelectedRunner(common.Empty()),
    );
    return response.runner.isEmpty ? null : response.runner;
  }

  @override
  Future<void> setSelectedRunner(String? runner) async {
    await _authGuard.execute(
      () => _client.setSelectedRunner(
        grpc.SetSelectedRunnerRequest(runner: runner ?? ''),
      ),
    );
  }

  @override
  Future<String?> getDefaultRunnerModel(String runner) async {
    final response = await _authGuard.execute(
      () => _client.getDefaultRunnerModel(
        grpc.GetDefaultRunnerModelRequest(runner: runner),
      ),
    );
    return response.model.isEmpty ? null : response.model;
  }

  @override
  Future<void> setDefaultRunnerModel(String runner, String? model) async {
    await _authGuard.execute(
      () => _client.setDefaultRunnerModel(
        grpc.SetDefaultRunnerModelRequest(runner: runner, model: model ?? ''),
      ),
    );
  }

  @override
  Future<ChatSessionSettings> getSessionSettings(int sessionId) async {
    final response = await _authGuard.execute(
      () => _client.getSessionSettings(
        grpc.GetSessionSettingsRequest(sessionId: Int64(sessionId)),
      ),
    );
    return ChatSessionSettings(
      sessionId: response.sessionId.toInt(),
      systemPrompt: response.systemPrompt,
      stopSequences: List<String>.from(response.stopSequences),
      timeoutSeconds: response.timeoutSeconds,
      temperature: response.hasTemperature() ? response.temperature : null,
      maxTokens: response.hasMaxTokens() ? response.maxTokens : null,
      topK: response.hasTopK() ? response.topK : null,
      topP: response.hasTopP() ? response.topP : null,
      jsonMode: response.jsonMode,
      jsonSchema: response.jsonSchema,
      toolsJson: response.toolsJson,
      profile: response.profile,
    );
  }

  @override
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
  }) async {
    final request = grpc.UpdateSessionSettingsRequest(
      sessionId: Int64(sessionId),
      systemPrompt: systemPrompt,
      stopSequences: stopSequences,
      timeoutSeconds: timeoutSeconds,
      jsonMode: jsonMode,
      jsonSchema: jsonSchema,
      toolsJson: toolsJson,
      profile: profile,
    );
    if (temperature != null) {
      request.temperature = temperature;
    }
    if (maxTokens != null) {
      request.maxTokens = maxTokens;
    }
    if (topK != null) {
      request.topK = topK;
    }
    if (topP != null) {
      request.topP = topP;
    }
    final response = await _authGuard.execute(
      () => _client.updateSessionSettings(request),
    );
    return ChatSessionSettings(
      sessionId: response.sessionId.toInt(),
      systemPrompt: response.systemPrompt,
      stopSequences: List<String>.from(response.stopSequences),
      timeoutSeconds: response.timeoutSeconds,
      temperature: response.hasTemperature() ? response.temperature : null,
      maxTokens: response.hasMaxTokens() ? response.maxTokens : null,
      topK: response.hasTopK() ? response.topK : null,
      topP: response.hasTopP() ? response.topP : null,
      jsonMode: response.jsonMode,
      jsonSchema: response.jsonSchema,
      toolsJson: response.toolsJson,
      profile: response.profile,
    );
  }
}
