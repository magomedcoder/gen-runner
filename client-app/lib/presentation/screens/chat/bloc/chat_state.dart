import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';

const _kKeepCurrentSessionId = Symbol('_kKeepCurrentSessionId');
const _kKeepToolProgress = Object();

class ChatState extends Equatable {
  final bool isConnected;
  final bool isLoading;
  final bool isStreaming;
  final int? currentSessionId;
  final List<ChatSession> sessions;
  final List<Message> messages;
  final String? currentStreamingText;
  final String? toolProgressLabel;
  final String? error;
  final List<String> runners;
  final Map<String, String> runnerNames;
  final String? selectedRunner;
  final bool? hasActiveRunners;
  final bool runnersStatusRefreshing;
  final ChatSessionSettings? sessionSettings;
  final String? retryText;
  final String? retryAttachmentFileName;
  final List<int>? retryAttachmentContent;
  final int? retryAttachmentFileId;
  final Set<int> editedMessageIds;
  final Map<int, List<UserMessageEdit>> editsByMessageId;
  final Map<int, int> editCursorByMessageId;
  final Map<int, int> pendingEditNavDeltaByMessageId;
  final Set<int> regeneratedAssistantMessageIds;
  final Map<int, List<AssistantMessageRegeneration>> regenerationsByMessageId;
  final Map<int, int> regenerationCursorByMessageId;
  final Map<int, int> pendingRegenerationNavDeltaByMessageId;

  const ChatState({
    this.isConnected = false,
    this.isLoading = false,
    this.isStreaming = false,
    this.currentSessionId,
    this.sessions = const [],
    this.messages = const [],
    this.currentStreamingText,
    this.toolProgressLabel,
    this.error,
    this.runners = const [],
    this.runnerNames = const {},
    this.selectedRunner,
    this.hasActiveRunners,
    this.runnersStatusRefreshing = false,
    this.sessionSettings,
    this.retryText,
    this.retryAttachmentFileName,
    this.retryAttachmentContent,
    this.retryAttachmentFileId,
    this.editedMessageIds = const {},
    this.editsByMessageId = const {},
    this.editCursorByMessageId = const {},
    this.pendingEditNavDeltaByMessageId = const {},
    this.regeneratedAssistantMessageIds = const {},
    this.regenerationsByMessageId = const {},
    this.regenerationCursorByMessageId = const {},
    this.pendingRegenerationNavDeltaByMessageId = const {},
  });

  ChatState copyWith({
    bool? isConnected,
    bool? isLoading,
    bool? isStreaming,
    Object? currentSessionId = _kKeepCurrentSessionId,
    List<ChatSession>? sessions,
    List<Message>? messages,
    String? currentStreamingText,
    Object? toolProgressLabel = _kKeepToolProgress,
    String? error,
    List<String>? runners,
    Map<String, String>? runnerNames,
    String? selectedRunner,
    bool? hasActiveRunners,
    bool? runnersStatusRefreshing,
    ChatSessionSettings? sessionSettings,
    String? retryText,
    String? retryAttachmentFileName,
    List<int>? retryAttachmentContent,
    int? retryAttachmentFileId,
    bool clearRetryPayload = false,
    bool clearToolProgress = false,
    Set<int>? editedMessageIds,
    Map<int, List<UserMessageEdit>>? editsByMessageId,
    Map<int, int>? editCursorByMessageId,
    Map<int, int>? pendingEditNavDeltaByMessageId,
    Set<int>? regeneratedAssistantMessageIds,
    Map<int, List<AssistantMessageRegeneration>>? regenerationsByMessageId,
    Map<int, int>? regenerationCursorByMessageId,
    Map<int, int>? pendingRegenerationNavDeltaByMessageId,
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
      toolProgressLabel: clearToolProgress
          ? null
          : (identical(toolProgressLabel, _kKeepToolProgress)
              ? this.toolProgressLabel
              : toolProgressLabel as String?),
      error: error,
      runners: runners ?? this.runners,
      runnerNames: runnerNames ?? this.runnerNames,
      selectedRunner: selectedRunner ?? this.selectedRunner,
      hasActiveRunners: hasActiveRunners ?? this.hasActiveRunners,
      runnersStatusRefreshing: runnersStatusRefreshing ?? this.runnersStatusRefreshing,
      sessionSettings: sessionSettings ?? this.sessionSettings,
      retryText: clearRetryPayload ? null : (retryText ?? this.retryText),
      retryAttachmentFileName: clearRetryPayload
        ? null
        : (retryAttachmentFileName ?? this.retryAttachmentFileName),
      retryAttachmentContent: clearRetryPayload
        ? null
        : (retryAttachmentContent ?? this.retryAttachmentContent),
      retryAttachmentFileId: clearRetryPayload
        ? null
        : (retryAttachmentFileId ?? this.retryAttachmentFileId),
      editedMessageIds: editedMessageIds ?? this.editedMessageIds,
      editsByMessageId: editsByMessageId ?? this.editsByMessageId,
      editCursorByMessageId: editCursorByMessageId ?? this.editCursorByMessageId,
      pendingEditNavDeltaByMessageId: pendingEditNavDeltaByMessageId ?? this.pendingEditNavDeltaByMessageId,
      regeneratedAssistantMessageIds: regeneratedAssistantMessageIds ?? this.regeneratedAssistantMessageIds,
      regenerationsByMessageId: regenerationsByMessageId ?? this.regenerationsByMessageId,
      regenerationCursorByMessageId: regenerationCursorByMessageId ?? this.regenerationCursorByMessageId,
      pendingRegenerationNavDeltaByMessageId: pendingRegenerationNavDeltaByMessageId ?? this.pendingRegenerationNavDeltaByMessageId,
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
    toolProgressLabel,
    error,
    runners,
    runnerNames,
    selectedRunner,
    hasActiveRunners,
    runnersStatusRefreshing,
    sessionSettings,
    retryText,
    retryAttachmentFileName,
    retryAttachmentContent,
    retryAttachmentFileId,
    editedMessageIds,
    editsByMessageId,
    editCursorByMessageId,
    pendingEditNavDeltaByMessageId,
    regeneratedAssistantMessageIds,
    regenerationsByMessageId,
    regenerationCursorByMessageId,
    pendingRegenerationNavDeltaByMessageId,
  ];
}
