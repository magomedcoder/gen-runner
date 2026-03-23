import 'package:equatable/equatable.dart';
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
  final List<String> models;
  final String? selectedModel;
  final bool? hasActiveRunners;

  const ChatState({
    this.isConnected = false,
    this.isLoading = false,
    this.isStreaming = false,
    this.currentSessionId,
    this.sessions = const [],
    this.messages = const [],
    this.currentStreamingText,
    this.error,
    this.models = const [],
    this.selectedModel,
    this.hasActiveRunners,
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
    List<String>? models,
    String? selectedModel,
    bool? hasActiveRunners,
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
      models: models ?? this.models,
      selectedModel: selectedModel ?? this.selectedModel,
      hasActiveRunners: hasActiveRunners ?? this.hasActiveRunners,
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
    models,
    selectedModel,
    hasActiveRunners,
  ];
}
