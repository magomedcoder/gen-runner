import 'dart:async';
import 'dart:typed_data';

import 'package:bloc_concurrency/bloc_concurrency.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/chat_backend_user_error.dart';
import 'package:gen/core/grpc_unavailable.dart';
import 'package:gen/core/chat_image_attachment.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/request_logout_on_unauthorized.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/rag_document_preview.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/rag_ingestion_ui.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';
import 'package:gen/domain/usecases/chat/connect_usecase.dart';
import 'package:gen/domain/usecases/chat/create_session_usecase.dart';
import 'package:gen/domain/usecases/chat/delete_session_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_usecase.dart';
import 'package:gen/domain/usecases/chat/get_sessions_usecase.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/continue_assistant_usecase.dart';
import 'package:gen/domain/usecases/chat/regenerate_assistant_usecase.dart';
import 'package:gen/domain/usecases/chat/edit_user_message_and_continue_usecase.dart';
import 'package:gen/domain/usecases/chat/get_assistant_message_regenerations_usecase.dart';
import 'package:gen/domain/usecases/chat/get_user_message_edits_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_for_assistant_message_version_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_for_user_message_version_usecase.dart';
import 'package:gen/domain/usecases/chat/get_file_ingestion_status_usecase.dart';
import 'package:gen/domain/usecases/chat/put_session_file_usecase.dart';
import 'package:gen/domain/usecases/chat/send_message_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_title_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_status_usecase.dart';
import 'package:gen/domain/usecases/runners/get_user_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/get_web_search_availability_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/assistant_chat_stream.dart';
import 'package:gen/presentation/screens/chat/bloc/assistant_stream_finalize.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_stream_errors.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/session_file_rag_wait.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_runner_helpers.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_bloc.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_event.dart';

int _localTempMessageId() => -DateTime.now().microsecondsSinceEpoch;

const int _kMessagePageSize = 40;

class ChatBloc extends Bloc<ChatEvent, ChatState> {
  final AuthBloc authBloc;
  final AppTopNoticeBloc appTopNoticeBloc;
  final ConnectUseCase connectUseCase;
  final GetRunnersUseCase getRunnersUseCase;
  final GetUserRunnersUseCase getUserRunnersUseCase;
  final GetSessionSettingsUseCase getSessionSettingsUseCase;
  final UpdateSessionSettingsUseCase updateSessionSettingsUseCase;
  final SendMessageUseCase sendMessageUseCase;
  final PutSessionFileUseCase putSessionFileUseCase;
  final GetFileIngestionStatusUseCase getFileIngestionStatusUseCase;
  final RegenerateAssistantUseCase regenerateAssistantUseCase;
  final ContinueAssistantUseCase continueAssistantUseCase;
  final EditUserMessageAndContinueUseCase editUserMessageAndContinueUseCase;
  final GetUserMessageEditsUseCase getUserMessageEditsUseCase;
  final GetSessionMessagesForUserMessageVersionUseCase getSessionMessagesForUserMessageVersionUseCase;
  final GetAssistantMessageRegenerationsUseCase getAssistantMessageRegenerationsUseCase;
  final GetSessionMessagesForAssistantMessageVersionUseCase getSessionMessagesForAssistantMessageVersionUseCase;
  final CreateSessionUseCase createSessionUseCase;
  final GetSessionsUseCase getSessionsUseCase;
  final GetSessionMessagesUseCase getSessionMessagesUseCase;
  final DeleteSessionUseCase deleteSessionUseCase;
  final UpdateSessionTitleUseCase updateSessionTitleUseCase;
  final GetRunnersStatusUseCase getRunnersStatusUseCase;
  final GetSelectedRunnerUseCase getSelectedRunnerUseCase;
  final SetSelectedRunnerUseCase setSelectedRunnerUseCase;
  final GetWebSearchAvailabilityUseCase getWebSearchAvailabilityUseCase;

  StreamSubscription<ChatStreamChunk>? _streamSubscription;
  Completer<bool>? _streamCompleter;

  int _streamingAssistantMessageId = 0;

  List<RunnerInfo> _lastRunnerInfos = const [];

  Future<void> _abortAssistantStreamPipeline({
    bool resetStreamingMessageId = true,
  }) async {
    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;
    if (resetStreamingMessageId) {
      _streamingAssistantMessageId = 0;
    }
  }

  Future<void> _teardownAssistantStreamPipeline() async {
    await _streamSubscription?.cancel();
    _streamSubscription = null;
    _streamCompleter = null;
    _streamingAssistantMessageId = 0;
  }

  ChatState _copyAfterAssistantStreamSuccess({
    List<Message>? messages,
    Set<int>? regeneratedAssistantMessageIds,
  }) {
    return state.copyWith(
      messages: messages,
      regeneratedAssistantMessageIds: regeneratedAssistantMessageIds,
      isLoading: false,
      isStreaming: false,
      clearStreamingSessionId: true,
      clearStreamingParked: true,
      currentStreamingText: null,
      currentStreamingReasoning: null,
      clearToolProgress: true,
      clearRetryPayload: true,
      clearPartialAssistant: true,
      ragPreviewBySessionFile: _ragPreviewAfterClear(state),
      clearRagDocumentPreview: true,
    );
  }

  Future<void> _prefetchEditsForMessages(
    int sessionId,
    List<Message> messages,
    Emitter<ChatState> emit,
  ) async {
    final candidates = <Message>[];
    for (final m in messages) {
      if (m.role != MessageRole.user || m.id <= 0) {
        continue;
      }

      final ua = m.updatedAt;
      if (ua == null) {
        continue;
      }

      if (ua.millisecondsSinceEpoch == m.createdAt.millisecondsSinceEpoch) {
        continue;
      }

      candidates.add(m);
    }
    if (candidates.isEmpty) {
      return;
    }

    final take = candidates.length > 20
        ? candidates.sublist(candidates.length - 20)
        : candidates;
    final editsById = Map<int, List<UserMessageEdit>>.from(
      state.editsByMessageId,
    );
    final cursorById = Map<int, int>.from(state.editCursorByMessageId);
    final editedIds = <int>{...state.editedMessageIds};

    for (final m in take) {
      final existing = editsById[m.id];
      if (existing != null) {
        final preferred = state.editCursorByMessageId[m.id];
        cursorById[m.id] =
            preferred ?? (existing.isEmpty ? 0 : existing.length);
        editedIds.add(m.id);
        continue;
      }

      try {
        final editsRaw = await getUserMessageEditsUseCase(
          sessionId: sessionId,
          userMessageId: m.id,
        );

        final edits = [...editsRaw]
          ..sort((a, b) => a.createdAt.compareTo(b.createdAt));
        editsById[m.id] = edits;
        final preferred = state.editCursorByMessageId[m.id];
        cursorById[m.id] = preferred ?? (edits.isEmpty ? 0 : edits.length);
        editedIds.add(m.id);
      } catch (_) {}
      if (emit.isDone) {
        return;
      }
    }

    emit(
      state.copyWith(
        editsByMessageId: editsById,
        editCursorByMessageId: cursorById,
        editedMessageIds: editedIds,
      ),
    );
  }

  ChatBloc({
    required this.authBloc,
    required this.appTopNoticeBloc,
    required this.connectUseCase,
    required this.getRunnersUseCase,
    required this.getUserRunnersUseCase,
    required this.getSessionSettingsUseCase,
    required this.updateSessionSettingsUseCase,
    required this.sendMessageUseCase,
    required this.putSessionFileUseCase,
    required this.getFileIngestionStatusUseCase,
    required this.regenerateAssistantUseCase,
    required this.continueAssistantUseCase,
    required this.editUserMessageAndContinueUseCase,
    required this.getUserMessageEditsUseCase,
    required this.getSessionMessagesForUserMessageVersionUseCase,
    required this.getAssistantMessageRegenerationsUseCase,
    required this.getSessionMessagesForAssistantMessageVersionUseCase,
    required this.createSessionUseCase,
    required this.getSessionsUseCase,
    required this.getSessionMessagesUseCase,
    required this.deleteSessionUseCase,
    required this.updateSessionTitleUseCase,
    required this.getRunnersStatusUseCase,
    required this.getSelectedRunnerUseCase,
    required this.setSelectedRunnerUseCase,
    required this.getWebSearchAvailabilityUseCase,
  }) : super(const ChatState()) {
    on<ChatStarted>(_onChatStarted);
    on<ChatReconnectAfterConnectionRestored>(
      _onReconnectAfterConnectionRestored,
    );
    on<ChatCreateSession>(_onCreateSession);
    on<ChatLoadSessions>(_onLoadSessions);
    on<ChatSelectSession>(_onSelectSession);
    on<ChatLoadOlderMessages>(_onLoadOlderMessages, transformer: droppable());
    on<ChatSendMessage>(_onChatSendMessage, transformer: droppable());
    on<ChatClearError>(_onChatClearError);
    on<ChatDismissStreamNotice>(_onDismissStreamNotice);
    on<ChatDismissRagDocumentPreview>(_onDismissRagDocumentPreview);
    on<ChatStopGeneration>(_onChatStopGeneration);
    on<ChatRetryLastMessage>(_onRetryLastMessage);
    on<ChatRegenerateAssistant>(
      _onRegenerateAssistant,
      transformer: droppable(),
    );
    on<ChatContinueAssistant>(_onContinueAssistant, transformer: droppable());
    on<ChatEditUserMessageAndContinue>(
      _onEditUserMessageAndContinue,
      transformer: droppable(),
    );
    on<ChatShowUserMessageEdits>(
      _onShowUserMessageEdits,
      transformer: droppable(),
    );
    on<ChatNavigateUserMessageEdit>(
      _onNavigateUserMessageEdit,
      transformer: droppable(),
    );
    on<ChatShowAssistantMessageRegenerations>(
      _onShowAssistantMessageRegenerations,
      transformer: droppable(),
    );
    on<ChatNavigateAssistantMessageRegeneration>(
      _onNavigateAssistantMessageRegeneration,
      transformer: droppable(),
    );
    on<ChatDeleteSession>(_onDeleteSession);
    on<ChatUpdateSessionTitle>(_onUpdateSessionTitle);
    on<ChatLoadRunners>(_onLoadRunners);
    on<ChatSelectRunner>(_onSelectRunner);
    on<ChatLoadSessionSettings>(_onLoadSessionSettings);
    on<ChatUpdateSessionSettings>(_onUpdateSessionSettings);
    on<ChatSetModelReasoning>(_onSetModelReasoning);
    on<ChatSetWebSearch>(_onSetWebSearch);
    on<ChatSetMcp>(_onSetMcp);
  }

  void _reportServerUnreachableIfNeeded(Object e) {
    if (isGrpcUnavailable(e)) {
      appTopNoticeBloc.add(const AppTopNoticeReportUnreachable());
    }
  }

  Future<void> _resyncSessionMessagesAfterStream(
    int sessionId,
    Emitter<ChatState> emit,
  ) async {
    if (isClosed) {
      return;
    }
    try {
      final page = await getSessionMessagesUseCase(
        sessionId,
        beforeMessageId: 0,
        pageSize: _kMessagePageSize,
      );
      if (isClosed || state.currentSessionId != sessionId) {
        return;
      }
      final cleanedRegen = state.regeneratedAssistantMessageIds
          .where((id) => id > 0)
          .toSet();
      emit(
        state.copyWith(
          messages: page.messages,
          hasMoreOlderMessages: page.hasMoreOlder,
          error: null,
          regeneratedAssistantMessageIds: cleanedRegen,
        ),
      );
      await _prefetchEditsForMessages(sessionId, page.messages, emit);
    } catch (e, st) {
      Logs().w(
        'ChatBloc: не удалось синхронизировать сообщения после стрима',
        exception: e,
        stackTrace: st,
      );
      requestLogoutIfUnauthorized(e, authBloc);
    }
  }

  Future<void> _onShowUserMessageEdits(
    ChatShowUserMessageEdits event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }

    if (state.isStreamingInCurrentSession) {
      return;
    }

    try {
      final editsRaw = await getUserMessageEditsUseCase(
        sessionId: sessionId,
        userMessageId: event.userMessageId,
      );
      final edits = [...editsRaw]
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt));

      final editsById = Map<int, List<UserMessageEdit>>.from(
        state.editsByMessageId,
      );
      editsById[event.userMessageId] = edits;

      final cursorById = Map<int, int>.from(state.editCursorByMessageId);
      cursorById[event.userMessageId] = edits.isEmpty ? 0 : edits.length;

      final pending = state.pendingEditNavDeltaByMessageId[event.userMessageId];
      final pendingMap = Map<int, int>.from(
        state.pendingEditNavDeltaByMessageId,
      );
      pendingMap.remove(event.userMessageId);

      if (pending != null && edits.isNotEmpty) {
        final versionsCount = edits.length + 1;
        final cur = cursorById[event.userMessageId] ?? (versionsCount - 1);
        cursorById[event.userMessageId] = (cur + pending).clamp(
          0,
          versionsCount - 1,
        );
      }

      emit(
        state.copyWith(
          editsByMessageId: editsById,
          editCursorByMessageId: cursorById,
          pendingEditNavDeltaByMessageId: pendingMap,
        ),
      );
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось загрузить историю правок',
          ),
        ),
      );
    }
  }

  Future<void> _onNavigateUserMessageEdit(
    ChatNavigateUserMessageEdit event,
    Emitter<ChatState> emit,
  ) async {
    final edits = state.editsByMessageId[event.userMessageId];
    if (edits == null) {
      final pending = Map<int, int>.from(state.pendingEditNavDeltaByMessageId);
      pending[event.userMessageId] = event.delta;
      emit(state.copyWith(pendingEditNavDeltaByMessageId: pending));
      add(ChatShowUserMessageEdits(event.userMessageId));
      return;
    }

    if (edits.isEmpty) {
      return;
    }

    final versionsCount = edits.length + 1;
    final current =
        state.editCursorByMessageId[event.userMessageId] ?? (versionsCount - 1);
    final next = (current + event.delta).clamp(0, versionsCount - 1);
    if (next == current) {
      return;
    }

    final cursorById = Map<int, int>.from(state.editCursorByMessageId);
    cursorById[event.userMessageId] = next;
    emit(state.copyWith(editCursorByMessageId: cursorById));
    final sessionId = state.currentSessionId;
    if (sessionId != null) {
      try {
        final view = await getSessionMessagesForUserMessageVersionUseCase(
          sessionId: sessionId,
          userMessageId: event.userMessageId,
          versionIndex: next,
        );

        if (emit.isDone) {
          return;
        }

        emit(state.copyWith(messages: view));
      } catch (e) {
        requestLogoutIfUnauthorized(e, authBloc);
        _reportServerUnreachableIfNeeded(e);
        if (emit.isDone) {
          return;
        }

        emit(
          state.copyWith(
            error: chatHeadlineIfBackendReachable(
              e,
              'Не удалось загрузить ветку версии',
            ),
          ),
        );
      }
    }
  }

  Future<void> _onShowAssistantMessageRegenerations(
    ChatShowAssistantMessageRegenerations event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }
    if (state.isStreamingInCurrentSession) {
      return;
    }
    try {
      final rowsRaw = await getAssistantMessageRegenerationsUseCase(
        sessionId: sessionId,
        assistantMessageId: event.assistantMessageId,
      );
      final rows = [...rowsRaw]
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt));

      final byId = Map<int, List<AssistantMessageRegeneration>>.from(
        state.regenerationsByMessageId,
      );
      byId[event.assistantMessageId] = rows;

      final cursorById = Map<int, int>.from(
        state.regenerationCursorByMessageId,
      );
      cursorById[event.assistantMessageId] = rows.isEmpty ? 0 : rows.length;

      final pending = state
          .pendingRegenerationNavDeltaByMessageId[event.assistantMessageId];
      final pendingMap = Map<int, int>.from(
        state.pendingRegenerationNavDeltaByMessageId,
      );
      pendingMap.remove(event.assistantMessageId);

      if (pending != null && rows.isNotEmpty) {
        final versionsCount = rows.length + 1;
        final cur = cursorById[event.assistantMessageId] ?? (versionsCount - 1);
        cursorById[event.assistantMessageId] = (cur + pending).clamp(
          0,
          versionsCount - 1,
        );
      }

      final regeneratedIds = <int>{
        ...state.regeneratedAssistantMessageIds,
        event.assistantMessageId,
      };

      emit(
        state.copyWith(
          regenerationsByMessageId: byId,
          regenerationCursorByMessageId: cursorById,
          pendingRegenerationNavDeltaByMessageId: pendingMap,
          regeneratedAssistantMessageIds: regeneratedIds,
        ),
      );
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось загрузить историю перегенераций',
          ),
        ),
      );
    }
  }

  Future<void> _onNavigateAssistantMessageRegeneration(
    ChatNavigateAssistantMessageRegeneration event,
    Emitter<ChatState> emit,
  ) async {
    final regens = state.regenerationsByMessageId[event.assistantMessageId];
    if (regens == null) {
      final pending = Map<int, int>.from(
        state.pendingRegenerationNavDeltaByMessageId,
      );
      pending[event.assistantMessageId] = event.delta;
      emit(state.copyWith(pendingRegenerationNavDeltaByMessageId: pending));
      add(ChatShowAssistantMessageRegenerations(event.assistantMessageId));
      return;
    }

    if (regens.isEmpty) {
      return;
    }

    final versionsCount = regens.length + 1;
    final current =
        state.regenerationCursorByMessageId[event.assistantMessageId] ??
        (versionsCount - 1);
    final next = (current + event.delta).clamp(0, versionsCount - 1);
    if (next == current) {
      return;
    }

    final cursorById = Map<int, int>.from(state.regenerationCursorByMessageId);
    cursorById[event.assistantMessageId] = next;
    emit(state.copyWith(regenerationCursorByMessageId: cursorById));

    final sessionId = state.currentSessionId;
    if (sessionId != null) {
      try {
        final view = await getSessionMessagesForAssistantMessageVersionUseCase(
          sessionId: sessionId,
          assistantMessageId: event.assistantMessageId,
          versionIndex: next,
        );

        if (emit.isDone) {
          return;
        }

        emit(state.copyWith(messages: view));
      } catch (e) {
        requestLogoutIfUnauthorized(e, authBloc);
        _reportServerUnreachableIfNeeded(e);
        if (emit.isDone) {
          return;
        }
        emit(
          state.copyWith(
            error: chatHeadlineIfBackendReachable(
              e,
              'Не удалось загрузить версию ответа',
            ),
          ),
        );
      }
    }
  }

  Future<void> _onEditUserMessageAndContinue(
    ChatEditUserMessageAndContinue event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }

    if (state.isStreamingInCurrentSession) {
      return;
    }

    final newText = event.newContent.trim();
    if (newText.isEmpty) {
      return;
    }

    final idx = state.messages.indexWhere((m) => m.id == event.userMessageId);
    if (idx < 0) {
      return;
    }

    final target = state.messages[idx];
    if (target.role != MessageRole.user || target.id <= 0) {
      return;
    }

    await _abortAssistantStreamPipeline();

    final updatedUser = Message(
      id: target.id,
      content: newText,
      role: MessageRole.user,
      createdAt: target.createdAt,
      updatedAt: DateTime.now(),
      attachmentFileName: target.attachmentFileName,
      attachmentFileNames: target.attachmentFileNames,
      attachmentMime: target.attachmentMime,
      attachmentContent: target.attachmentContent,
      attachmentFileId: target.attachmentFileId,
      attachmentFileIds: target.attachmentFileIds,
    );
    final prefixMessages = [...state.messages.sublist(0, idx), updatedUser];

    final acc = AssistantStreamAccum();

    final edited = <int>{...state.editedMessageIds, target.id};
    emit(
      state.copyWith(
        messages: prefixMessages,
        editedMessageIds: edited,
        isLoading: true,
        isStreaming: true,
        streamingSessionId: sessionId,
        clearStreamingParked: true,
        currentStreamingText: '',
        currentStreamingReasoning: null,
        clearToolProgress: true,
        error: null,
        clearRetryPayload: true,
        clearPartialAssistant: true,
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );

    _streamCompleter = Completer<bool>();

    try {
      final stream = editUserMessageAndContinueUseCase(
        sessionId,
        event.userMessageId,
        newText,
      );

      _streamSubscription = subscribeAssistantChatStream(
        stream,
        completer: _streamCompleter!,
        onChunk: (chunk) => handleAssistantChatStreamChunk(
          chunk: chunk,
          emit: emit,
          currentState: () => state,
          isCurrentSession: () => state.currentSessionId == sessionId,
          acc: acc,
          onAssistantMessageId: (id) {
            if (id > 0) {
              _streamingAssistantMessageId = id;
            }
          },
        ),
      );

      final cancelled = await _streamCompleter!.future;
      if (cancelled) {
        return;
      }

      final fin = finalizeAssistantStreamAccum(acc);
      if (fin != null) {
        final assistantMessage = assistantMessageFromStreamFinal(
          fin: fin,
          streamingAssistantMessageId: _streamingAssistantMessageId,
          fallbackMessageId: _localTempMessageId(),
        );

        final merged = [...prefixMessages, assistantMessage];
        if (state.currentSessionId == sessionId) {
          emit(_copyAfterAssistantStreamSuccess(messages: merged));
          await _resyncSessionMessagesAfterStream(sessionId, emit);
          if (!isClosed && state.currentSessionId == sessionId) {
            add(ChatShowUserMessageEdits(event.userMessageId));
          }
        } else {
          emit(_copyAfterAssistantStreamSuccess());
        }
      } else {
        emit(
          state.copyWith(
            messages: state.currentSessionId == sessionId
                ? prefixMessages
                : null,
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            currentStreamingText: null,
            currentStreamingReasoning: null,
            clearToolProgress: true,
            error: kChatEmptyAssistantResponseMessage,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
      }
    } on Object catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          messages: state.currentSessionId == sessionId ? prefixMessages : null,
          isLoading: false,
          isStreaming: false,
          clearStreamingSessionId: true,
          clearStreamingParked: true,
          currentStreamingReasoning: null,
          error: chatStreamErrorForState(
            e,
            lead: 'Не удалось отредактировать сообщение',
          ),
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
    } finally {
      await _teardownAssistantStreamPipeline();
    }
  }

  Future<void> _onChatStarted(
    ChatStarted event,
    Emitter<ChatState> emit,
  ) async {
    Logs().d('ChatBloc: старт загрузки чата');
    emit(state.copyWith(isLoading: true));

    try {
      final isConnected = await connectUseCase();

      bool? hasActiveRunners;
      try {
        hasActiveRunners = await getRunnersStatusUseCase();
      } catch (_) {
        hasActiveRunners = true;
      }

      if (isConnected) {
        try {
          final sessionsFuture = getSessionsUseCase(page: 1, pageSize: 20);
          final sessions = await sessionsFuture;
          final isAdmin = authBloc.state.user?.isAdmin ?? false;

          List<RunnerInfo> runnerInfosSnapshot = const [];
          List<String> runners = const [];
          Map<String, String> runnerNames = const {};
          String? selectedRunner;
          try {
            if (isAdmin) {
              final ri = await getRunnersUseCase();
              runnerInfosSnapshot = ri;
              runners = extractAvailableRunners(ri);
              runnerNames = extractRunnerNames(ri);
              if (runners.isNotEmpty && state.selectedRunner == null) {
                final defaultRunner = await getSelectedRunnerUseCase();
                if (defaultRunner != null && runners.contains(defaultRunner)) {
                  selectedRunner = defaultRunner;
                } else {
                  selectedRunner = runners.first;
                  try {
                    await setSelectedRunnerUseCase(selectedRunner);
                  } catch (_) {}
                }
              }
            } else {
              try {
                final ri = await getUserRunnersUseCase();
                runnerInfosSnapshot = ri;
                runners = extractAvailableRunners(ri);
                runnerNames = extractRunnerNames(ri);

                final saved = await getSelectedRunnerUseCase();
                if (saved != null &&
                    saved.isNotEmpty &&
                    runners.contains(saved)) {
                  selectedRunner = saved;
                } else if (runners.isNotEmpty) {
                  selectedRunner = runners.first;
                  try {
                    await setSelectedRunnerUseCase(selectedRunner);
                  } catch (_) {}
                }
              } catch (_) {}
            }
          } catch (_) {}

          const List<Message> messages = <Message>[];
          if (selectedRunner == null && runners.isNotEmpty) {
            selectedRunner = runners.first;
          }

          _lastRunnerInfos = runnerInfosSnapshot;
          final effectiveSel = selectedRunner ?? state.selectedRunner;
          final runnerHealth = runnerHealthForSelection(
            effectiveSel,
            runnerInfosSnapshot,
          );

          final webSearchGlobal = await getWebSearchAvailabilityUseCase();

          Logs().i('ChatBloc: чат загружен, сессий: ${sessions.length}');
          emit(
            state.copyWith(
              isConnected: isConnected,
              hasCompletedInitialConnection: true,
              isLoading: false,
              sessions: sessions,
              currentSessionId: null,
              messages: messages,
              hasMoreOlderMessages: false,
              clearPartialAssistant: true,
              runners: runners,
              runnerNames: runnerNames,
              selectedRunner: selectedRunner ?? state.selectedRunner,
              hasActiveRunners: hasActiveRunners,
              selectedRunnerEnabled: runnerHealth.$1,
              selectedRunnerConnected: runnerHealth.$2,
              error: null,
              draftModelReasoningEnabled: false,
              webSearchGloballyEnabled: webSearchGlobal,
              draftWebSearchEnabled: false,
              draftWebSearchProvider: '',
              draftMcpEnabled: false,
              draftMcpServerIds: const [],
            ),
          );
        } catch (e) {
          Logs().e('ChatBloc: ошибка загрузки сессий', exception: e);
          requestLogoutIfUnauthorized(e, authBloc);
          _reportServerUnreachableIfNeeded(e);
          emit(
            state.copyWith(
              isConnected: isGrpcUnavailable(e) ? false : isConnected,
              hasCompletedInitialConnection: true,
              isLoading: false,
              hasActiveRunners: hasActiveRunners,
              webSearchGloballyEnabled: false,
              error: chatHeadlineIfBackendReachable(
                e,
                'Не удалось загрузить список чатов',
              ),
            ),
          );
        }
      } else {
        Logs().w('ChatBloc: не удалось подключиться к серверу');
        emit(
          state.copyWith(
            isConnected: isConnected,
            hasCompletedInitialConnection: true,
            isLoading: false,
            webSearchGloballyEnabled: false,
            error: null,
          ),
        );
      }
    } catch (e) {
      Logs().e('ChatBloc: ошибка подключения', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isConnected: false,
          hasCompletedInitialConnection: true,
          isLoading: false,
          webSearchGloballyEnabled: false,
          error: null,
        ),
      );
    }
  }

  Future<void> _onCreateSession(
    ChatCreateSession event,
    Emitter<ChatState> emit,
  ) async {
    if (!state.isStreaming &&
        state.currentSessionId == null &&
        state.messages.isEmpty) {
      return;
    }

    await _abortAssistantStreamPipeline();

    emit(
      state.copyWith(
        currentSessionId: null,
        messages: const [],
        error: null,
        currentStreamingText: null,
        currentStreamingReasoning: null,
        isLoading: false,
        isStreaming: false,
        clearStreamingSessionId: true,
        clearStreamingParked: true,
        clearToolProgress: true,
        editedMessageIds: const {},
        editsByMessageId: const {},
        editCursorByMessageId: const {},
        pendingEditNavDeltaByMessageId: const {},
        regeneratedAssistantMessageIds: const {},
        regenerationsByMessageId: const {},
        regenerationCursorByMessageId: const {},
        pendingRegenerationNavDeltaByMessageId: const {},
        hasMoreOlderMessages: false,
        isLoadingOlderMessages: false,
        clearPartialAssistant: true,
        draftModelReasoningEnabled: false,
        draftWebSearchEnabled: false,
        draftWebSearchProvider: '',
        draftMcpEnabled: false,
        draftMcpServerIds: const [],
      ),
    );
  }

  Future<void> _onReconnectAfterConnectionRestored(
    ChatReconnectAfterConnectionRestored event,
    Emitter<ChatState> emit,
  ) async {
    if (!authBloc.state.isAuthenticated) {
      return;
    }

    Logs().i('ChatBloc: восстановление связи - обновление сессий и чата');
    emit(state.copyWith(isLoading: true, error: null));

    try {
      final isConnected = await connectUseCase();

      bool? hasActiveRunners;
      try {
        hasActiveRunners = await getRunnersStatusUseCase();
      } catch (_) {
        hasActiveRunners = true;
      }

      if (!isConnected) {
        emit(state.copyWith(isLoading: false, isConnected: false));
        return;
      }

      final sessions = await getSessionsUseCase(page: 1, pageSize: 20);
      final isAdmin = authBloc.state.user?.isAdmin ?? false;

      List<RunnerInfo> runnerInfosSnapshot = const [];
      List<String> runners = const [];
      Map<String, String> runnerNames = const {};
      String? selectedRunner;
      try {
        if (isAdmin) {
          final ri = await getRunnersUseCase();
          runnerInfosSnapshot = ri;
          runners = extractAvailableRunners(ri);
          runnerNames = extractRunnerNames(ri);
          if (runners.isNotEmpty && state.selectedRunner == null) {
            final defaultRunner = await getSelectedRunnerUseCase();
            if (defaultRunner != null && runners.contains(defaultRunner)) {
              selectedRunner = defaultRunner;
            } else {
              selectedRunner = runners.first;
              try {
                await setSelectedRunnerUseCase(selectedRunner);
              } catch (_) {}
            }
          }
        } else {
          try {
            final ri = await getUserRunnersUseCase();
            runnerInfosSnapshot = ri;
            runners = extractAvailableRunners(ri);
            runnerNames = extractRunnerNames(ri);

            final saved = await getSelectedRunnerUseCase();
            if (saved != null && saved.isNotEmpty && runners.contains(saved)) {
              selectedRunner = saved;
            } else if (runners.isNotEmpty) {
              selectedRunner = runners.first;
              try {
                await setSelectedRunnerUseCase(selectedRunner);
              } catch (_) {}
            }
          } catch (_) {}
        }
      } catch (_) {}

      if (selectedRunner == null && runners.isNotEmpty) {
        selectedRunner = runners.first;
      }

      _lastRunnerInfos = runnerInfosSnapshot;
      final effectiveReconnectSel = selectedRunner ?? state.selectedRunner;
      final reconnectHealth = runnerHealthForSelection(
        effectiveReconnectSel,
        runnerInfosSnapshot,
      );

      final webSearchGlobal = await getWebSearchAvailabilityUseCase();

      final previousSessionId = state.currentSessionId;

      emit(
        state.copyWith(
          isConnected: true,
          isLoading: previousSessionId != null,
          sessions: sessions,
          runners: runners,
          runnerNames: runnerNames,
          selectedRunner: selectedRunner ?? state.selectedRunner,
          hasActiveRunners: hasActiveRunners,
          selectedRunnerEnabled: reconnectHealth.$1,
          selectedRunnerConnected: reconnectHealth.$2,
          webSearchGloballyEnabled: webSearchGlobal,
          draftWebSearchEnabled: webSearchGlobal
              ? state.draftWebSearchEnabled
              : false,
          draftWebSearchProvider: webSearchGlobal
              ? state.draftWebSearchProvider
              : '',
          error: null,
        ),
      );

      if (previousSessionId != null) {
        final stillExists = sessions.any((s) => s.id == previousSessionId);
        if (stillExists) {
          add(ChatSelectSession(previousSessionId, forceReload: true));
        } else {
          emit(
            state.copyWith(
              currentSessionId: null,
              messages: const [],
              isLoading: false,
              hasMoreOlderMessages: false,
              clearPartialAssistant: true,
              editedMessageIds: const {},
              editsByMessageId: const {},
              editCursorByMessageId: const {},
              pendingEditNavDeltaByMessageId: const {},
              regeneratedAssistantMessageIds: const {},
              regenerationsByMessageId: const {},
              regenerationCursorByMessageId: const {},
              pendingRegenerationNavDeltaByMessageId: const {},
            ),
          );
        }
      } else {
        emit(state.copyWith(isLoading: false));
      }
    } catch (e) {
      Logs().e(
        'ChatBloc: ошибка обновления после восстановления связи',
        exception: e,
      );
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          error: isGrpcUnavailable(e)
              ? null
              : 'Не удалось обновить данные чата',
        ),
      );
    }
  }

  Future<void> _onLoadSessions(
    ChatLoadSessions event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      final sessions = await getSessionsUseCase(
        page: event.page,
        pageSize: event.pageSize,
      );

      emit(state.copyWith(sessions: sessions, isLoading: false, error: null));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось загрузить список чатов',
          ),
        ),
      );
    }
  }

  Future<void> _onSelectSession(
    ChatSelectSession event,
    Emitter<ChatState> emit,
  ) async {
    if (state.currentSessionId == event.sessionId && !event.forceReload) {
      return;
    }

    if (!event.forceReload &&
        state.isStreaming &&
        state.streamingSessionId == event.sessionId &&
        state.streamingParkedMessages != null) {
      emit(
        state.copyWith(
          currentSessionId: event.sessionId,
          messages: state.streamingParkedMessages!,
          clearStreamingParked: true,
          isLoading: false,
          error: null,
          editedMessageIds: const {},
          editsByMessageId: const {},
          editCursorByMessageId: const {},
          pendingEditNavDeltaByMessageId: const {},
          regeneratedAssistantMessageIds: const {},
          regenerationsByMessageId: const {},
          regenerationCursorByMessageId: const {},
          pendingRegenerationNavDeltaByMessageId: const {},
          hasMoreOlderMessages: false,
          isLoadingOlderMessages: false,
          clearPartialAssistant: true,
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
      return;
    }

    final parkStream =
        state.isStreaming &&
        state.streamingSessionId == state.currentSessionId &&
        state.currentSessionId != null;
    final nextParked = parkStream
        ? List<Message>.from(state.messages)
        : state.streamingParkedMessages;

    emit(
      state.copyWith(
        currentSessionId: event.sessionId,
        messages: const [],
        isLoading: true,
        error: null,
        editedMessageIds: const {},
        editsByMessageId: const {},
        editCursorByMessageId: const {},
        pendingEditNavDeltaByMessageId: const {},
        regeneratedAssistantMessageIds: const {},
        regenerationsByMessageId: const {},
        regenerationCursorByMessageId: const {},
        pendingRegenerationNavDeltaByMessageId: const {},
        hasMoreOlderMessages: false,
        isLoadingOlderMessages: false,
        clearPartialAssistant: true,
        streamingParkedMessages: nextParked,
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );

    try {
      final page = await getSessionMessagesUseCase(
        event.sessionId,
        beforeMessageId: 0,
        pageSize: _kMessagePageSize,
      );

      String? runnerForSession = state.selectedRunner;
      if (state.runners.isNotEmpty) {
        if (runnerForSession == null ||
            !state.runners.contains(runnerForSession)) {
          runnerForSession = state.runners.first;
        }
      }

      emit(
        state.copyWith(
          messages: page.messages,
          isLoading: false,
          selectedRunner: runnerForSession,
          hasMoreOlderMessages: page.hasMoreOlder,
        ),
      );
      await _prefetchEditsForMessages(event.sessionId, page.messages, emit);
      add(ChatLoadSessionSettings(event.sessionId));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось загрузить сообщения',
          ),
        ),
      );
    }
  }

  Future<void> _onLoadOlderMessages(
    ChatLoadOlderMessages event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null ||
        state.isLoadingOlderMessages ||
        !state.hasMoreOlderMessages ||
        state.messages.isEmpty) {
      return;
    }

    emit(state.copyWith(isLoadingOlderMessages: true, error: null));

    try {
      final beforeId = state.messages.first.id;
      final page = await getSessionMessagesUseCase(
        sessionId,
        beforeMessageId: beforeId,
        pageSize: _kMessagePageSize,
      );

      emit(
        state.copyWith(
          messages: [...page.messages, ...state.messages],
          hasMoreOlderMessages: page.hasMoreOlder,
          isLoadingOlderMessages: false,
          error: null,
        ),
      );
      await _prefetchEditsForMessages(sessionId, page.messages, emit);
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoadingOlderMessages: false,
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось загрузить сообщения',
          ),
        ),
      );
    }
  }

  bool _isSameAttachment(List<int>? a, Uint8List? b) {
    if (a == null && b == null) {
      return true;
    }

    if (a == null || b == null) {
      return false;
    }

    if (a.length != b.length) {
      return false;
    }

    for (var i = 0; i < a.length; i++) {
      if (a[i] != b[i]) {
        return false;
      }
    }

    return true;
  }

  Future<void> _onChatSendMessage(
    ChatSendMessage event,
    Emitter<ChatState> emit,
  ) async {
    await _sendMessageInternal(event, emit, allowReuseLastUserMessage: false);
  }

  Future<void> _sendMessageInternal(
    ChatSendMessage event,
    Emitter<ChatState> emit, {
    required bool allowReuseLastUserMessage,
  }) async {
    final text = event.text.trim();
    final attachmentNames = <String>[
      ...event.attachmentFileNames.where((n) => n.trim().isNotEmpty),
    ];
    final attachmentContents = <List<int>>[
      ...event.attachmentContents.where((bytes) => bytes.isNotEmpty),
    ];
    if (attachmentNames.isEmpty &&
        event.attachmentFileName != null &&
        event.attachmentFileName!.trim().isNotEmpty) {
      attachmentNames.add(event.attachmentFileName!.trim());
    }
    if (attachmentContents.isEmpty &&
        event.attachmentContent != null &&
        event.attachmentContent!.isNotEmpty) {
      attachmentContents.add(event.attachmentContent!);
    }
    final hasAttachmentBytes =
        attachmentNames.isNotEmpty &&
        attachmentNames.length == attachmentContents.length;
    final attachmentFileIds = <int>[
      ...event.attachmentFileIds.where((id) => id > 0),
    ];
    if (attachmentFileIds.isEmpty &&
        event.attachmentFileId != null &&
        event.attachmentFileId! > 0) {
      attachmentFileIds.add(event.attachmentFileId!);
    }
    final hasAttachmentById = attachmentFileIds.isNotEmpty;
    if (text.isEmpty && !hasAttachmentBytes && !hasAttachmentById) {
      return;
    }

    await _abortAssistantStreamPipeline();

    var sessionId = state.currentSessionId;
    final draftReasoning = state.draftModelReasoningEnabled;
    final draftWebSearch = state.draftWebSearchEnabled;
    final draftWebProv = state.draftWebSearchProvider;
    final draftMcpIds = state.draftMcpServerIds;
    final draftMcp = draftMcpIds.isNotEmpty;
    if (sessionId == null) {
      try {
        final session = await createSessionUseCase();
        sessionId = session.id;

        final updatedSessions = [session, ...state.sessions];

        emit(
          state.copyWith(
            currentSessionId: sessionId,
            sessions: updatedSessions,
            messages: const [],
          ),
        );
        if (draftReasoning || draftWebSearch || draftMcp) {
          try {
            final settings = await updateSessionSettingsUseCase(
              sessionId: sessionId,
              systemPrompt: '',
              stopSequences: const [],
              timeoutSeconds: 0,
              temperature: null,
              topK: null,
              topP: null,
              profile: '',
              modelReasoningEnabled: draftReasoning,
              webSearchEnabled: draftWebSearch,
              webSearchProvider: draftWebProv,
              mcpEnabled: draftMcp,
              mcpServerIds: draftMcpIds,
            );
            emit(state.copyWith(sessionSettings: settings));
          } catch (e) {
            requestLogoutIfUnauthorized(e, authBloc);
            _reportServerUnreachableIfNeeded(e);
            emit(
              state.copyWith(
                error: isGrpcUnavailable(e)
                    ? null
                    : userSafeErrorMessage(
                        e,
                        fallback:
                            'Не удалось сохранить настройки чата (размышление / поиск / MCP)',
                      ),
                isLoading: false,
              ),
            );
            return;
          }
        } else {
          add(ChatLoadSessionSettings(sessionId));
        }
      } catch (e) {
        requestLogoutIfUnauthorized(e, authBloc);
        _reportServerUnreachableIfNeeded(e);
        emit(
          state.copyWith(
            error: chatHeadlineIfBackendReachable(
              e,
              'Не удалось создать чат',
            ),
            isLoading: false,
          ),
        );
        return;
      }
    }

    final resolvedAttachmentFileIds = <int>[...attachmentFileIds];
    if (hasAttachmentBytes) {
      try {
        for (var i = 0; i < attachmentNames.length; i++) {
          final uploadName = attachmentNames[i];
          if (filenameEligibleForSessionRag(uploadName) && !isClosed) {
            emit(
              state.copyWith(
                ragIngestionUi: RagIngestionUi.uploading(uploadName),
              ),
            );
          }
          final uploadedFileId = await putSessionFileUseCase(
            sessionId: sessionId,
            filename: uploadName,
            content: attachmentContents[i],
          );
          resolvedAttachmentFileIds.add(uploadedFileId);
        }
      } on Object catch (e) {
        Logs().e('ChatBloc: ошибка загрузки вложения', exception: e);
        requestLogoutIfUnauthorized(e, authBloc);
        _reportServerUnreachableIfNeeded(e);
        final msg = chatHeadlineIfBackendReachable(
          e,
          'Не удалось загрузить файл',
        );
        emit(
          state.copyWith(
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            error: msg,
            clearRagIngestionUi: true,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
        return;
      }
    }

    final hasResolvedFiles = resolvedAttachmentFileIds.isNotEmpty;
    if (text.isEmpty && !hasResolvedFiles) {
      return;
    }

    var useFileRag = false;
    final attachName = attachmentNames.isNotEmpty
        ? attachmentNames.first
        : (event.attachmentFileName ?? '');
    final ragEligible =
        hasResolvedFiles &&
        resolvedAttachmentFileIds.length == 1 &&
        attachName.isNotEmpty &&
        filenameEligibleForSessionRag(attachName);
    if (ragEligible) {
      useFileRag = await waitForSessionFileRagReady(
        sessionId,
        resolvedAttachmentFileIds.first,
        getFileIngestionStatusUseCase,
        onPoll: (s) {
          if (!isClosed) {
            emit(
              state.copyWith(
                ragIngestionUi: RagIngestionUi.fromPoll(attachName, s),
              ),
            );
          }
        },
      );
    }

    final firstAttachName =
        attachmentNames.isNotEmpty ? attachmentNames.first : null;
    final userMessage = Message(
      id: _localTempMessageId(),
      content: text,
      role: MessageRole.user,
      createdAt: DateTime.now(),
      attachmentFileName: firstAttachName,
      attachmentFileNames: attachmentNames,
      attachmentMime: guessImageMimeFromFilename(firstAttachName),
      attachmentContent: null,
      attachmentFileId: hasResolvedFiles
          ? resolvedAttachmentFileIds.first
          : null,
      attachmentFileIds: resolvedAttachmentFileIds,
      useFileRag: useFileRag,
    );

    var updatedMessages = [...state.messages, userMessage];
    if (allowReuseLastUserMessage && state.messages.isNotEmpty) {
      final last = state.messages.last;
      final sameUserMessage =
          last.role == MessageRole.user &&
          last.content == text &&
          last.attachmentFileName == userMessage.attachmentFileName &&
          last.attachmentMime == userMessage.attachmentMime &&
          last.attachmentFileId == userMessage.attachmentFileId &&
          last.attachmentFileIds.length ==
              userMessage.attachmentFileIds.length &&
          last.attachmentFileIds.every(
            userMessage.attachmentFileIds.contains,
          ) &&
          _isSameAttachment(event.attachmentContent, last.attachmentContent);
      if (sameUserMessage) {
        updatedMessages = [...state.messages];
      }
    }
    final acc = AssistantStreamAccum();

    emit(
      state.copyWith(
        messages: updatedMessages,
        isLoading: true,
        isStreaming: true,
        streamingSessionId: sessionId,
        clearStreamingParked: true,
        currentStreamingText: '',
        currentStreamingReasoning: null,
        clearToolProgress: true,
        error: null,
        clearRetryPayload: true,
        clearPartialAssistant: true,
        clearRagIngestionUi: true,
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );

    _streamCompleter = Completer<bool>();

    try {
      final stream = sendMessageUseCase(sessionId, updatedMessages.last);

      _streamSubscription = subscribeAssistantChatStream(
        stream,
        completer: _streamCompleter!,
        onChunk: (chunk) => handleAssistantChatStreamChunk(
          chunk: chunk,
          emit: emit,
          currentState: () => state,
          isCurrentSession: () => state.currentSessionId == sessionId,
          acc: acc,
          onAssistantMessageId: (id) {
            if (id > 0) {
              _streamingAssistantMessageId = id;
            }
          },
        ),
      );

      final cancelled = await _streamCompleter!.future;

      if (cancelled) {
        return;
      }

      final finSend = finalizeAssistantStreamAccum(acc);
      if (finSend != null) {
        final assistantMessage = assistantMessageFromStreamFinal(
          fin: finSend,
          streamingAssistantMessageId: _streamingAssistantMessageId,
          fallbackMessageId: _localTempMessageId(),
        );

        final allMessages = [...updatedMessages, assistantMessage];

        if (state.currentSessionId == sessionId) {
          emit(_copyAfterAssistantStreamSuccess(messages: allMessages));
          await _resyncSessionMessagesAfterStream(sessionId, emit);
        } else {
          emit(_copyAfterAssistantStreamSuccess());
        }
      } else {
        Logs().w('ChatBloc: пустой ответ от сервера при отправке сообщения');
        emit(
          state.copyWith(
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            currentStreamingText: null,
            currentStreamingReasoning: null,
            clearToolProgress: true,
            error: kChatEmptyAssistantResponseMessage,
            retryText: event.text,
            retryAttachmentFileName: userMessage.attachmentFileName,
            retryAttachmentFileNames: userMessage.attachmentFileNames,
            retryAttachmentContent: null,
            retryAttachmentContents: const [],
            retryAttachmentFileId: userMessage.attachmentFileId,
            retryAttachmentFileIds: userMessage.attachmentFileIds,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
      }
    } on Object catch (e) {
      Logs().e('ChatBloc: ошибка отправки сообщения', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          isStreaming: false,
          clearStreamingSessionId: true,
          clearStreamingParked: true,
          currentStreamingReasoning: null,
          error: chatStreamErrorForState(
            e,
            lead: 'Не удалось отправить сообщение',
          ),
          retryText: event.text,
          retryAttachmentFileName: userMessage.attachmentFileName,
          retryAttachmentFileNames: userMessage.attachmentFileNames,
          retryAttachmentContent: null,
          retryAttachmentContents: const [],
          retryAttachmentFileId: userMessage.attachmentFileId,
          retryAttachmentFileIds: userMessage.attachmentFileIds,
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
    } finally {
      await _teardownAssistantStreamPipeline();
    }
  }

  Future<void> _onRegenerateAssistant(
    ChatRegenerateAssistant event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }
    if (state.isStreamingInCurrentSession) {
      return;
    }

    final idx = state.messages.indexWhere(
      (m) => m.id == event.assistantMessageId,
    );
    if (idx < 0) {
      return;
    }
    if (idx != state.messages.length - 1) {
      return;
    }
    final target = state.messages[idx];
    if (target.role != MessageRole.assistant) {
      return;
    }

    await _abortAssistantStreamPipeline();

    final prefixMessages = state.messages.sublist(0, idx);
    final previousAssistant = target;
    final acc = AssistantStreamAccum();

    emit(
      state.copyWith(
        messages: prefixMessages,
        isLoading: true,
        isStreaming: true,
        streamingSessionId: sessionId,
        clearStreamingParked: true,
        currentStreamingText: '',
        currentStreamingReasoning: null,
        clearToolProgress: true,
        error: null,
        clearRetryPayload: true,
        clearPartialAssistant: true,
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );

    _streamCompleter = Completer<bool>();

    try {
      final stream = regenerateAssistantUseCase(
        sessionId,
        event.assistantMessageId,
      );

      _streamSubscription = subscribeAssistantChatStream(
        stream,
        completer: _streamCompleter!,
        onChunk: (chunk) => handleAssistantChatStreamChunk(
          chunk: chunk,
          emit: emit,
          currentState: () => state,
          isCurrentSession: () => state.currentSessionId == sessionId,
          acc: acc,
          onAssistantMessageId: (id) {
            if (id > 0) {
              _streamingAssistantMessageId = id;
            }
          },
        ),
      );

      final cancelled = await _streamCompleter!.future;

      if (cancelled) {
        return;
      }

      final finRegen = finalizeAssistantStreamAccum(acc);
      if (finRegen != null) {
        final assistantMessage = assistantMessageFromStreamFinal(
          fin: finRegen,
          streamingAssistantMessageId: _streamingAssistantMessageId,
          fallbackMessageId: event.assistantMessageId,
        );

        final regenerated = <int>{
          ...state.regeneratedAssistantMessageIds,
          assistantMessage.id,
        };
        final merged = [...prefixMessages, assistantMessage];
        if (state.currentSessionId == sessionId) {
          emit(
            _copyAfterAssistantStreamSuccess(
              messages: merged,
              regeneratedAssistantMessageIds: regenerated,
            ),
          );
          await _resyncSessionMessagesAfterStream(sessionId, emit);
          if (!isClosed &&
              state.currentSessionId == sessionId &&
              state.messages.isNotEmpty) {
            final last = state.messages.last;
            if (last.role == MessageRole.assistant && last.id > 0) {
              emit(
                state.copyWith(
                  regeneratedAssistantMessageIds: {
                    ...state.regeneratedAssistantMessageIds,
                    last.id,
                  },
                ),
              );
              add(ChatShowAssistantMessageRegenerations(last.id));
            }
          }
        } else {
          emit(
            _copyAfterAssistantStreamSuccess(
              regeneratedAssistantMessageIds: regenerated,
            ),
          );
        }
      } else {
        Logs().w('ChatBloc: пустой ответ при перегенерации');
        emit(
          state.copyWith(
            messages: state.currentSessionId == sessionId
                ? [...prefixMessages, previousAssistant]
                : null,
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            currentStreamingText: null,
            currentStreamingReasoning: null,
            clearToolProgress: true,
            error: kChatEmptyAssistantResponseMessage,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
      }
    } on Object catch (e) {
      Logs().e('ChatBloc: ошибка перегенерации', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          messages: state.currentSessionId == sessionId
              ? [...prefixMessages, previousAssistant]
              : null,
          isLoading: false,
          isStreaming: false,
          clearStreamingSessionId: true,
          clearStreamingParked: true,
          currentStreamingReasoning: null,
          error: chatStreamErrorForState(
            e,
            lead: 'Не удалось перегенерировать ответ',
          ),
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
    } finally {
      await _teardownAssistantStreamPipeline();
    }
  }

  Future<void> _onContinueAssistant(
    ChatContinueAssistant event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }
    if (state.isStreamingInCurrentSession) {
      return;
    }

    final idx = state.messages.indexWhere(
      (m) => m.id == event.assistantMessageId,
    );
    if (idx < 0) {
      return;
    }
    if (idx != state.messages.length - 1) {
      return;
    }
    final target = state.messages[idx];
    if (target.role != MessageRole.assistant || target.id <= 0) {
      return;
    }

    await _abortAssistantStreamPipeline();

    final prefixMessages = state.messages.sublist(0, idx);
    final previousAssistant = target;
    final acc = AssistantStreamAccum();
    seedContinueAccumFromAssistantMessage(acc, previousAssistant);

    emit(
      state.copyWith(
        messages: prefixMessages,
        isLoading: true,
        isStreaming: true,
        streamingSessionId: sessionId,
        clearStreamingParked: true,
        currentStreamingText: acc.rawAssistantText,
        currentStreamingReasoning: acc.nativeReasoning.isEmpty
            ? null
            : acc.nativeReasoning,
        clearToolProgress: true,
        error: null,
        clearRetryPayload: true,
        clearPartialAssistant: true,
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );

    _streamCompleter = Completer<bool>();

    try {
      final stream = continueAssistantUseCase(
        sessionId,
        event.assistantMessageId,
      );

      _streamSubscription = subscribeAssistantChatStream(
        stream,
        completer: _streamCompleter!,
        onChunk: (chunk) => handleAssistantChatStreamChunk(
          chunk: chunk,
          emit: emit,
          currentState: () => state,
          isCurrentSession: () => state.currentSessionId == sessionId,
          acc: acc,
          onAssistantMessageId: (id) {
            if (id > 0) {
              _streamingAssistantMessageId = id;
            }
          },
        ),
      );

      final cancelled = await _streamCompleter!.future;

      if (cancelled) {
        return;
      }

      final finCont = finalizeAssistantStreamAccum(acc);
      if (finCont != null) {
        final assistantMessage = assistantMessageFromStreamFinal(
          fin: finCont,
          streamingAssistantMessageId: _streamingAssistantMessageId,
          fallbackMessageId: event.assistantMessageId,
        );

        final merged = [...prefixMessages, assistantMessage];
        if (state.currentSessionId == sessionId) {
          emit(_copyAfterAssistantStreamSuccess(messages: merged));
          await _resyncSessionMessagesAfterStream(sessionId, emit);
          if (!isClosed &&
              state.currentSessionId == sessionId &&
              state.messages.isNotEmpty) {
            final last = state.messages.last;
            if (last.role == MessageRole.assistant && last.id > 0) {
              add(ChatShowAssistantMessageRegenerations(last.id));
            }
          }
        } else {
          emit(_copyAfterAssistantStreamSuccess());
        }
      } else {
        Logs().w('ChatBloc: пустой ответ при продолжении');
        emit(
          state.copyWith(
            messages: state.currentSessionId == sessionId
                ? [...prefixMessages, previousAssistant]
                : null,
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            currentStreamingText: null,
            currentStreamingReasoning: null,
            clearToolProgress: true,
            error: kChatEmptyAssistantResponseMessage,
            partialAssistantMessageId:
                state.currentSessionId == sessionId && previousAssistant.id > 0
                ? previousAssistant.id
                : null,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
      }
    } on Object catch (e) {
      Logs().e('ChatBloc: ошибка продолжения ответа', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          messages: state.currentSessionId == sessionId
              ? [...prefixMessages, previousAssistant]
              : null,
          isLoading: false,
          isStreaming: false,
          clearStreamingSessionId: true,
          clearStreamingParked: true,
          currentStreamingText: null,
          currentStreamingReasoning: null,
          error: chatStreamErrorForState(
            e,
            lead: 'Не удалось продолжить ответ',
          ),
          partialAssistantMessageId:
              state.currentSessionId == sessionId && previousAssistant.id > 0
              ? previousAssistant.id
              : null,
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
    } finally {
      await _teardownAssistantStreamPipeline();
    }
  }

  Future<void> _onRetryLastMessage(
    ChatRetryLastMessage event,
    Emitter<ChatState> emit,
  ) async {
    final retryText = state.retryText ?? '';
    final hasPayload =
        retryText.trim().isNotEmpty ||
        state.retryAttachmentFileName != null ||
        state.retryAttachmentFileNames.isNotEmpty ||
        state.retryAttachmentContent != null ||
        state.retryAttachmentContents.isNotEmpty ||
        state.retryAttachmentFileId != null ||
        state.retryAttachmentFileIds.isNotEmpty;
    if (!hasPayload) {
      return;
    }
    await _sendMessageInternal(
      ChatSendMessage(
        retryText,
        attachmentFileName: state.retryAttachmentFileName,
        attachmentFileNames: state.retryAttachmentFileNames,
        attachmentContent: state.retryAttachmentContent,
        attachmentContents: state.retryAttachmentContents,
        attachmentFileId: state.retryAttachmentFileId,
        attachmentFileIds: state.retryAttachmentFileIds,
      ),
      emit,
      allowReuseLastUserMessage: true,
    );
  }

  Future<void> _onLoadSessionSettings(
    ChatLoadSessionSettings event,
    Emitter<ChatState> emit,
  ) async {
    try {
      final settings = await getSessionSettingsUseCase(event.sessionId);
      emit(state.copyWith(sessionSettings: settings));
    } catch (_) {}
  }

  Future<void> _onUpdateSessionSettings(
    ChatUpdateSessionSettings event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }
    try {
      final settings = await updateSessionSettingsUseCase(
        sessionId: sessionId,
        systemPrompt: event.systemPrompt,
        stopSequences: event.stopSequences,
        timeoutSeconds: event.timeoutSeconds,
        temperature: event.temperature,
        topK: event.topK,
        topP: event.topP,
        profile: event.profile,
        modelReasoningEnabled: event.modelReasoningEnabled,
        webSearchEnabled: event.webSearchEnabled,
        webSearchProvider: event.webSearchProvider,
        mcpEnabled: event.mcpEnabled,
        mcpServerIds: event.mcpServerIds,
      );
      emit(state.copyWith(sessionSettings: settings));
    } catch (e) {
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: isGrpcUnavailable(e)
              ? null
              : userSafeErrorMessage(
                  e,
                  fallback: 'Ошибка сохранения настроек чата',
                ),
        ),
      );
    }
  }

  Future<void> _onSetModelReasoning(
    ChatSetModelReasoning event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      emit(state.copyWith(draftModelReasoningEnabled: event.enabled));
      return;
    }
    final cur = state.sessionSettings;
    if (cur == null) {
      return;
    }
    try {
      final settings = await updateSessionSettingsUseCase(
        sessionId: sessionId,
        systemPrompt: cur.systemPrompt,
        stopSequences: cur.stopSequences,
        timeoutSeconds: cur.timeoutSeconds,
        temperature: cur.temperature,
        topK: cur.topK,
        topP: cur.topP,
        profile: cur.profile,
        modelReasoningEnabled: event.enabled,
        webSearchEnabled: cur.webSearchEnabled,
        webSearchProvider: cur.webSearchProvider,
        mcpEnabled: cur.mcpEnabled,
        mcpServerIds: cur.mcpServerIds,
      );
      emit(state.copyWith(sessionSettings: settings));
    } catch (e) {
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: isGrpcUnavailable(e)
              ? null
              : userSafeErrorMessage(
                  e,
                  fallback:
                      'Не удалось сохранить настройку «Размышление модели»',
                ),
        ),
      );
    }
  }

  Future<void> _onSetWebSearch(
    ChatSetWebSearch event,
    Emitter<ChatState> emit,
  ) async {
    if (event.enabled && !state.webSearchGloballyEnabled) {
      return;
    }
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      final prov = event.enabled ? event.provider : '';
      emit(
        state.copyWith(
          draftWebSearchEnabled: event.enabled,
          draftWebSearchProvider: prov,
        ),
      );
      return;
    }
    final cur = state.sessionSettings;
    if (cur == null) {
      return;
    }
    try {
      final prov = event.enabled ? event.provider : '';
      final settings = await updateSessionSettingsUseCase(
        sessionId: sessionId,
        systemPrompt: cur.systemPrompt,
        stopSequences: cur.stopSequences,
        timeoutSeconds: cur.timeoutSeconds,
        temperature: cur.temperature,
        topK: cur.topK,
        topP: cur.topP,
        profile: cur.profile,
        modelReasoningEnabled: cur.modelReasoningEnabled,
        webSearchEnabled: event.enabled,
        webSearchProvider: prov,
        mcpEnabled: cur.mcpEnabled,
        mcpServerIds: cur.mcpServerIds,
      );
      emit(state.copyWith(sessionSettings: settings));
    } catch (e) {
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: isGrpcUnavailable(e)
              ? null
              : userSafeErrorMessage(
                  e,
                  fallback:
                      'Не удалось сохранить настройку «Поиск в интернете»',
                ),
        ),
      );
    }
  }

  Future<void> _onSetMcp(ChatSetMcp event, Emitter<ChatState> emit) async {
    final enabled = event.serverIds.isNotEmpty;
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      emit(
        state.copyWith(
          draftMcpEnabled: enabled,
          draftMcpServerIds: event.serverIds,
        ),
      );
      return;
    }
    final cur = state.sessionSettings;
    if (cur == null) {
      return;
    }
    try {
      final settings = await updateSessionSettingsUseCase(
        sessionId: sessionId,
        systemPrompt: cur.systemPrompt,
        stopSequences: cur.stopSequences,
        timeoutSeconds: cur.timeoutSeconds,
        temperature: cur.temperature,
        topK: cur.topK,
        topP: cur.topP,
        profile: cur.profile,
        modelReasoningEnabled: cur.modelReasoningEnabled,
        webSearchEnabled: cur.webSearchEnabled,
        webSearchProvider: cur.webSearchProvider,
        mcpEnabled: enabled,
        mcpServerIds: event.serverIds,
      );
      emit(state.copyWith(sessionSettings: settings));
    } catch (e) {
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          error: isGrpcUnavailable(e)
              ? null
              : userSafeErrorMessage(
                  e,
                  fallback: 'Не удалось сохранить настройки MCP',
                ),
        ),
      );
    }
  }

  Future<void> _onDeleteSession(
    ChatDeleteSession event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      if (state.streamingSessionId == event.sessionId) {
        await _abortAssistantStreamPipeline();
      }

      await deleteSessionUseCase(event.sessionId);

      final updatedSessions = state.sessions
          .where((session) => session.id != event.sessionId)
          .toList();

      final shouldClearCurrent = state.currentSessionId == event.sessionId;
      final killedStream = state.streamingSessionId == event.sessionId;

      emit(
        state.copyWith(
          sessions: updatedSessions,
          currentSessionId: shouldClearCurrent ? null : state.currentSessionId,
          messages: shouldClearCurrent ? const [] : state.messages,
          isLoading: false,
          isStreaming: killedStream ? false : state.isStreaming,
          clearStreamingSessionId: killedStream,
          clearStreamingParked: killedStream,
          currentStreamingText: killedStream
              ? null
              : state.currentStreamingText,
          currentStreamingReasoning: killedStream
              ? null
              : state.currentStreamingReasoning,
          clearToolProgress: killedStream,
          error: null,
        ),
      );
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось удалить чат',
          ),
        ),
      );
    }
  }

  Future<void> _onUpdateSessionTitle(
    ChatUpdateSessionTitle event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      final updatedSession = await updateSessionTitleUseCase(
        event.sessionId,
        event.title,
      );

      final updatedSessions = state.sessions.map((session) {
        if (session.id == event.sessionId) {
          return updatedSession;
        }
        return session;
      }).toList();

      emit(
        state.copyWith(
          sessions: updatedSessions,
          isLoading: false,
          error: null,
        ),
      );
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      _reportServerUnreachableIfNeeded(e);
      emit(
        state.copyWith(
          isLoading: false,
          error: chatHeadlineIfBackendReachable(
            e,
            'Не удалось обновить название чата',
          ),
        ),
      );
    }
  }

  Future<void> _onLoadRunners(
    ChatLoadRunners event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(runnersStatusRefreshing: true));
    try {
      bool? hasActiveRunners = state.hasActiveRunners;
      try {
        hasActiveRunners = await getRunnersStatusUseCase();
      } catch (_) {}

      final webGlob = await getWebSearchAvailabilityUseCase();

      final isAdmin = authBloc.state.user?.isAdmin ?? false;
      if (!isAdmin) {
        try {
          final runnerInfos = await getUserRunnersUseCase();
          _lastRunnerInfos = runnerInfos;
          final runners = extractAvailableRunners(runnerInfos);
          final runnerNames = extractRunnerNames(runnerInfos);
          String? selectedRunner = state.selectedRunner;
          if (runners.isNotEmpty &&
              selectedRunner != null &&
              !runners.contains(selectedRunner)) {
            selectedRunner = runners.first;
            try {
              await setSelectedRunnerUseCase(selectedRunner);
            } catch (_) {}
          }

          final effLoad = selectedRunner ?? state.selectedRunner;
          final loadHealth = runnerHealthForSelection(effLoad, runnerInfos);

          emit(
            state.copyWith(
              runners: runners,
              runnerNames: runnerNames,
              selectedRunner: selectedRunner,
              hasActiveRunners: hasActiveRunners,
              selectedRunnerEnabled: loadHealth.$1,
              selectedRunnerConnected: loadHealth.$2,
              webSearchGloballyEnabled: webGlob,
              draftWebSearchEnabled: webGlob
                  ? state.draftWebSearchEnabled
                  : false,
              draftWebSearchProvider: webGlob
                  ? state.draftWebSearchProvider
                  : '',
              runnersStatusRefreshing: false,
            ),
          );
          return;
        } catch (_) {
          emit(
            state.copyWith(
              hasActiveRunners: hasActiveRunners,
              webSearchGloballyEnabled: webGlob,
              draftWebSearchEnabled: webGlob
                  ? state.draftWebSearchEnabled
                  : false,
              draftWebSearchProvider: webGlob
                  ? state.draftWebSearchProvider
                  : '',
              runnersStatusRefreshing: false,
            ),
          );
          return;
        }
      }

      final runnerInfos = await getRunnersUseCase();
      _lastRunnerInfos = runnerInfos;
      final runners = extractAvailableRunners(runnerInfos);
      final runnerNames = extractRunnerNames(runnerInfos);
      String? selectedRunner = state.selectedRunner;
      if (runners.isNotEmpty && selectedRunner == null) {
        final defaultRunner = await getSelectedRunnerUseCase();
        if (defaultRunner != null && runners.contains(defaultRunner)) {
          selectedRunner = defaultRunner;
        } else {
          selectedRunner = runners.first;
          try {
            await setSelectedRunnerUseCase(selectedRunner);
          } catch (_) {}
        }
      }

      if (runners.isNotEmpty &&
          selectedRunner != null &&
          !runners.contains(selectedRunner)) {
        selectedRunner = runners.first;
        try {
          await setSelectedRunnerUseCase(selectedRunner);
        } catch (_) {}
      }

      final effLoadAdmin = selectedRunner ?? state.selectedRunner;
      final loadHealthAdmin = runnerHealthForSelection(
        effLoadAdmin,
        runnerInfos,
      );

      emit(
        state.copyWith(
          runners: runners,
          runnerNames: runnerNames,
          selectedRunner: selectedRunner ?? state.selectedRunner,
          hasActiveRunners: hasActiveRunners,
          selectedRunnerEnabled: loadHealthAdmin.$1,
          selectedRunnerConnected: loadHealthAdmin.$2,
          webSearchGloballyEnabled: webGlob,
          draftWebSearchEnabled: webGlob ? state.draftWebSearchEnabled : false,
          draftWebSearchProvider: webGlob ? state.draftWebSearchProvider : '',
          runnersStatusRefreshing: false,
        ),
      );
    } catch (_) {
      emit(state.copyWith(runnersStatusRefreshing: false));
    }
  }

  Future<void> _onSelectRunner(
    ChatSelectRunner event,
    Emitter<ChatState> emit,
  ) async {
    try {
      await setSelectedRunnerUseCase(event.runner);
    } catch (_) {}
    final h = runnerHealthForSelection(event.runner, _lastRunnerInfos);
    emit(
      state.copyWith(
        selectedRunner: event.runner,
        selectedRunnerEnabled: h.$1,
        selectedRunnerConnected: h.$2,
      ),
    );
  }

  void _onChatClearError(ChatClearError event, Emitter<ChatState> emit) {
    emit(state.copyWith(error: null));
  }

  void _onDismissStreamNotice(
    ChatDismissStreamNotice event,
    Emitter<ChatState> emit,
  ) {
    emit(state.copyWith(clearStreamNotice: true));
  }

  Map<String, RagDocumentPreview> _ragPreviewAfterClear(ChatState s) {
    final next = Map<String, RagDocumentPreview>.from(
      s.ragPreviewBySessionFile,
    );
    final p = s.ragDocumentPreview;
    final sid = s.streamingSessionId ?? s.currentSessionId;
    if (p != null && sid != null && p.fileId > 0) {
      next['${sid}_${p.fileId}'] = p;
    }
    return next;
  }

  void _onDismissRagDocumentPreview(
    ChatDismissRagDocumentPreview event,
    Emitter<ChatState> emit,
  ) {
    emit(
      state.copyWith(
        ragPreviewBySessionFile: _ragPreviewAfterClear(state),
        clearRagDocumentPreview: true,
      ),
    );
  }

  Future<void> _onChatStopGeneration(
    ChatStopGeneration event,
    Emitter<ChatState> emit,
  ) async {
    final streamSid = state.streamingSessionId;
    final partial = state.currentStreamingText;
    final partialReasoning = state.currentStreamingReasoning;

    await _abortAssistantStreamPipeline(resetStreamingMessageId: false);

    if (partial != null && partial.isNotEmpty && streamSid != null) {
      final aid = _streamingAssistantMessageId > 0
          ? _streamingAssistantMessageId
          : _localTempMessageId();
      final assistantMessage = Message(
        id: aid,
        content: partial,
        role: MessageRole.assistant,
        createdAt: DateTime.now(),
        reasoningContent:
            (partialReasoning != null && partialReasoning.trim().isNotEmpty)
            ? partialReasoning
            : null,
      );

      if (state.currentSessionId == streamSid) {
        emit(
          state.copyWith(
            messages: [...state.messages, assistantMessage],
            isLoading: false,
            isStreaming: false,
            clearStreamingSessionId: true,
            clearStreamingParked: true,
            currentStreamingText: null,
            currentStreamingReasoning: null,
            clearToolProgress: true,
            partialAssistantMessageId: aid > 0 ? aid : null,
            ragPreviewBySessionFile: _ragPreviewAfterClear(state),
            clearRagDocumentPreview: true,
          ),
        );
      } else {
        final parked = state.streamingParkedMessages;
        if (parked != null) {
          emit(
            state.copyWith(
              streamingParkedMessages: [...parked, assistantMessage],
              isLoading: false,
              isStreaming: false,
              clearStreamingSessionId: true,
              currentStreamingText: null,
              currentStreamingReasoning: null,
              clearToolProgress: true,
              ragPreviewBySessionFile: _ragPreviewAfterClear(state),
              clearRagDocumentPreview: true,
            ),
          );
        } else {
          emit(
            state.copyWith(
              isLoading: false,
              isStreaming: false,
              clearStreamingSessionId: true,
              clearStreamingParked: true,
              currentStreamingText: null,
              currentStreamingReasoning: null,
              clearToolProgress: true,
              ragPreviewBySessionFile: _ragPreviewAfterClear(state),
              clearRagDocumentPreview: true,
            ),
          );
        }
      }
    } else {
      emit(
        state.copyWith(
          isLoading: false,
          isStreaming: false,
          clearStreamingSessionId: true,
          clearStreamingParked: true,
          currentStreamingText: null,
          currentStreamingReasoning: null,
          clearToolProgress: true,
          ragPreviewBySessionFile: _ragPreviewAfterClear(state),
          clearRagDocumentPreview: true,
        ),
      );
    }
    _streamingAssistantMessageId = 0;
  }

  @override
  Future<void> close() {
    _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;
    _streamingAssistantMessageId = 0;
    return super.close();
  }
}
