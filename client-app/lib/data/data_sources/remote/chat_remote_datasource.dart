import 'dart:async';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';
import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/data/mappers/message_mapper.dart';
import 'package:gen/data/mappers/session_mapper.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/domain/entities/session_file_download.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/session_messages_page.dart';
import 'package:gen/domain/entities/spreadsheet_apply_result.dart';
import 'package:gen/domain/entities/file_ingestion_status.dart';
import 'package:gen/generated/grpc_pb/chat.pb.dart' as chat_pb;
import 'package:gen/generated/grpc_pb/common.pb.dart' as common;
import 'package:gen/generated/grpc_pb/chat.pbgrpc.dart' as grpc;

void _logGrpcServerMessage(GrpcError e, String context) {
  final m = e.message?.trim();
  if (m != null && m.isNotEmpty) {
    Logs().w('ChatRemote: $context [code=${e.code}] $m');
  }
}

abstract class IChatRemoteDataSource {
  Future<bool> checkConnection();

  Stream<ChatStreamChunk> sendChatMessage(
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

  Future<List<ChatSession>> getSessions(int page, int pageSize);

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
    required bool modelReasoningEnabled,
    required bool webSearchEnabled,
    required String webSearchProvider,
    required bool mcpEnabled,
    required List<int> mcpServerIds,
  });
  Future<String?> getSelectedRunner();
  Future<void> setSelectedRunner(String? runner);

  Future<int> putSessionFile({
    required int sessionId,
    required String filename,
    required List<int> content,
    int ttlSeconds = 0,
  });

  Future<FileIngestionStatus> getFileIngestionStatus({
    required int sessionId,
    required int fileId,
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
    Message message,
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
        if (message.role != MessageRole.user) {
          throw ApiFailure('Последнее сообщение должно быть role=user');
        }

        final fid = message.attachmentFileId;
        if (message.attachmentContent != null &&
            message.attachmentContent!.isNotEmpty) {
          throw ApiFailure(
            'Вложение должно быть загружено через PutSessionFile; '
            'в SendMessage передаётся только attachment_file_id.',
          );
        }
        final request = grpc.SendMessageRequest()
          ..sessionId = Int64(sessionId)
          ..text = message.content;
        if (fid != null && fid > 0) {
          request.attachmentFileId = Int64(fid);
        }
        if (message.useFileRag) {
          request.useFileRag = true;
          if (message.fileRagTopK > 0) {
            request.fileRagTopK = message.fileRagTopK;
          }
          if (message.fileRagEmbedModel.isNotEmpty) {
            request.fileRagEmbedModel = message.fileRagEmbedModel;
          }
        }
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
            if (response.chunkKind == chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_NOTICE) {
              final t = response.content.trim();
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.notice,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
              return;
            }
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
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_REASONING) {
              final t = response.content;
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.reasoning,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
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
              await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'), st);
              return;
            }
            if (e is GrpcError) {
              Logs().e('ChatRemote: sendMessage', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else if (e.code == StatusCode.invalidArgument) {
                _logGrpcServerMessage(e, 'sendMessage invalidArgument');
                await closeWithError(
                  ApiFailure('Некорректный запрос (код ${e.code})'),
                  st,
                );
              } else if (e.code == StatusCode.failedPrecondition) {
                final m = e.message?.trim();
                await closeWithError(
                  ApiFailure(m != null && m.isNotEmpty ? m : 'Условие не выполнено (код ${e.code})'),
                  st,
                );
              } else {
                await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'), st);
              }
              return;
            }
            await closeWithError(
              ApiFailure('Ошибка отправки сообщения: $e'),
              st,
            );
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
          await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'));
          return;
        }
        Logs().e('ChatRemote: sendMessage', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else if (e.code == StatusCode.invalidArgument) {
          _logGrpcServerMessage(e, 'sendMessage invalidArgument');
          await closeWithError(
            ApiFailure('Некорректный запрос (код ${e.code})'),
          );
        } else if (e.code == StatusCode.failedPrecondition) {
          final m = e.message?.trim();
          await closeWithError(
            ApiFailure(m != null && m.isNotEmpty ? m : 'Условие не выполнено (код ${e.code})'),
          );
        } else {
          await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'));
        }
      } on Failure catch (e, st) {
        await closeWithError(e, st);
      } catch (e, st) {
        Logs().e('ChatRemote: sendMessage', exception: e);
        await closeWithError(ApiFailure('Ошибка отправки сообщения: $e'), st);
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
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_NOTICE) {
              final t = response.content.trim();
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.notice,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
              return;
            }
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
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_REASONING) {
              final t = response.content;
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.reasoning,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
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
              await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'), st);
              return;
            }
            if (e is GrpcError) {
              Logs().e('ChatRemote: regenerateAssistantResponse', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else if (e.code == StatusCode.invalidArgument) {
                _logGrpcServerMessage(e, 'regenerate invalidArgument');
                await closeWithError(
                  ApiFailure('Некорректный запрос (код ${e.code})'),
                  st,
                );
              } else if (e.code == StatusCode.failedPrecondition) {
                _logGrpcServerMessage(e, 'regenerate failedPrecondition');
                await closeWithError(
                  ApiFailure('Операция недоступна (код ${e.code})'),
                  st,
                );
              } else {
                await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'), st);
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
          await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'));
          return;
        }
        Logs().e('ChatRemote: regenerateAssistantResponse', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else if (e.code == StatusCode.invalidArgument) {
          _logGrpcServerMessage(e, 'regenerate invalidArgument');
          await closeWithError(
            ApiFailure('Некорректный запрос (код ${e.code})'),
          );
        } else if (e.code == StatusCode.failedPrecondition) {
          _logGrpcServerMessage(e, 'regenerate failedPrecondition');
          await closeWithError(
            ApiFailure('Операция недоступна (код ${e.code})'),
          );
        } else {
          await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'));
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
  Stream<ChatStreamChunk> continueAssistantResponse(
    int sessionId,
    int assistantMessageId,
  ) {
    Logs().d(
      'ChatRemote: continueAssistantResponse sessionId=$sessionId msgId=$assistantMessageId',
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
        final request = grpc.ContinueAssistantRequest()
          ..sessionId = Int64(sessionId)
          ..assistantMessageId = Int64(assistantMessageId);
        final responseStream = _client.continueAssistantResponse(request);
        streamSubscription = responseStream.listen(
          (response) {
            if (controller.isClosed) {
              return;
            }
            if (response.done) {
              Logs().i('ChatRemote: continueAssistantResponse завершён');
              controller.close();
              return;
            }
            final mid = response.id.toInt();
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_NOTICE) {
              final t = response.content.trim();
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.notice,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
              return;
            }
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
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_REASONING) {
              final t = response.content;
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.reasoning,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
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
              await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'), st);
              return;
            }
            if (e is GrpcError) {
              Logs().e('ChatRemote: continueAssistantResponse', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else if (e.code == StatusCode.invalidArgument) {
                _logGrpcServerMessage(e, 'continue invalidArgument');
                await closeWithError(
                  ApiFailure('Некорректный запрос (код ${e.code})'),
                  st,
                );
              } else if (e.code == StatusCode.failedPrecondition) {
                _logGrpcServerMessage(e, 'continue failedPrecondition');
                await closeWithError(
                  ApiFailure('Операция недоступна (код ${e.code})'),
                  st,
                );
              } else {
                await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'), st);
              }
              return;
            }
            await closeWithError(ApiFailure('Ошибка продолжения ответа'), st);
          },
          onDone: () async {
            if (!controller.isClosed) {
              Logs().i('ChatRemote: continueAssistantResponse завершён');
              await controller.close();
            }
          },
          cancelOnError: true,
        );
      } on GrpcError catch (e) {
        if (e.code == StatusCode.deadlineExceeded) {
          await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'));
          return;
        }
        Logs().e('ChatRemote: continueAssistantResponse', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else if (e.code == StatusCode.invalidArgument) {
          _logGrpcServerMessage(e, 'continue invalidArgument');
          await closeWithError(
            ApiFailure('Некорректный запрос (код ${e.code})'),
          );
        } else if (e.code == StatusCode.failedPrecondition) {
          _logGrpcServerMessage(e, 'continue failedPrecondition');
          await closeWithError(
            ApiFailure('Операция недоступна (код ${e.code})'),
          );
        } else {
          await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'));
        }
      } on Failure catch (e, st) {
        await closeWithError(e, st);
      } catch (e, st) {
        Logs().e('ChatRemote: continueAssistantResponse', exception: e);
        await closeWithError(ApiFailure('Ошибка продолжения ответа'), st);
      }
    }();

    controller.onCancel = () async {
      Logs().d('ChatRemote: continueAssistantResponse отменён клиентом');
      await streamSubscription?.cancel();
      streamSubscription = null;
    };

    return controller.stream;
  }

  @override
  Stream<ChatStreamChunk> editUserMessageAndContinue(
    int sessionId,
    int userMessageId,
    String newContent,
  ) {
    Logs().d('ChatRemote: editUserMessageAndContinue sessionId=$sessionId msgId=$userMessageId');
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
        if (userMessageId <= 0) {
          throw ApiFailure('Некорректный идентификатор сообщения');
        }

        final content = newContent.trim();
        if (content.isEmpty) {
          throw ApiFailure('Текст не может быть пустым');
        }

        final request = grpc.EditUserMessageAndContinueRequest()
          ..sessionId = Int64(sessionId)
          ..userMessageId = Int64(userMessageId)
          ..newContent = content;
        final responseStream = _client.editUserMessageAndContinue(request);
        streamSubscription = responseStream.listen(
          (response) {
            if (controller.isClosed) {
              return;
            }

            if (response.done) {
              Logs().i('ChatRemote: editUserMessageAndContinue завершён');
              controller.close();
              return;
            }

            final mid = response.id.toInt();
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_NOTICE) {
              final t = response.content.trim();
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.notice,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
              return;
            }
            if (response.chunkKind == chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_TOOL_STATUS) {
              controller.add(
                ChatStreamChunk(
                  kind: ChatStreamChunkKind.toolStatus,
                  text: response.content,
                  toolName: response.hasToolName() ? response.toolName : null,
                  messageId: mid,
                ),
              );
              return;
            }
            if (response.chunkKind ==
                chat_pb.StreamChunkKind.STREAM_CHUNK_KIND_REASONING) {
              final t = response.content;
              if (t.isNotEmpty) {
                controller.add(
                  ChatStreamChunk(
                    kind: ChatStreamChunkKind.reasoning,
                    text: t,
                    messageId: mid,
                  ),
                );
              }
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
              await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'), st);
              return;
            }

            if (e is GrpcError) {
              Logs().e('ChatRemote: editUserMessageAndContinue', exception: e);
              if (e.code == StatusCode.unauthenticated) {
                await closeWithError(UnauthorizedFailure(kSessionExpiredMessage), st);
              } else if (e.code == StatusCode.invalidArgument) {
                _logGrpcServerMessage(e, 'editMessage invalidArgument');
                await closeWithError(
                  ApiFailure('Некорректный запрос (код ${e.code})'),
                  st,
                );
              } else if (e.code == StatusCode.permissionDenied) {
                _logGrpcServerMessage(e, 'editMessage permissionDenied');
                await closeWithError(
                  ApiFailure('Нет доступа (код ${e.code})'),
                  st,
                );
              } else {
                await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'), st);
              }
              return;
            }
            await closeWithError(ApiFailure('Ошибка редактирования сообщения'), st);
          },
          onDone: () async {
            if (!controller.isClosed) {
              Logs().i('ChatRemote: editUserMessageAndContinue завершён');
              await controller.close();
            }
          },
          cancelOnError: true,
        );
      } on GrpcError catch (e) {
        if (e.code == StatusCode.deadlineExceeded) {
          await closeWithError(NetworkFailure('Таймаут запроса (код ${e.code})'));
          return;
        }

        Logs().e('ChatRemote: editUserMessageAndContinue', exception: e);
        if (e.code == StatusCode.unauthenticated) {
          await closeWithError(UnauthorizedFailure(kSessionExpiredMessage));
        } else if (e.code == StatusCode.invalidArgument) {
          _logGrpcServerMessage(e, 'editMessage invalidArgument');
          await closeWithError(
            ApiFailure('Некорректный запрос (код ${e.code})'),
          );
        } else if (e.code == StatusCode.permissionDenied) {
          _logGrpcServerMessage(e, 'editMessage permissionDenied');
          await closeWithError(
            ApiFailure('Нет доступа (код ${e.code})'),
          );
        } else {
          await closeWithError(NetworkFailure('Ошибка сервера (код ${e.code})'));
        }
      } on Failure catch (e, st) {
        await closeWithError(e, st);
      } catch (e, st) {
        Logs().e('ChatRemote: editUserMessageAndContinue', exception: e);
        await closeWithError(ApiFailure('Ошибка редактирования сообщения'), st);
      }
    }();

    controller.onCancel = () async {
      Logs().d('ChatRemote: editUserMessageAndContinue отменён клиентом');
      await streamSubscription?.cancel();
      streamSubscription = null;
    };

    return controller.stream;
  }

  DateTime _dtFromUnixSeconds(int seconds) {
    return DateTime.fromMillisecondsSinceEpoch(seconds * 1000);
  }

  @override
  Future<List<UserMessageEdit>> getUserMessageEdits({
    required int sessionId,
    required int userMessageId,
  }) async {
    try {
      final req = grpc.GetUserMessageEditsRequest()
        ..sessionId = Int64(sessionId)
        ..userMessageId = Int64(userMessageId);
      final resp = await _authGuard.execute(() => _client.getUserMessageEdits(req));
      return resp.edits.map((e) => UserMessageEdit(
        id: e.id.toInt(),
        messageId: e.messageId.toInt(),
        createdAt: _dtFromUnixSeconds(e.createdAt.toInt()),
        oldContent: e.oldContent,
        newContent: e.newContent,
      ))
      .toList();
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getUserMessageEdits', exception: e);
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }

      if (e.code == StatusCode.invalidArgument) {
        _logGrpcServerMessage(e, 'getUserMessageEdits');
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }
      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'getUserMessageEdits denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }
      throwGrpcError(e, 'история правок');
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка получения истории правок');
    }
  }

  @override
  Future<List<Message>> getSessionMessagesForUserMessageVersion({
    required int sessionId,
    required int userMessageId,
    required int versionIndex,
  }) async {
    try {
      final req = grpc.GetSessionMessagesForUserMessageVersionRequest()
        ..sessionId = Int64(sessionId)
        ..userMessageId = Int64(userMessageId)
        ..versionIndex = versionIndex;
      final resp = await _authGuard.execute(() => _client.getSessionMessagesForUserMessageVersion(req));

      return MessageMapper.listFromProto(resp.messages);
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getSessionMessagesForUserMessageVersion', exception: e);
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }

      if (e.code == StatusCode.invalidArgument) {
        _logGrpcServerMessage(e, 'getSessionMessagesForUserMessageVersion');
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }

      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'getSessionMessagesForUserMessageVersion denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }

      throwGrpcError(e, 'версия истории сообщений');
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка получения версии истории');
    }
  }

  @override
  Future<List<AssistantMessageRegeneration>> getAssistantMessageRegenerations({
    required int sessionId,
    required int assistantMessageId,
  }) async {
    try {
      final req = grpc.GetAssistantMessageRegenerationsRequest()
        ..sessionId = Int64(sessionId)
        ..assistantMessageId = Int64(assistantMessageId);
      final resp = await _authGuard.execute(() => _client.getAssistantMessageRegenerations(req));

      return resp.regenerations.map((r) => AssistantMessageRegeneration(
        id: r.id.toInt(),
        messageId: r.messageId.toInt(),
        createdAt: _dtFromUnixSeconds(r.createdAt.toInt()),
        oldContent: r.oldContent,
        newContent: r.newContent,
      ))
      .toList();
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getAssistantMessageRegenerations', exception: e);
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }

      if (e.code == StatusCode.invalidArgument) {
        _logGrpcServerMessage(e, 'getAssistantMessageRegenerations');
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }

      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'getAssistantMessageRegenerations denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }

      throwGrpcError(e, 'история перегенераций');
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка получения истории перегенераций');
    }
  }

  @override
  Future<List<Message>> getSessionMessagesForAssistantMessageVersion({
    required int sessionId,
    required int assistantMessageId,
    required int versionIndex,
  }) async {
    try {
      final req = grpc.GetSessionMessagesForAssistantMessageVersionRequest()
        ..sessionId = Int64(sessionId)
        ..assistantMessageId = Int64(assistantMessageId)
        ..versionIndex = versionIndex;
      final resp = await _authGuard.execute(() => _client.getSessionMessagesForAssistantMessageVersion(req));

      return MessageMapper.listFromProto(resp.messages);
    } on GrpcError catch (e) {
      Logs().e('ChatRemote: getSessionMessagesForAssistantMessageVersion', exception: e);
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }

      if (e.code == StatusCode.invalidArgument) {
        _logGrpcServerMessage(e, 'getSessionMessagesForAssistantMessageVersion');
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }

      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'getSessionMessagesForAssistantMessageVersion denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }

      throwGrpcError(e, 'версия ответа ассистента');
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка получения версии ответа');
    }
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
      throwGrpcError(e, 'создание сессии');
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
      throwGrpcError(e, 'получение сессии');
    } catch (e, st) {
      Logs().e('ChatRemote: getSession', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения сессии'),
      );
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
      throwGrpcError(e, 'список сессий');
    } catch (e) {
      Logs().e('ChatRemote: getSessions', exception: e);
      throw ApiFailure('Ошибка получения списка сессий');
    }
  }

  @override
  Future<SessionMessagesPage> getSessionMessagesPage({
    required int sessionId,
    int beforeMessageId = 0,
    int pageSize = 40,
  }) async {
    try {
      final request = grpc.GetSessionMessagesRequest(
        sessionId: Int64(sessionId),
        page: 1,
        pageSize: pageSize,
        beforeMessageId: Int64(beforeMessageId),
      );

      final response = await _authGuard.execute(
        () => _client.getSessionMessages(request),
      );

      return SessionMessagesPage(
        messages: MessageMapper.listFromProto(response.messages),
        hasMoreOlder: response.hasMoreOlder,
      );
    } on GrpcError catch (e) {
      throwGrpcError(e, 'сообщения сессии');
    } catch (e, st) {
      Logs().e('ChatRemote: getSessionMessagesPage', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка получения сообщений'),
      );
    }
  }

  @override
  Future<void> deleteSession(int sessionId) async {
    try {
      final request = grpc.DeleteSessionRequest(sessionId: Int64(sessionId));

      await _authGuard.execute(() => _client.deleteSession(request));
    } on GrpcError catch (e) {
      throwGrpcError(e, 'удаление сессии');
    } catch (e, st) {
      Logs().e('ChatRemote: deleteSession', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка удаления сессии'),
      );
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
      throwGrpcError(e, 'обновление заголовка сессии');
    } catch (e, st) {
      Logs().e('ChatRemote: updateSessionTitle', exception: e, stackTrace: st);
      throw ApiFailure(
        userSafeErrorMessage(e, fallback: 'Ошибка обновления заголовка'),
      );
    }
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
      modelReasoningEnabled: response.hasModelReasoningEnabled()
          ? response.modelReasoningEnabled
          : false,
      webSearchEnabled: response.webSearchEnabled,
      webSearchProvider: response.webSearchProvider,
      mcpEnabled: response.mcpEnabled,
      mcpServerIds: response.mcpServerIds.map((e) => e.toInt()).toList(),
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
    required bool modelReasoningEnabled,
    required bool webSearchEnabled,
    required String webSearchProvider,
    required bool mcpEnabled,
    required List<int> mcpServerIds,
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
      modelReasoningEnabled: modelReasoningEnabled,
      webSearchEnabled: webSearchEnabled,
      webSearchProvider: webSearchProvider,
      mcpEnabled: mcpEnabled,
    );
    request.mcpServerIds.addAll(mcpServerIds.map(Int64.new));
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
      modelReasoningEnabled: response.hasModelReasoningEnabled()
          ? response.modelReasoningEnabled
          : false,
      webSearchEnabled: response.webSearchEnabled,
      webSearchProvider: response.webSearchProvider,
      mcpEnabled: response.mcpEnabled,
      mcpServerIds: response.mcpServerIds.map((e) => e.toInt()).toList(),
    );
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
        _logGrpcServerMessage(e, 'putSessionFile');
        throw ApiFailure('Некорректные данные (код ${e.code})');
      }
      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'putSessionFile denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }
      throwGrpcError(e, 'загрузка файла сессии');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: putSessionFile', exception: e);
      throw ApiFailure('Ошибка загрузки файла сессии');
    }
  }

  @override
  Future<FileIngestionStatus> getFileIngestionStatus({
    required int sessionId,
    required int fileId,
  }) async {
    final req = chat_pb.GetIngestionStatusRequest(
      sessionId: Int64(sessionId),
      fileId: Int64(fileId),
    );
    try {
      final resp = await _authGuard.execute(() => _client.getIngestionStatus(req));
      return FileIngestionStatus(
        status: resp.status,
        lastError: resp.lastError,
        chunkCount: resp.chunkCount,
        sourceContentSha256: resp.sourceContentSha256,
        pipelineVersion: resp.pipelineVersion,
        embeddingModel: resp.embeddingModel,
      );
    } on GrpcError catch (e) {
      if (e.code == StatusCode.unauthenticated) {
        throw UnauthorizedFailure(kSessionExpiredMessage);
      }

      if (e.code == StatusCode.permissionDenied) {
        throw ApiFailure('Нет доступа (код ${e.code})');
      }

      if (e.code == StatusCode.invalidArgument) {
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }
      throwGrpcError(e, 'статус индексации файла');
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка статуса индексации');
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
        _logGrpcServerMessage(e, 'getSessionFile notFound');
        throw ApiFailure('Файл не найден (код ${e.code})');
      }
      if (e.code == StatusCode.permissionDenied) {
        _logGrpcServerMessage(e, 'getSessionFile denied');
        throw ApiFailure('Нет доступа (код ${e.code})');
      }
      if (e.code == StatusCode.invalidArgument) {
        _logGrpcServerMessage(e, 'getSessionFile invalidArgument');
        throw ApiFailure('Некорректный запрос (код ${e.code})');
      }
      throwGrpcError(e, 'получение файла сессии');
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
        _logGrpcServerMessage(e, 'applySpreadsheet');
        throw ApiFailure('Некорректные данные (код ${e.code})');
      }
      throwGrpcError(e, 'таблица');
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
        _logGrpcServerMessage(e, 'buildDocx');
        throw ApiFailure('Некорректные данные (код ${e.code})');
      }
      throwGrpcError(e, 'документ Word');
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
        _logGrpcServerMessage(e, 'applyMarkdownPatch');
        throw ApiFailure('Некорректные данные (код ${e.code})');
      }
      throwGrpcError(e, 'патч текста');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('ChatRemote: applyMarkdownPatch', exception: e);
      throw ApiFailure('Ошибка патча текста');
    }
  }
}
