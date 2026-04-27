import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/rag_document_preview.dart';
import 'package:gen/domain/entities/rag_ingestion_ui.dart';
import 'package:gen/domain/entities/session.dart';
import 'package:gen/domain/entities/assistant_message_regeneration.dart';
import 'package:gen/domain/entities/user_message_edit.dart';

const _kKeepCurrentSessionId = Symbol('_kKeepCurrentSessionId');
const _kKeepToolProgress = Object();
const _kKeepStreamNotice = Object();
const _kKeepRagDocumentPreview = Object();
const _kKeepRagIngestionUi = Object();
const _kKeepSelectedRunnerEnabled = Object();
const _kKeepSelectedRunnerConnected = Object();

class ChatState extends Equatable {
  final bool isConnected;
  final bool hasCompletedInitialConnection;
  final bool isLoading;
  final bool isStreaming;
  final int? streamingSessionId;
  final List<Message>? streamingParkedMessages;
  final int? currentSessionId;
  final List<ChatSession> sessions;
  final List<Message> messages;
  final String? currentStreamingText;
  final String? currentStreamingReasoning;
  final String? toolProgressLabel;
  final String? error;
  final String? streamNotice;
  final RagDocumentPreview? ragDocumentPreview;
  final List<String> runners;
  final Map<String, String> runnerNames;
  final String? selectedRunner;
  final bool? hasActiveRunners;
  final bool runnersStatusRefreshing;
  final bool? selectedRunnerEnabled;
  final bool? selectedRunnerConnected;
  final ChatSessionSettings? sessionSettings;
  final String? retryText;
  final String? retryAttachmentFileName;
  final List<String> retryAttachmentFileNames;
  final List<int>? retryAttachmentContent;
  final List<List<int>> retryAttachmentContents;
  final int? retryAttachmentFileId;
  final List<int> retryAttachmentFileIds;
  final Set<int> editedMessageIds;
  final Map<int, List<UserMessageEdit>> editsByMessageId;
  final Map<int, int> editCursorByMessageId;
  final Map<int, int> pendingEditNavDeltaByMessageId;
  final Set<int> regeneratedAssistantMessageIds;
  final Map<int, List<AssistantMessageRegeneration>> regenerationsByMessageId;
  final Map<int, int> regenerationCursorByMessageId;
  final Map<int, int> pendingRegenerationNavDeltaByMessageId;
  final bool hasMoreOlderMessages;
  final bool isLoadingOlderMessages;
  final int? partialAssistantMessageId;
  final bool draftModelReasoningEnabled;
  final bool webSearchGloballyEnabled;
  final bool draftWebSearchEnabled;
  final String draftWebSearchProvider;
  final bool draftMcpEnabled;
  final List<int> draftMcpServerIds;
  final RagIngestionUi? ragIngestionUi;
  final Map<String, RagDocumentPreview> ragPreviewBySessionFile;

  bool get isStreamingInCurrentSession =>
      isStreaming &&
      streamingSessionId != null &&
      currentSessionId == streamingSessionId;

  bool get isEmptyChatComposer =>
      messages.isEmpty && !isStreamingInCurrentSession;

  const ChatState({
    this.isConnected = false,
    this.hasCompletedInitialConnection = false,
    this.isLoading = false,
    this.isStreaming = false,
    this.streamingSessionId,
    this.streamingParkedMessages,
    this.currentSessionId,
    this.sessions = const [],
    this.messages = const [],
    this.currentStreamingText,
    this.currentStreamingReasoning,
    this.toolProgressLabel,
    this.error,
    this.streamNotice,
    this.ragDocumentPreview,
    this.runners = const [],
    this.runnerNames = const {},
    this.selectedRunner,
    this.hasActiveRunners,
    this.runnersStatusRefreshing = false,
    this.selectedRunnerEnabled,
    this.selectedRunnerConnected,
    this.sessionSettings,
    this.retryText,
    this.retryAttachmentFileName,
    this.retryAttachmentFileNames = const [],
    this.retryAttachmentContent,
    this.retryAttachmentContents = const [],
    this.retryAttachmentFileId,
    this.retryAttachmentFileIds = const [],
    this.editedMessageIds = const {},
    this.editsByMessageId = const {},
    this.editCursorByMessageId = const {},
    this.pendingEditNavDeltaByMessageId = const {},
    this.regeneratedAssistantMessageIds = const {},
    this.regenerationsByMessageId = const {},
    this.regenerationCursorByMessageId = const {},
    this.pendingRegenerationNavDeltaByMessageId = const {},
    this.hasMoreOlderMessages = false,
    this.isLoadingOlderMessages = false,
    this.partialAssistantMessageId,
    this.draftModelReasoningEnabled = false,
    this.webSearchGloballyEnabled = false,
    this.draftWebSearchEnabled = false,
    this.draftWebSearchProvider = '',
    this.draftMcpEnabled = false,
    this.draftMcpServerIds = const [],
    this.ragIngestionUi,
    this.ragPreviewBySessionFile = const {},
  });

  ChatState copyWith({
    bool? isConnected,
    bool? hasCompletedInitialConnection,
    bool? isLoading,
    bool? isStreaming,
    int? streamingSessionId,
    bool clearStreamingSessionId = false,
    List<Message>? streamingParkedMessages,
    bool clearStreamingParked = false,
    Object? currentSessionId = _kKeepCurrentSessionId,
    List<ChatSession>? sessions,
    List<Message>? messages,
    String? currentStreamingText,
    String? currentStreamingReasoning,
    Object? toolProgressLabel = _kKeepToolProgress,
    String? error,
    Object? streamNotice = _kKeepStreamNotice,
    bool clearStreamNotice = false,
    Object? ragDocumentPreview = _kKeepRagDocumentPreview,
    bool clearRagDocumentPreview = false,
    List<String>? runners,
    Map<String, String>? runnerNames,
    String? selectedRunner,
    bool? hasActiveRunners,
    bool? runnersStatusRefreshing,
    Object? selectedRunnerEnabled = _kKeepSelectedRunnerEnabled,
    Object? selectedRunnerConnected = _kKeepSelectedRunnerConnected,
    bool clearSelectedRunnerHealth = false,
    ChatSessionSettings? sessionSettings,
    String? retryText,
    String? retryAttachmentFileName,
    List<String>? retryAttachmentFileNames,
    List<int>? retryAttachmentContent,
    List<List<int>>? retryAttachmentContents,
    int? retryAttachmentFileId,
    List<int>? retryAttachmentFileIds,
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
    bool? hasMoreOlderMessages,
    bool? isLoadingOlderMessages,
    int? partialAssistantMessageId,
    bool clearPartialAssistant = false,
    bool? draftModelReasoningEnabled,
    bool? webSearchGloballyEnabled,
    bool? draftWebSearchEnabled,
    String? draftWebSearchProvider,
    bool? draftMcpEnabled,
    List<int>? draftMcpServerIds,
    Object? ragIngestionUi = _kKeepRagIngestionUi,
    bool clearRagIngestionUi = false,
    Map<String, RagDocumentPreview>? ragPreviewBySessionFile,
  }) {
    return ChatState(
      isConnected: isConnected ?? this.isConnected,
      hasCompletedInitialConnection:
          hasCompletedInitialConnection ?? this.hasCompletedInitialConnection,
      isLoading: isLoading ?? this.isLoading,
      isStreaming: isStreaming ?? this.isStreaming,
      streamingSessionId: clearStreamingSessionId
          ? null
          : (streamingSessionId ?? this.streamingSessionId),
      streamingParkedMessages: clearStreamingParked
          ? null
          : (streamingParkedMessages ?? this.streamingParkedMessages),
      currentSessionId: identical(currentSessionId, _kKeepCurrentSessionId)
          ? this.currentSessionId
          : currentSessionId as int?,
      sessions: sessions ?? this.sessions,
      messages: messages ?? this.messages,
      currentStreamingText: currentStreamingText,
      currentStreamingReasoning: currentStreamingReasoning,
      toolProgressLabel: clearToolProgress
          ? null
          : (identical(toolProgressLabel, _kKeepToolProgress)
                ? this.toolProgressLabel
                : toolProgressLabel as String?),
      error: error,
      streamNotice: clearStreamNotice
          ? null
          : (identical(streamNotice, _kKeepStreamNotice)
                ? this.streamNotice
                : streamNotice as String?),
      ragDocumentPreview: clearRagDocumentPreview
          ? null
          : (identical(ragDocumentPreview, _kKeepRagDocumentPreview)
                ? this.ragDocumentPreview
                : ragDocumentPreview as RagDocumentPreview?),
      runners: runners ?? this.runners,
      runnerNames: runnerNames ?? this.runnerNames,
      selectedRunner: selectedRunner ?? this.selectedRunner,
      hasActiveRunners: hasActiveRunners ?? this.hasActiveRunners,
      runnersStatusRefreshing:
          runnersStatusRefreshing ?? this.runnersStatusRefreshing,
      selectedRunnerEnabled: clearSelectedRunnerHealth
          ? null
          : (identical(selectedRunnerEnabled, _kKeepSelectedRunnerEnabled)
                ? this.selectedRunnerEnabled
                : selectedRunnerEnabled as bool?),
      selectedRunnerConnected: clearSelectedRunnerHealth
          ? null
          : (identical(selectedRunnerConnected, _kKeepSelectedRunnerConnected)
                ? this.selectedRunnerConnected
                : selectedRunnerConnected as bool?),
      sessionSettings: sessionSettings ?? this.sessionSettings,
      retryText: clearRetryPayload ? null : (retryText ?? this.retryText),
      retryAttachmentFileName: clearRetryPayload
          ? null
          : (retryAttachmentFileName ?? this.retryAttachmentFileName),
      retryAttachmentFileNames: clearRetryPayload
          ? const []
          : (retryAttachmentFileNames ?? this.retryAttachmentFileNames),
      retryAttachmentContent: clearRetryPayload
          ? null
          : (retryAttachmentContent ?? this.retryAttachmentContent),
      retryAttachmentContents: clearRetryPayload
          ? const []
          : (retryAttachmentContents ?? this.retryAttachmentContents),
      retryAttachmentFileId: clearRetryPayload
          ? null
          : (retryAttachmentFileId ?? this.retryAttachmentFileId),
      retryAttachmentFileIds: clearRetryPayload
          ? const []
          : (retryAttachmentFileIds ?? this.retryAttachmentFileIds),
      editedMessageIds: editedMessageIds ?? this.editedMessageIds,
      editsByMessageId: editsByMessageId ?? this.editsByMessageId,
      editCursorByMessageId:
          editCursorByMessageId ?? this.editCursorByMessageId,
      pendingEditNavDeltaByMessageId:
          pendingEditNavDeltaByMessageId ?? this.pendingEditNavDeltaByMessageId,
      regeneratedAssistantMessageIds:
          regeneratedAssistantMessageIds ?? this.regeneratedAssistantMessageIds,
      regenerationsByMessageId:
          regenerationsByMessageId ?? this.regenerationsByMessageId,
      regenerationCursorByMessageId:
          regenerationCursorByMessageId ?? this.regenerationCursorByMessageId,
      pendingRegenerationNavDeltaByMessageId:
          pendingRegenerationNavDeltaByMessageId ??
          this.pendingRegenerationNavDeltaByMessageId,
      hasMoreOlderMessages: hasMoreOlderMessages ?? this.hasMoreOlderMessages,
      isLoadingOlderMessages:
          isLoadingOlderMessages ?? this.isLoadingOlderMessages,
      partialAssistantMessageId: clearPartialAssistant
          ? null
          : (partialAssistantMessageId ?? this.partialAssistantMessageId),
      draftModelReasoningEnabled:
          draftModelReasoningEnabled ?? this.draftModelReasoningEnabled,
      webSearchGloballyEnabled:
          webSearchGloballyEnabled ?? this.webSearchGloballyEnabled,
      draftWebSearchEnabled:
          draftWebSearchEnabled ?? this.draftWebSearchEnabled,
      draftWebSearchProvider:
          draftWebSearchProvider ?? this.draftWebSearchProvider,
      draftMcpEnabled: draftMcpEnabled ?? this.draftMcpEnabled,
      draftMcpServerIds: draftMcpServerIds ?? this.draftMcpServerIds,
      ragIngestionUi: clearRagIngestionUi
          ? null
          : (identical(ragIngestionUi, _kKeepRagIngestionUi)
                ? this.ragIngestionUi
                : ragIngestionUi as RagIngestionUi?),
      ragPreviewBySessionFile:
          ragPreviewBySessionFile ?? this.ragPreviewBySessionFile,
    );
  }

  @override
  List<Object?> get props => [
    isConnected,
    hasCompletedInitialConnection,
    isLoading,
    isStreaming,
    streamingSessionId,
    streamingParkedMessages,
    currentSessionId,
    sessions,
    messages,
    currentStreamingText,
    currentStreamingReasoning,
    toolProgressLabel,
    error,
    streamNotice,
    ragDocumentPreview,
    runners,
    runnerNames,
    selectedRunner,
    hasActiveRunners,
    runnersStatusRefreshing,
    selectedRunnerEnabled,
    selectedRunnerConnected,
    sessionSettings,
    retryText,
    retryAttachmentFileName,
    retryAttachmentFileNames,
    retryAttachmentContent,
    retryAttachmentContents,
    retryAttachmentFileId,
    retryAttachmentFileIds,
    editedMessageIds,
    editsByMessageId,
    editCursorByMessageId,
    pendingEditNavDeltaByMessageId,
    regeneratedAssistantMessageIds,
    regenerationsByMessageId,
    regenerationCursorByMessageId,
    pendingRegenerationNavDeltaByMessageId,
    hasMoreOlderMessages,
    isLoadingOlderMessages,
    partialAssistantMessageId,
    draftModelReasoningEnabled,
    webSearchGloballyEnabled,
    draftWebSearchEnabled,
    draftWebSearchProvider,
    draftMcpEnabled,
    draftMcpServerIds,
    ragIngestionUi,
    ragPreviewBySessionFile,
  ];
}
