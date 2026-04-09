import 'package:equatable/equatable.dart';

abstract class ChatEvent extends Equatable {
  const ChatEvent();

  @override
  List<Object?> get props => [];
}

class ChatStarted extends ChatEvent {
  const ChatStarted();
}

class ChatReconnectAfterConnectionRestored extends ChatEvent {
  const ChatReconnectAfterConnectionRestored();
}

class ChatCreateSession extends ChatEvent {
  final String? title;

  const ChatCreateSession({this.title});

  @override
  List<Object?> get props => [title];
}

class ChatLoadSessions extends ChatEvent {
  final int page;
  final int pageSize;

  const ChatLoadSessions({this.page = 1, this.pageSize = 20});

  @override
  List<Object?> get props => [page, pageSize];
}

class ChatSelectSession extends ChatEvent {
  final int sessionId;
  final bool forceReload;

  const ChatSelectSession(this.sessionId, {this.forceReload = false});

  @override
  List<Object?> get props => [sessionId, forceReload];
}

class ChatLoadOlderMessages extends ChatEvent {
  const ChatLoadOlderMessages();

  @override
  List<Object?> get props => [];
}

class ChatSendMessage extends ChatEvent {
  final String text;
  final String? attachmentFileName;
  final List<int>? attachmentContent;
  final int? attachmentFileId;

  const ChatSendMessage(
    this.text, {
    this.attachmentFileName,
    this.attachmentContent,
    this.attachmentFileId,
  });

  @override
  List<Object?> get props => [text, attachmentFileName, attachmentContent, attachmentFileId];
}

class ChatClearError extends ChatEvent {
  const ChatClearError();
}

class ChatDismissStreamNotice extends ChatEvent {
  const ChatDismissStreamNotice();
}

class ChatStopGeneration extends ChatEvent {
  const ChatStopGeneration();
}

class ChatRetryLastMessage extends ChatEvent {
  const ChatRetryLastMessage();
}

class ChatRegenerateAssistant extends ChatEvent {
  final int assistantMessageId;

  const ChatRegenerateAssistant(this.assistantMessageId);

  @override
  List<Object?> get props => [assistantMessageId];
}

class ChatContinueAssistant extends ChatEvent {
  final int assistantMessageId;

  const ChatContinueAssistant(this.assistantMessageId);

  @override
  List<Object?> get props => [assistantMessageId];
}

class ChatEditUserMessageAndContinue extends ChatEvent {
  final int userMessageId;
  final String newContent;

  const ChatEditUserMessageAndContinue(this.userMessageId, this.newContent);

  @override
  List<Object?> get props => [userMessageId, newContent];
}

class ChatShowUserMessageEdits extends ChatEvent {
  final int userMessageId;

  const ChatShowUserMessageEdits(this.userMessageId);

  @override
  List<Object?> get props => [userMessageId];
}

class ChatNavigateUserMessageEdit extends ChatEvent {
  final int userMessageId;
  final int delta;

  const ChatNavigateUserMessageEdit(this.userMessageId, this.delta);

  @override
  List<Object?> get props => [userMessageId, delta];
}

class ChatShowAssistantMessageRegenerations extends ChatEvent {
  final int assistantMessageId;

  const ChatShowAssistantMessageRegenerations(this.assistantMessageId);

  @override
  List<Object?> get props => [assistantMessageId];
}

class ChatNavigateAssistantMessageRegeneration extends ChatEvent {
  final int assistantMessageId;
  final int delta;

  const ChatNavigateAssistantMessageRegeneration(this.assistantMessageId, this.delta);

  @override
  List<Object?> get props => [assistantMessageId, delta];
}

class ChatLoadRunners extends ChatEvent {
  const ChatLoadRunners();
}

class ChatSelectRunner extends ChatEvent {
  final String runner;

  const ChatSelectRunner(this.runner);

  @override
  List<Object?> get props => [runner];
}

class ChatDeleteSession extends ChatEvent {
  final int sessionId;

  const ChatDeleteSession(this.sessionId);

  @override
  List<Object?> get props => [sessionId];
}

class ChatUpdateSessionTitle extends ChatEvent {
  final int sessionId;
  final String title;

  const ChatUpdateSessionTitle(this.sessionId, this.title);

  @override
  List<Object?> get props => [sessionId, title];
}

class ChatLoadSessionSettings extends ChatEvent {
  final int sessionId;

  const ChatLoadSessionSettings(this.sessionId);

  @override
  List<Object?> get props => [sessionId];
}

class ChatUpdateSessionSettings extends ChatEvent {
  final String systemPrompt;
  final List<String> stopSequences;
  final int timeoutSeconds;
  final double? temperature;
  final int? topK;
  final double? topP;
  final bool jsonMode;
  final String jsonSchema;
  final String toolsJson;
  final String profile;
  final bool modelReasoningEnabled;
  final bool webSearchEnabled;
  final String webSearchProvider;
  final bool mcpEnabled;
  final List<int> mcpServerIds;

  const ChatUpdateSessionSettings({
    required this.systemPrompt,
    required this.stopSequences,
    required this.timeoutSeconds,
    this.temperature,
    this.topK,
    this.topP,
    required this.jsonMode,
    required this.jsonSchema,
    required this.toolsJson,
    required this.profile,
    this.modelReasoningEnabled = false,
    this.webSearchEnabled = false,
    this.webSearchProvider = '',
    this.mcpEnabled = false,
    this.mcpServerIds = const [],
  });

  @override
  List<Object?> get props => [
    systemPrompt,
    stopSequences,
    timeoutSeconds,
    temperature,
    topK,
    topP,
    jsonMode,
    jsonSchema,
    toolsJson,
    profile,
    modelReasoningEnabled,
    webSearchEnabled,
    webSearchProvider,
    mcpEnabled,
    mcpServerIds,
  ];
}

class ChatSetModelReasoning extends ChatEvent {
  final bool enabled;

  const ChatSetModelReasoning(this.enabled);

  @override
  List<Object?> get props => [enabled];
}

class ChatSetWebSearch extends ChatEvent {
  final bool enabled;
  final String provider;

  const ChatSetWebSearch({required this.enabled, this.provider = ''});

  @override
  List<Object?> get props => [enabled, provider];
}

class ChatSetMcpDraft extends ChatEvent {
  final bool enabled;
  final List<int> serverIds;

  const ChatSetMcpDraft({required this.enabled, this.serverIds = const []});

  @override
  List<Object?> get props => [enabled, serverIds];
}
