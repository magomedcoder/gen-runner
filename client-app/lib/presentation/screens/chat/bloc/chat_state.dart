import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';

const _kKeepCurrentSessionId = Symbol('_kKeepCurrentSessionId');

class ChatState extends Equatable {
  final bool isConnected;
  final bool isLoading;
  final bool isStreaming;
  final int? currentSessionId;
  final List<ChatSession> sessions;
  final List<Message> messages;
  final String? currentStreamingText;
  final String? error;
  final List<String> runners;
  final Map<String, String> runnerNames;
  final String? selectedRunner;
  final bool? hasActiveRunners;
  final ChatSessionSettings? sessionSettings;
  final String? retryText;
  final String? retryAttachmentFileName;
  final List<int>? retryAttachmentContent;

  const ChatState({
    this.isConnected = false,
    this.isLoading = false,
    this.isStreaming = false,
    this.currentSessionId,
    this.sessions = const [],
    this.messages = const [],
    this.currentStreamingText,
    this.error,
    this.runners = const [],
    this.runnerNames = const {},
    this.selectedRunner,
    this.hasActiveRunners,
    this.sessionSettings,
    this.retryText,
    this.retryAttachmentFileName,
    this.retryAttachmentContent,
  });

  ChatState copyWith({
    bool? isConnected,
    bool? isLoading,
    bool? isStreaming,
    Object? currentSessionId = _kKeepCurrentSessionId,
    List<ChatSession>? sessions,
    List<Message>? messages,
    String? currentStreamingText,
    String? error,
    List<String>? runners,
    Map<String, String>? runnerNames,
    String? selectedRunner,
    bool? hasActiveRunners,
    ChatSessionSettings? sessionSettings,
    String? retryText,
    String? retryAttachmentFileName,
    List<int>? retryAttachmentContent,
    bool clearRetryPayload = false,
  }) {
    return ChatState(
      isConnected: isConnected ?? this.isConnected,
      isLoading: isLoading ?? this.isLoading,
      isStreaming: isStreaming ?? this.isStreaming,
      currentSessionId: identical(currentSessionId, _kKeepCurrentSessionId)
        ? this.currentSessionId
        : currentSessionId as int?,
      sessions: sessions ?? this.sessions,
      messages: messages ?? this.messages,
      currentStreamingText: currentStreamingText,
      error: error,
      runners: runners ?? this.runners,
      runnerNames: runnerNames ?? this.runnerNames,
      selectedRunner: selectedRunner ?? this.selectedRunner,
      hasActiveRunners: hasActiveRunners ?? this.hasActiveRunners,
      sessionSettings: sessionSettings ?? this.sessionSettings,
      retryText: clearRetryPayload ? null : (retryText ?? this.retryText),
      retryAttachmentFileName: clearRetryPayload
        ? null
        : (retryAttachmentFileName ?? this.retryAttachmentFileName),
      retryAttachmentContent: clearRetryPayload
        ? null
        : (retryAttachmentContent ?? this.retryAttachmentContent),
    );
  }

  @override
  List<Object?> get props => [
    isConnected,
    isLoading,
    isStreaming,
    currentSessionId,
    sessions,
    messages,
    currentStreamingText,
    error,
    runners,
    runnerNames,
    selectedRunner,
    hasActiveRunners,
    sessionSettings,
    retryText,
    retryAttachmentFileName,
    retryAttachmentContent,
  ];
}
