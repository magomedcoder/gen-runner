import 'dart:async';
import 'dart:typed_data';

import 'package:bloc_concurrency/bloc_concurrency.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/request_logout_on_unauthorized.dart';
import 'package:gen/domain/entities/chat_stream_chunk.dart';
import 'package:gen/domain/entities/message.dart';
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
import 'package:gen/domain/usecases/chat/regenerate_assistant_usecase.dart';
import 'package:gen/domain/usecases/chat/edit_user_message_and_continue_usecase.dart';
import 'package:gen/domain/usecases/chat/get_assistant_message_regenerations_usecase.dart';
import 'package:gen/domain/usecases/chat/get_user_message_edits_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_for_assistant_message_version_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_for_user_message_version_usecase.dart';
import 'package:gen/domain/usecases/chat/send_message_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_title_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_status_usecase.dart';
import 'package:gen/domain/usecases/runners/get_user_runners_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

int _localTempMessageId() => -DateTime.now().microsecondsSinceEpoch;

class ChatBloc extends Bloc<ChatEvent, ChatState> {
  final AuthBloc authBloc;
  final ConnectUseCase connectUseCase;
  final GetRunnersUseCase getRunnersUseCase;
  final GetUserRunnersUseCase getUserRunnersUseCase;
  final GetSessionSettingsUseCase getSessionSettingsUseCase;
  final UpdateSessionSettingsUseCase updateSessionSettingsUseCase;
  final SendMessageUseCase sendMessageUseCase;
  final RegenerateAssistantUseCase regenerateAssistantUseCase;
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

  StreamSubscription<ChatStreamChunk>? _streamSubscription;
  Completer<bool>? _streamCompleter;

  int _streamingAssistantMessageId = 0;

  Future<List<Message>?> _resetViewToLatestBranchIfNeeded(
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return null;
    }

    int? userMessageId;
    List<UserMessageEdit>? edits;
    for (var i = state.messages.length - 1; i >= 0; i--) {
      final m = state.messages[i];
      if (m.role != MessageRole.user || m.id <= 0) {
        continue;
      }
      final e = state.editsByMessageId[m.id];
      if (e != null && e.isNotEmpty) {
        userMessageId = m.id;
        edits = e;
        break;
      }
    }
    if (userMessageId == null || edits == null) {
      return null;
    }

    final latestIdx = edits.length;
    final currentIdx = state.editCursorByMessageId[userMessageId] ?? latestIdx;
    if (currentIdx == latestIdx) {
      return null;
    }

    final cursorById = Map<int, int>.from(state.editCursorByMessageId);
    cursorById[userMessageId] = latestIdx;
    emit(state.copyWith(editCursorByMessageId: cursorById));
    if (emit.isDone) {
      return null;
    }

    final view = await getSessionMessagesForUserMessageVersionUseCase(
      sessionId: sessionId,
      userMessageId: userMessageId,
      versionIndex: latestIdx,
    );

    if (emit.isDone) {
      return null;
    }

    emit(state.copyWith(messages: view));

    return view;
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

    final take = candidates.length > 20 ? candidates.sublist(candidates.length - 20) : candidates;
    final editsById = Map<int, List<UserMessageEdit>>.from(state.editsByMessageId);
    final cursorById = Map<int, int>.from(state.editCursorByMessageId);
    final editedIds = <int>{...state.editedMessageIds};

    for (final m in take) {
      final existing = editsById[m.id];
      if (existing != null) {
        cursorById[m.id] = existing.isEmpty ? 0 : existing.length;
        editedIds.add(m.id);
        continue;
      }

      try {
        final editsRaw = await getUserMessageEditsUseCase(
          sessionId: sessionId,
          userMessageId: m.id,
        );

        final edits = [...editsRaw]..sort((a, b) => a.createdAt.compareTo(b.createdAt));
        editsById[m.id] = edits;
        cursorById[m.id] = edits.isEmpty ? 0 : edits.length;
        editedIds.add(m.id);
      } catch (_) {

      }
      if (emit.isDone) {
        return;
      }
    }

    emit(state.copyWith(
      editsByMessageId: editsById,
      editCursorByMessageId: cursorById,
      editedMessageIds: editedIds,
    ));
  }

  ChatBloc({
    required this.authBloc,
    required this.connectUseCase,
    required this.getRunnersUseCase,
    required this.getUserRunnersUseCase,
    required this.getSessionSettingsUseCase,
    required this.updateSessionSettingsUseCase,
    required this.sendMessageUseCase,
    required this.regenerateAssistantUseCase,
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
  }) : super(const ChatState()) {
    on<ChatStarted>(_onChatStarted);
    on<ChatCreateSession>(_onCreateSession);
    on<ChatLoadSessions>(_onLoadSessions);
    on<ChatSelectSession>(_onSelectSession);
    on<ChatLoadSessionMessages>(_onLoadSessionMessages);
    on<ChatSendMessage>(_onChatSendMessage, transformer: droppable());
    on<ChatClearError>(_onChatClearError);
    on<ChatStopGeneration>(_onChatStopGeneration);
    on<ChatRetryLastMessage>(_onRetryLastMessage);
    on<ChatRegenerateAssistant>(_onRegenerateAssistant, transformer: droppable());
    on<ChatEditUserMessageAndContinue>(_onEditUserMessageAndContinue, transformer: droppable());
    on<ChatShowUserMessageEdits>(_onShowUserMessageEdits, transformer: droppable());
    on<ChatNavigateUserMessageEdit>(_onNavigateUserMessageEdit, transformer: droppable());
    on<ChatShowAssistantMessageRegenerations>(_onShowAssistantMessageRegenerations, transformer: droppable());
    on<ChatNavigateAssistantMessageRegeneration>(_onNavigateAssistantMessageRegeneration, transformer: droppable());
    on<ChatDeleteSession>(_onDeleteSession);
    on<ChatUpdateSessionTitle>(_onUpdateSessionTitle);
    on<ChatLoadRunners>(_onLoadRunners);
    on<ChatSelectRunner>(_onSelectRunner);
    on<ChatLoadSessionSettings>(_onLoadSessionSettings);
    on<ChatUpdateSessionSettings>(_onUpdateSessionSettings);
  }

  Future<void> _onShowUserMessageEdits(
    ChatShowUserMessageEdits event,
    Emitter<ChatState> emit,
  ) async {
    final sessionId = state.currentSessionId;
    if (sessionId == null) {
      return;
    }

    if (state.isStreaming) {
      return;
    }

    try {
      final editsRaw = await getUserMessageEditsUseCase(
        sessionId: sessionId,
        userMessageId: event.userMessageId,
      );
      final edits = [...editsRaw]..sort((a, b) => a.createdAt.compareTo(b.createdAt));

      final editsById = Map<int, List<UserMessageEdit>>.from(state.editsByMessageId);
      editsById[event.userMessageId] = edits;

      final cursorById = Map<int, int>.from(state.editCursorByMessageId);
      cursorById[event.userMessageId] = edits.isEmpty ? 0 : edits.length;

      final pending = state.pendingEditNavDeltaByMessageId[event.userMessageId];
      final pendingMap = Map<int, int>.from(state.pendingEditNavDeltaByMessageId);
      pendingMap.remove(event.userMessageId);

      if (pending != null && edits.isNotEmpty) {
        final versionsCount = edits.length + 1;
        final cur = cursorById[event.userMessageId] ?? (versionsCount - 1);
        cursorById[event.userMessageId] = (cur + pending).clamp(0, versionsCount - 1);
      }

      emit(state.copyWith(
        editsByMessageId: editsById,
        editCursorByMessageId: cursorById,
        pendingEditNavDeltaByMessageId: pendingMap,
      ));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(error: 'Не удалось загрузить историю правок'));
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
    final current = state.editCursorByMessageId[event.userMessageId] ?? (versionsCount - 1);
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
        if (emit.isDone) {
          return;
        }

        emit(state.copyWith(error: 'Не удалось загрузить ветку версии'));
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
    if (state.isStreaming) {
      return;
    }
    try {
      final rowsRaw = await getAssistantMessageRegenerationsUseCase(
        sessionId: sessionId,
        assistantMessageId: event.assistantMessageId,
      );
      final rows = [...rowsRaw]..sort((a, b) => a.createdAt.compareTo(b.createdAt));

      final byId = Map<int, List<AssistantMessageRegeneration>>.from(state.regenerationsByMessageId);
      byId[event.assistantMessageId] = rows;

      final cursorById = Map<int, int>.from(state.regenerationCursorByMessageId);
      cursorById[event.assistantMessageId] = rows.isEmpty ? 0 : rows.length;

      final pending = state.pendingRegenerationNavDeltaByMessageId[event.assistantMessageId];
      final pendingMap = Map<int, int>.from(state.pendingRegenerationNavDeltaByMessageId);
      pendingMap.remove(event.assistantMessageId);

      if (pending != null && rows.isNotEmpty) {
        final versionsCount = rows.length + 1;
        final cur = cursorById[event.assistantMessageId] ?? (versionsCount - 1);
        cursorById[event.assistantMessageId] = (cur + pending).clamp(0, versionsCount - 1);
      }

      final regeneratedIds = <int>{...state.regeneratedAssistantMessageIds, event.assistantMessageId};

      emit(state.copyWith(
        regenerationsByMessageId: byId,
        regenerationCursorByMessageId: cursorById,
        pendingRegenerationNavDeltaByMessageId: pendingMap,
        regeneratedAssistantMessageIds: regeneratedIds,
      ));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(error: 'Не удалось загрузить историю перегенераций'));
    }
  }

  Future<void> _onNavigateAssistantMessageRegeneration(
    ChatNavigateAssistantMessageRegeneration event,
    Emitter<ChatState> emit,
  ) async {
    final regens = state.regenerationsByMessageId[event.assistantMessageId];
    if (regens == null) {
      final pending = Map<int, int>.from(state.pendingRegenerationNavDeltaByMessageId);
      pending[event.assistantMessageId] = event.delta;
      emit(state.copyWith(pendingRegenerationNavDeltaByMessageId: pending));
      add(ChatShowAssistantMessageRegenerations(event.assistantMessageId));
      return;
    }

    if (regens.isEmpty) {
      return;
    }

    final versionsCount = regens.length + 1;
    final current = state.regenerationCursorByMessageId[event.assistantMessageId] ?? (versionsCount - 1);
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
        if (emit.isDone) {
          return;
        }
        emit(state.copyWith(error: 'Не удалось загрузить версию ответа'));
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

    if (state.isStreaming) {
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

    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }

    _streamSubscription = null;
    _streamCompleter = null;
    _streamingAssistantMessageId = 0;

    final updatedUser = Message(
      id: target.id,
      content: newText,
      role: MessageRole.user,
      createdAt: target.createdAt,
      updatedAt: DateTime.now(),
      attachmentFileName: target.attachmentFileName,
      attachmentContent: target.attachmentContent,
      attachmentFileId: target.attachmentFileId,
    );
    final prefixMessages = [
      ...state.messages.sublist(0, idx),
      updatedUser,
    ];

    var streamingText = '';

    final edited = <int>{...state.editedMessageIds, target.id};
    emit(state.copyWith(
      messages: prefixMessages,
      editedMessageIds: edited,
      isLoading: true,
      isStreaming: true,
      currentStreamingText: '',
      clearToolProgress: true,
      error: null,
      clearRetryPayload: true,
    ));

    _streamCompleter = Completer<bool>();

    try {
      final stream = editUserMessageAndContinueUseCase(
        sessionId,
        event.userMessageId,
        newText,
      );

      _streamSubscription = stream.listen(
        (chunk) {
          if (chunk.kind == ChatStreamChunkKind.toolStatus) {
            final line = chunk.text.trim().isNotEmpty ? chunk.text : (chunk.toolName ?? 'инструмент');
            emit(state.copyWith(toolProgressLabel: line));
            return;
          }

          if (chunk.messageId > 0) {
            _streamingAssistantMessageId = chunk.messageId;
          }

          streamingText += chunk.text;
          emit(state.copyWith(
            currentStreamingText: streamingText,
            clearToolProgress: true,
          ));
        },
        onDone: () {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.complete(false);
          }
        },
        onError: (e, st) {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.completeError(e, st);
          }
        },
        cancelOnError: false,
      );

      final cancelled = await _streamCompleter!.future;
      if (cancelled) {
        return;
      }

      if (streamingText.isNotEmpty) {
        final aid = _streamingAssistantMessageId > 0
            ? _streamingAssistantMessageId
            : _localTempMessageId();
        final assistantMessage = Message(
          id: aid,
          content: streamingText,
          role: MessageRole.assistant,
          createdAt: DateTime.now(),
        );

        emit(state.copyWith(
          messages: [...prefixMessages, assistantMessage],
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          clearRetryPayload: true,
        ));

        add(ChatShowUserMessageEdits(event.userMessageId));
      } else {
        emit(state.copyWith(
          messages: prefixMessages,
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          error: 'Сервер не вернул ответ. Проверьте доступность раннера и попробуйте снова.',
        ));
      }
    } on Object catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(
        messages: prefixMessages,
        isLoading: false,
        isStreaming: false,
        error: 'Ошибка редактирования сообщения',
      ));
    } finally {
      await _streamSubscription?.cancel();
      _streamSubscription = null;
      _streamCompleter = null;
      _streamingAssistantMessageId = 0;
    }
  }

  List<String> _extractAvailableRunners(List<RunnerInfo> runners) {
    final addresses = <String>{
      for (final runner in runners)
        if (runner.enabled && runner.address.isNotEmpty)
          runner.address,
    };
    final sorted = addresses.toList()..sort();

    return sorted;
  }

  Map<String, String> _extractRunnerNames(List<RunnerInfo> runners) {
    final names = <String, String>{};
    for (final runner in runners) {
      if (!runner.enabled || runner.address.isEmpty) {
        continue;
      }

      final name = runner.name.trim();
      names[runner.address] = name.isNotEmpty ? name : runner.address;
    }

    return names;
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

          List<String> runners = const [];
          Map<String, String> runnerNames = const {};
          String? selectedRunner;
          try {
            if (isAdmin) {
              final runnerInfos = await getRunnersUseCase();
              runners = _extractAvailableRunners(runnerInfos);
              runnerNames = _extractRunnerNames(runnerInfos);
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
                final runnerInfos = await getUserRunnersUseCase();
                runners = _extractAvailableRunners(runnerInfos);
                runnerNames = _extractRunnerNames(runnerInfos);

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

          int? currentSessionId;
          List<Message> messages = const [];

          if (sessions.isNotEmpty) {
            currentSessionId = sessions.first.id;

            final sessionMessages = await getSessionMessagesUseCase(
              currentSessionId,
              page: 1,
              pageSize: 50,
            );
            messages = sessionMessages;
            await _prefetchEditsForMessages(currentSessionId, messages, emit);
            try {
              final s = await getSessionSettingsUseCase(currentSessionId);
              emit(state.copyWith(sessionSettings: s));
            } catch (_) {}

            if (selectedRunner == null && runners.isNotEmpty) {
              selectedRunner = runners.first;
            }
          }

          Logs().i('ChatBloc: чат загружен, сессий: ${sessions.length}');
          emit(state.copyWith(
              isConnected: isConnected,
              isLoading: false,
              sessions: sessions,
              currentSessionId: currentSessionId,
              messages: messages,
              runners: runners,
              runnerNames: runnerNames,
              selectedRunner: selectedRunner ?? state.selectedRunner,
              hasActiveRunners: hasActiveRunners,
              error: null,
            ));
        } catch (e) {
          Logs().e('ChatBloc: ошибка загрузки сессий', exception: e);
          requestLogoutIfUnauthorized(e, authBloc);
          emit(state.copyWith(
            isConnected: isConnected,
            isLoading: false,
            hasActiveRunners: hasActiveRunners,
            error: 'Ошибка загрузки сессий',
          ));
        }
      } else {
        Logs().w('ChatBloc: не удалось подключиться к серверу');
        emit(
          state.copyWith(
            isConnected: isConnected,
            isLoading: false,
            error: isConnected ? null : 'Не удалось подключиться к серверу',
          ),
        );
      }
    } catch (e) {
      Logs().e('ChatBloc: ошибка подключения', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(
        isConnected: false,
        isLoading: false,
        error: 'Ошибка подключения',
      ));
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

    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;

    emit(state.copyWith(
      currentSessionId: null,
      messages: const [],
      error: null,
      currentStreamingText: null,
      isLoading: false,
      isStreaming: false,
    ));
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
      emit(state.copyWith(isLoading: false, error: 'Ошибка загрузки сессий'));
    }
  }

  Future<void> _onSelectSession(
    ChatSelectSession event,
    Emitter<ChatState> emit,
  ) async {
    if (state.currentSessionId == event.sessionId) {
      return;
    }

    emit(state.copyWith(
      currentSessionId: event.sessionId,
      messages: const [],
      isLoading: true,
      error: null,
    ));

    try {
      final messages = await getSessionMessagesUseCase(
        event.sessionId,
        page: 1,
        pageSize: 50,
      );

      String? runnerForSession = state.selectedRunner;
      if (state.runners.isNotEmpty) {
        if (runnerForSession == null ||
            !state.runners.contains(runnerForSession)) {
          runnerForSession = state.runners.first;
        }
      }

      emit(state.copyWith(
        messages: messages,
        isLoading: false,
        selectedRunner: runnerForSession,
      ));
      await _prefetchEditsForMessages(event.sessionId, messages, emit);
      add(ChatLoadSessionSettings(event.sessionId));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(isLoading: false, error: 'Ошибка загрузки сообщений'));
    }
  }

  Future<void> _onLoadSessionMessages(
    ChatLoadSessionMessages event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      final messages = await getSessionMessagesUseCase(
        event.sessionId,
        page: event.page,
        pageSize: event.pageSize,
      );

      final allMessages = [...state.messages, ...messages];

      emit(state.copyWith(messages: allMessages, isLoading: false, error: null));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(isLoading: false, error: 'Ошибка загрузки сообщений'),);
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
    final hasAttachmentBytes = event.attachmentFileName != null &&
        event.attachmentContent != null &&
        event.attachmentContent!.isNotEmpty;
    final hasAttachmentById =
        event.attachmentFileId != null && event.attachmentFileId! > 0;
    if (text.isEmpty && !hasAttachmentBytes && !hasAttachmentById) {
      return;
    }

    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;
    _streamingAssistantMessageId = 0;

    var sessionId = state.currentSessionId;
    if (sessionId == null) {
      try {
        final session = await createSessionUseCase();
        sessionId = session.id;

        final updatedSessions = [session, ...state.sessions];

        emit(state.copyWith(
          currentSessionId: sessionId,
          sessions: updatedSessions,
          messages: const [],
        ));
        add(ChatLoadSessionSettings(sessionId));
      } catch (e) {
        requestLogoutIfUnauthorized(e, authBloc);
        emit(state.copyWith(error: 'Ошибка создания сессии', isLoading: false));
        return;
      }
    }

    final byServerFileId = hasAttachmentById;
    final userMessage = Message(
      id: _localTempMessageId(),
      content: text,
      role: MessageRole.user,
      createdAt: DateTime.now(),
      attachmentFileName: event.attachmentFileName,
      attachmentContent: byServerFileId
          ? null
          : (event.attachmentContent != null
              ? Uint8List.fromList(event.attachmentContent!)
              : null),
      attachmentFileId: byServerFileId ? event.attachmentFileId : null,
    );

    var updatedMessages = [...state.messages, userMessage];
    if (allowReuseLastUserMessage && state.messages.isNotEmpty) {
      final last = state.messages.last;
      final sameUserMessage =
          last.role == MessageRole.user &&
          last.content == text &&
          last.attachmentFileName == event.attachmentFileName &&
          last.attachmentFileId == event.attachmentFileId &&
          _isSameAttachment(event.attachmentContent, last.attachmentContent);
      if (sameUserMessage) {
        updatedMessages = [...state.messages];
      }
    }
    String streamingText = '';

    emit(state.copyWith(
      messages: updatedMessages,
      isLoading: true,
      isStreaming: true,
      currentStreamingText: '',
      clearToolProgress: true,
      error: null,
      clearRetryPayload: true,
    ));

    _streamCompleter = Completer<bool>();

    try {
      final stream = sendMessageUseCase(
        sessionId,
        updatedMessages,
      );

      _streamSubscription = stream.listen(
        (chunk) {
          if (chunk.kind == ChatStreamChunkKind.toolStatus) {
            final line = chunk.text.trim().isNotEmpty
                ? chunk.text
                : (chunk.toolName ?? 'инструмент');
            emit(state.copyWith(toolProgressLabel: line));
            return;
          }
          if (chunk.messageId > 0) {
            _streamingAssistantMessageId = chunk.messageId;
          }
          streamingText += chunk.text;
          emit(state.copyWith(
            currentStreamingText: streamingText,
            clearToolProgress: true,
          ));
        },
        onDone: () {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.complete(false);
          }
        },
        onError: (e, st) {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.completeError(e, st);
          }
        },
        cancelOnError: false,
      );

      final cancelled = await _streamCompleter!.future;

      if (cancelled) {
        return;
      }

      if (streamingText.isNotEmpty) {
        final aid = _streamingAssistantMessageId > 0
            ? _streamingAssistantMessageId
            : _localTempMessageId();
        final assistantMessage = Message(
          id: aid,
          content: streamingText,
          role: MessageRole.assistant,
          createdAt: DateTime.now(),
        );

        final allMessages = [...updatedMessages, assistantMessage];

        emit(state.copyWith(
          messages: allMessages,
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          clearRetryPayload: true,
        ));
      } else {
        Logs().w('ChatBloc: пустой ответ от сервера при отправке сообщения');
        emit(state.copyWith(
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          error: 'Сервер не вернул ответ. Проверьте доступность раннера и попробуйте снова.',
          retryText: event.text,
          retryAttachmentFileName: event.attachmentFileName,
          retryAttachmentContent: event.attachmentContent,
          retryAttachmentFileId: event.attachmentFileId,
        ));
      }
    } on Object catch (e) {
      Logs().e('ChatBloc: ошибка отправки сообщения', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(
        isLoading: false,
        isStreaming: false,
        error: 'Ошибка отправки сообщения',
        retryText: event.text,
        retryAttachmentFileName: event.attachmentFileName,
        retryAttachmentContent: event.attachmentContent,
        retryAttachmentFileId: event.attachmentFileId,
      ));
    } finally {
      await _streamSubscription?.cancel();
      _streamSubscription = null;
      _streamCompleter = null;
      _streamingAssistantMessageId = 0;
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
    if (state.isStreaming) {
      return;
    }

    try {
      await _resetViewToLatestBranchIfNeeded(emit);
    } catch (_) {

    }
    if (emit.isDone) {
      return;
    }

    final idx = state.messages.indexWhere((m) => m.id == event.assistantMessageId);
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

    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;
    _streamingAssistantMessageId = 0;

    final prefixMessages = state.messages.sublist(0, idx);
    final previousAssistant = target;
    var streamingText = '';

    emit(state.copyWith(
      messages: prefixMessages,
      isLoading: true,
      isStreaming: true,
      currentStreamingText: '',
      clearToolProgress: true,
      error: null,
      clearRetryPayload: true,
    ));

    _streamCompleter = Completer<bool>();

    try {
      final stream = regenerateAssistantUseCase(sessionId, event.assistantMessageId);

      _streamSubscription = stream.listen(
        (chunk) {
          if (chunk.kind == ChatStreamChunkKind.toolStatus) {
            final line = chunk.text.trim().isNotEmpty
                ? chunk.text
                : (chunk.toolName ?? 'инструмент');
            emit(state.copyWith(toolProgressLabel: line));
            return;
          }
          if (chunk.messageId > 0) {
            _streamingAssistantMessageId = chunk.messageId;
          }
          streamingText += chunk.text;
          emit(state.copyWith(
            currentStreamingText: streamingText,
            clearToolProgress: true,
          ));
        },
        onDone: () {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.complete(false);
          }
        },
        onError: (e, st) {
          if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
            _streamCompleter!.completeError(e, st);
          }
        },
        cancelOnError: false,
      );

      final cancelled = await _streamCompleter!.future;

      if (cancelled) {
        return;
      }

      if (streamingText.isNotEmpty) {
        final aid = _streamingAssistantMessageId > 0
            ? _streamingAssistantMessageId
            : event.assistantMessageId;
        final assistantMessage = Message(
          id: aid,
          content: streamingText,
          role: MessageRole.assistant,
          createdAt: DateTime.now(),
        );

        final regenerated = <int>{...state.regeneratedAssistantMessageIds, aid};
        emit(state.copyWith(
          messages: [...prefixMessages, assistantMessage],
          regeneratedAssistantMessageIds: regenerated,
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          clearRetryPayload: true,
        ));

        add(ChatShowAssistantMessageRegenerations(aid));
      } else {
        Logs().w('ChatBloc: пустой ответ при перегенерации');
        emit(state.copyWith(
          messages: [...prefixMessages, previousAssistant],
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          clearToolProgress: true,
          error:
              'Сервер не вернул ответ. Проверьте доступность раннера и попробуйте снова.',
        ));
      }
    } on Object catch (e) {
      Logs().e('ChatBloc: ошибка перегенерации', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(
        messages: [...prefixMessages, previousAssistant],
        isLoading: false,
        isStreaming: false,
        error: 'Ошибка перегенерации ответа',
      ));
    } finally {
      await _streamSubscription?.cancel();
      _streamSubscription = null;
      _streamCompleter = null;
      _streamingAssistantMessageId = 0;
    }
  }

  Future<void> _onRetryLastMessage(
    ChatRetryLastMessage event,
    Emitter<ChatState> emit,
  ) async {
    final retryText = state.retryText ?? '';
    final hasPayload = retryText.trim().isNotEmpty ||
        state.retryAttachmentFileName != null ||
        state.retryAttachmentContent != null ||
        state.retryAttachmentFileId != null;
    if (!hasPayload) {
      return;
    }
    await _sendMessageInternal(
      ChatSendMessage(
        retryText,
        attachmentFileName: state.retryAttachmentFileName,
        attachmentContent: state.retryAttachmentContent,
        attachmentFileId: state.retryAttachmentFileId,
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
        jsonMode: event.jsonMode,
        jsonSchema: event.jsonSchema,
        toolsJson: event.toolsJson,
        profile: event.profile,
      );
      emit(state.copyWith(sessionSettings: settings));
    } catch (e) {
      emit(state.copyWith(error: 'Ошибка сохранения настроек чата'));
    }
  }

  Future<void> _onDeleteSession(
    ChatDeleteSession event,
    Emitter<ChatState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));

    try {
      await deleteSessionUseCase(event.sessionId);

      final updatedSessions = state.sessions
          .where((session) => session.id != event.sessionId)
          .toList();

      final shouldClearCurrent = state.currentSessionId == event.sessionId;

      emit(state.copyWith(
        sessions: updatedSessions,
        currentSessionId: shouldClearCurrent ? null : state.currentSessionId,
        messages: shouldClearCurrent ? const [] : state.messages,
        isLoading: false,
        error: null,
      ));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(isLoading: false, error: 'Ошибка удаления сессии'));
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

      emit(state.copyWith(
        sessions: updatedSessions,
        isLoading: false,
        error: null,
      ));
    } catch (e) {
      requestLogoutIfUnauthorized(e, authBloc);
      emit(state.copyWith(isLoading: false, error: 'Ошибка обновления заголовка'));
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

      final isAdmin = authBloc.state.user?.isAdmin ?? false;
      if (!isAdmin) {
        try {
          final runnerInfos = await getUserRunnersUseCase();
          final runners = _extractAvailableRunners(runnerInfos);
          final runnerNames = _extractRunnerNames(runnerInfos);
          String? selectedRunner = state.selectedRunner;
          if (runners.isNotEmpty && selectedRunner != null && !runners.contains(selectedRunner)) {
            selectedRunner = runners.first;
            try {
              await setSelectedRunnerUseCase(selectedRunner);
            } catch (_) {}
          }

          emit(state.copyWith(
            runners: runners,
            runnerNames: runnerNames,
            selectedRunner: selectedRunner,
            hasActiveRunners: hasActiveRunners,
            runnersStatusRefreshing: false,
          ));
          return;
        } catch (_) {
          emit(state.copyWith(
            hasActiveRunners: hasActiveRunners,
            runnersStatusRefreshing: false,
          ));
          return;
        }
      }

      final runnerInfos = await getRunnersUseCase();
      final runners = _extractAvailableRunners(runnerInfos);
      final runnerNames = _extractRunnerNames(runnerInfos);
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

      if (runners.isNotEmpty && selectedRunner != null && !runners.contains(selectedRunner)) {
        selectedRunner = runners.first;
        try {
          await setSelectedRunnerUseCase(selectedRunner);
        } catch (_) {}
      }

      emit(state.copyWith(
        runners: runners,
        runnerNames: runnerNames,
        selectedRunner: selectedRunner ?? state.selectedRunner,
        hasActiveRunners: hasActiveRunners,
        runnersStatusRefreshing: false,
      ));
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
    emit(
      state.copyWith(selectedRunner: event.runner),
    );
  }

  void _onChatClearError(ChatClearError event, Emitter<ChatState> emit) {
    emit(state.copyWith(error: null));
  }

  Future<void> _onChatStopGeneration(
    ChatStopGeneration event,
    Emitter<ChatState> emit,
  ) async {
    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;

    if (state.currentStreamingText != null &&
        state.currentStreamingText!.isNotEmpty) {
      final aid = _streamingAssistantMessageId > 0
          ? _streamingAssistantMessageId
          : _localTempMessageId();
      final assistantMessage = Message(
        id: aid,
        content: state.currentStreamingText!,
        role: MessageRole.assistant,
        createdAt: DateTime.now(),
      );

      final allMessages = [...state.messages, assistantMessage];

      emit(state.copyWith(
        messages: allMessages,
        isLoading: false,
        isStreaming: false,
        currentStreamingText: null,
      ));
    } else {
      emit(state.copyWith(
        isLoading: false,
        isStreaming: false,
        currentStreamingText: null,
      ));
    }
    _streamingAssistantMessageId = 0;
  }

  @override
  Future<void> close() {
    _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }

    return super.close();
  }
}
