import 'dart:async';
import 'dart:typed_data';

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
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session_file_download.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/generated/grpc_pb/chat.pb.dart' as chat_pb;
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

  Stream<ChatStreamChunk> sendChatMessage(
    int sessionId,
    List<Message> messages,
  );

  Stream<ChatStreamChunk> regenerateAssistantResponse(
    int sessionId,
    int assistantMessageId,
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
  Stream<ChatStreamChunk> sendChatMessage(
    int sessionId,
    List<Message> messages,
  ) {
    Logs().d('ChatRemote: sendMessage sessionId=$sessionId');
    final controller = StreamController<ChatStreamChunk>();
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
            if (response.done) {
              Logs().i('ChatRemote: sendMessage завершён');
              controller.close();
              return;
            }
            final mid = response.id.toInt();
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_TOOL_STATUS) {
              controller.add(
                ChatStreamChunk(
                  kind: ChatStreamChunkKind.toolStatus,
                  text: response.content,
                  toolName:
                      response.hasToolName() ? response.toolName : null,
                  messageId: mid,
                ),
              );
              return;
            }
            if (response.content.isNotEmpty) {
              controller.add(
                ChatStreamChunk(
                  kind: ChatStreamChunkKind.text,
                  text: response.content,
                  messageId: mid,
                ),
              );
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
              } else if (e.code == StatusCode.invalidArgument) {
                final detail = e.message?.trim();
                await closeWithError(
                  ApiFailure(
                    detail != null && detail.isNotEmpty
                        ? detail
                        : 'Некорректные данные запроса',
                  ),
                  st,
                );
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
        } else if (e.code == StatusCode.invalidArgument) {
          final detail = e.message?.trim();
          await closeWithError(
            ApiFailure(
              detail != null && detail.isNotEmpty
                  ? detail
                  : 'Некорректные данные запроса',
            ),
          );
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
  Stream<ChatStreamChunk> regenerateAssistantResponse(
    int sessionId,
    int assistantMessageId,
  ) {
    Logs().d(
      'ChatRemote: regenerateAssistantResponse sessionId=$sessionId msgId=$assistantMessageId',
    );
    final controller = StreamController<ChatStreamChunk>();
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
        if (assistantMessageId <= 0) {
          throw ApiFailure('Некорректный идентификатор сообщения');
        }
        final request = grpc.RegenerateAssistantRequest()
          ..sessionId = Int64(sessionId)
          ..assistantMessageId = Int64(assistantMessageId);
        final responseStream = _client.regenerateAssistantResponse(request);
        streamSubscription = responseStream.listen(
          (response) {
            if (controller.isClosed) {
              return;
            }
            if (response.done) {
              Logs().i('ChatRemote: regenerateAssistantResponse завершён');
              controller.close();
              return;
            }
            final mid = response.id.toInt();
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_TOOL_STATUS) {
              controller.add(
                ChatStreamChunk(
                  kind: ChatStreamChunkKind.toolStatus,
                  text: response.content,
                  toolName:
                      response.hasToolName() ? response.toolName : null,
                  messageId: mid,
                ),
              );
              return;
            }
            if (response.content.isNotEmpty) {
              controller.add(
                ChatStreamChunk(
                  kind: ChatStreamChunkKind.text,
                  text: response.content,
                  messageId: mid,
                ),
              );
            }
          },
          onError: (Object e, StackTrace st) async {
            if (e is GrpcError && e.code == StatusCode.deadlineExceeded) {
              await closeWithError(NetworkFailure('Таймаут запроса gRPC'), st);
              return;
            }
            if (e is GrpcError) {
              Logs().e('ChatRemote: regenerateAssistantResponse', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else if (e.code == StatusCode.invalidArgument) {
                final detail = e.message?.trim();
                await closeWithError(
                  ApiFailure(
                    detail != null && detail.isNotEmpty
                        ? detail
                        : 'Некорректные данные запроса',
                  ),
                  st,
                );
              } else if (e.code == StatusCode.failedPrecondition) {
                final detail = e.message?.trim();
                await closeWithError(
                  ApiFailure(
                    detail != null && detail.isNotEmpty
                        ? detail
                        : 'Операция недоступна в текущем состоянии',
                  ),
                  st,
                );
              } else {
                await closeWithError(NetworkFailure('Ошибка gRPC'), st);
              }
              return;
            }
            await closeWithError(ApiFailure('Ошибка перегенерации ответа'), st);
          },
          onDone: () async {
            if (!controller.isClosed) {
              Logs().i('ChatRemote: regenerateAssistantResponse завершён');
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
        Logs().e('ChatRemote: regenerateAssistantResponse', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else if (e.code == StatusCode.invalidArgument) {
          final detail = e.message?.trim();
          await closeWithError(
            ApiFailure(
              detail != null && detail.isNotEmpty
                  ? detail
                  : 'Некорректные данные запроса',
            ),
          );
        } else if (e.code == StatusCode.failedPrecondition) {
          final detail = e.message?.trim();
          await closeWithError(
            ApiFailure(
              detail != null && detail.isNotEmpty
                  ? detail
                  : 'Операция недоступна в текущем состоянии',
            ),
          );
        } else {
          await closeWithError(NetworkFailure('Ошибка gRPC'));
        }
      } on Failure catch (e, st) {
        await closeWithError(e, st);
      } catch (e, st) {
        Logs().e('ChatRemote: regenerateAssistantResponse', exception: e);
        await closeWithError(ApiFailure('Ошибка перегенерации ответа'), st);
      }
    }();

    controller.onCancel = () async {
      Logs().d('ChatRemote: regenerateAssistantResponse отменён клиентом');
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
      topK: response.hasTopK() ? response.topK : null,
      topP: response.hasTopP() ? response.topP : null,
      jsonMode: response.jsonMode,
      jsonSchema: response.jsonSchema,
      toolsJson: response.toolsJson,
      profile: response.profile,
    );
  }

  @override
  Future<int> putSessionFile({
    required int sessionId,
    required String filename,
    required List<int> content,
    int ttlSeconds = 0,
  }) async {
    Logs().d('ChatRemote: putSessionFile sessionId=$sessionId');
    final req = chat_pb.PutSessionFileRequest(
      sessionId: Int64(sessionId),
      filename: filename,
      content: content,
      ttlSeconds: ttlSeconds,
    );
    try {
      final resp = await _authGuard.execute(() => _client.putSessionFile(req));
      return resp.fileId.toInt();
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: putSessionFile', exception: e);
      if (e.code == StatusCode.invalidArgument) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Некорректные данные файла',
        );
      }
      if (e.code == StatusCode.permissionDenied) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Нет доступа к сессии',
        );
      }
      throwGrpcError(e, 'Ошибка загрузки файла сессии');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: putSessionFile', exception: e);
      throw ApiFailure('Ошибка загрузки файла сессии');
    }
  }

  @override
  Future<SessionFileDownload> getSessionFile({
    required int sessionId,
    required int fileId,
  }) async {
    Logs().d('ChatRemote: getSessionFile sessionId=$sessionId fileId=$fileId');
    final req = chat_pb.GetSessionFileRequest(
      sessionId: Int64(sessionId),
      fileId: Int64(fileId),
    );
    try {
      final resp = await _authGuard.execute(() => _client.getSessionFile(req));
      return SessionFileDownload(
        filename: resp.filename,
        content: Uint8List.fromList(resp.content),
      );
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getSessionFile', exception: e);
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }
      if (e.code == StatusCode.notFound) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Файл не найден или удалён',
        );
      }
      if (e.code == StatusCode.permissionDenied) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Нет доступа к файлу',
        );
      }
      if (e.code == StatusCode.invalidArgument) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Некорректный запрос файла',
        );
      }
      throwGrpcError(e, 'Ошибка получения файла сессии');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: getSessionFile', exception: e);
      throw ApiFailure('Ошибка получения файла сессии');
    }
  }

  @override
  Future<SpreadsheetApplyResult> applySpreadsheet({
    List<int>? workbookXlsx,
    required String operationsJson,
    String previewSheet = '',
    String previewRange = '',
  }) async {
    Logs().d('ChatRemote: applySpreadsheet');
    final req = chat_pb.SpreadsheetApplyRequest(
      operationsJson: operationsJson,
      previewSheet: previewSheet,
      previewRange: previewRange,
    );
    if (workbookXlsx != null && workbookXlsx.isNotEmpty) {
      req.workbookXlsx = workbookXlsx;
    }
    try {
      final resp = await _authGuard.execute(() => _client.applySpreadsheet(req));
      return SpreadsheetApplyResult(
        workbookBytes: Uint8List.fromList(resp.workbookXlsx),
        previewTsv: resp.previewTsv,
        exportedCsv: resp.hasExportedCsv() && resp.exportedCsv.isNotEmpty
            ? resp.exportedCsv
            : null,
      );
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: applySpreadsheet', exception: e);
      if (e.code == StatusCode.invalidArgument) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Некорректные данные таблицы',
        );
      }
      throwGrpcError(e, 'Ошибка таблицы');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: applySpreadsheet', exception: e);
      throw ApiFailure('Ошибка таблицы');
    }
  }

  @override
  Future<Uint8List> buildDocx({required String specJson}) async {
    Logs().d('ChatRemote: buildDocx');
    final req = chat_pb.DocxBuildRequest(specJson: specJson);
    try {
      final resp = await _authGuard.execute(() => _client.buildDocx(req));
      return Uint8List.fromList(resp.docx);
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: buildDocx', exception: e);
      if (e.code == StatusCode.invalidArgument) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Некорректная спецификация документа',
        );
      }
      throwGrpcError(e, 'Ошибка документа Word');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: buildDocx', exception: e);
      throw ApiFailure('Ошибка документа Word');
    }
  }

  @override
  Future<String> applyMarkdownPatch({
    required String baseText,
    required String patchJson,
  }) async {
    Logs().d('ChatRemote: applyMarkdownPatch');
    final req = chat_pb.MarkdownPatchRequest(
      baseText: baseText,
      patchJson: patchJson,
    );
    try {
      final resp = await _authGuard.execute(() => _client.applyMarkdownPatch(req));
      return resp.text;
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: applyMarkdownPatch', exception: e);
      if (e.code == StatusCode.invalidArgument) {
        final detail = e.message?.trim();
        throw ApiFailure(
          detail != null && detail.isNotEmpty
              ? detail
              : 'Некорректный патч текста',
        );
      }
      throwGrpcError(e, 'Ошибка патча текста');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: applyMarkdownPatch', exception: e);
      throw ApiFailure('Ошибка патча текста');
    }
  }
}
