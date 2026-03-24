import 'dart:async';
import 'dart:typed_data';

import 'package:bloc_concurrency/bloc_concurrency.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/request_logout_on_unauthorized.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/usecases/chat/connect_usecase.dart';
import 'package:gen/domain/usecases/chat/create_session_usecase.dart';
import 'package:gen/domain/usecases/chat/delete_session_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_usecase.dart';
import 'package:gen/domain/usecases/chat/get_sessions_usecase.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/send_message_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_title_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_status_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

int _localTempMessageId() => -DateTime.now().microsecondsSinceEpoch;

class ChatBloc extends Bloc<ChatEvent, ChatState> {
  final AuthBloc authBloc;
  final ConnectUseCase connectUseCase;
  final GetRunnersUseCase getRunnersUseCase;
  final GetSessionSettingsUseCase getSessionSettingsUseCase;
  final UpdateSessionSettingsUseCase updateSessionSettingsUseCase;
  final SendMessageUseCase sendMessageUseCase;
  final CreateSessionUseCase createSessionUseCase;
  final GetSessionsUseCase getSessionsUseCase;
  final GetSessionMessagesUseCase getSessionMessagesUseCase;
  final DeleteSessionUseCase deleteSessionUseCase;
  final UpdateSessionTitleUseCase updateSessionTitleUseCase;
  final GetRunnersStatusUseCase getRunnersStatusUseCase;
  final GetSelectedRunnerUseCase getSelectedRunnerUseCase;
  final SetSelectedRunnerUseCase setSelectedRunnerUseCase;

  StreamSubscription<String>? _streamSubscription;
  Completer<bool>? _streamCompleter;

  ChatBloc({
    required this.authBloc,
    required this.connectUseCase,
    required this.getRunnersUseCase,
    required this.getSessionSettingsUseCase,
    required this.updateSessionSettingsUseCase,
    required this.sendMessageUseCase,
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
    on<ChatDeleteSession>(_onDeleteSession);
    on<ChatUpdateSessionTitle>(_onUpdateSessionTitle);
    on<ChatLoadRunners>(_onLoadRunners);
    on<ChatSelectRunner>(_onSelectRunner);
    on<ChatLoadSessionSettings>(_onLoadSessionSettings);
    on<ChatUpdateSessionSettings>(_onUpdateSessionSettings);
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
          final runnersFuture = getRunnersUseCase();

          final sessions = await sessionsFuture;
          List<String> runners = const [];
          Map<String, String> runnerNames = const {};
          String? selectedRunner;
          try {
            final runnerInfos = await runnersFuture;
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
    final hasAttachment = event.attachmentFileName != null && event.attachmentContent != null && event.attachmentContent!.isNotEmpty;
    if (text.isEmpty && !hasAttachment) {
      return;
    }

    await _streamSubscription?.cancel();
    if (_streamCompleter != null && !_streamCompleter!.isCompleted) {
      _streamCompleter!.complete(true);
    }
    _streamSubscription = null;
    _streamCompleter = null;

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

    final userMessage = Message(
      id: _localTempMessageId(),
      content: text,
      role: MessageRole.user,
      createdAt: DateTime.now(),
      attachmentFileName: event.attachmentFileName,
      attachmentContent: event.attachmentContent != null
        ? Uint8List.fromList(event.attachmentContent!)
        : null,
    );

    var updatedMessages = [...state.messages, userMessage];
    if (allowReuseLastUserMessage && state.messages.isNotEmpty) {
      final last = state.messages.last;
      final sameUserMessage =
          last.role == MessageRole.user &&
          last.content == text &&
          last.attachmentFileName == event.attachmentFileName &&
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
          streamingText += chunk;
          emit(state.copyWith(currentStreamingText: streamingText));
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
        final assistantMessage = Message(
          id: _localTempMessageId(),
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
          clearRetryPayload: true,
        ));
      } else {
        Logs().w('ChatBloc: пустой ответ от сервера при отправке сообщения');
        emit(state.copyWith(
          isLoading: false,
          isStreaming: false,
          currentStreamingText: null,
          error: 'Сервер не вернул ответ. Проверьте доступность раннера и попробуйте снова.',
          retryText: event.text,
          retryAttachmentFileName: event.attachmentFileName,
          retryAttachmentContent: event.attachmentContent,
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
      ));
    } finally {
      await _streamSubscription?.cancel();
      _streamSubscription = null;
      _streamCompleter = null;
    }
  }

  Future<void> _onRetryLastMessage(
    ChatRetryLastMessage event,
    Emitter<ChatState> emit,
  ) async {
    final retryText = state.retryText;
    if (retryText == null || retryText.trim().isEmpty) {
      return;
    }
    await _sendMessageInternal(
      ChatSendMessage(
        retryText,
        attachmentFileName: state.retryAttachmentFileName,
        attachmentContent: state.retryAttachmentContent,
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
        maxTokens: event.maxTokens,
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
    try {
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
      ));
    } catch (_) {}
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
      final assistantMessage = Message(
        id: _localTempMessageId(),
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
