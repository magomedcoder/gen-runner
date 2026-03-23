import 'package:equatable/equatable.dart';
import 'package:gen/generated/grpc_pb/editor.pb.dart' as grpc;

class EditorState extends Equatable {
  final bool isLoading;
  final String documentText;
  final List<String> undoStack;
  final List<String> redoStack;
  final List<String> models;
  final String? selectedModel;
  final grpc.TransformType type;
  final bool preserveMarkdown;
  final String? error;
  final int documentVersion;

  const EditorState({
    this.isLoading = false,
    this.documentText = '',
    this.undoStack = const [],
    this.redoStack = const [],
    this.models = const [],
    this.selectedModel,
    this.type = grpc.TransformType.TRANSFORM_TYPE_FIX,
    this.preserveMarkdown = false,
    this.error,
    this.documentVersion = 0,
  });

  bool get canUndo => undoStack.isNotEmpty;

  bool get canRedo => redoStack.isNotEmpty;

  EditorState copyWith({
    bool? isLoading,
    String? documentText,
    List<String>? undoStack,
    List<String>? redoStack,
    List<String>? models,
    String? selectedModel,
    bool clearSelectedModel = false,
    grpc.TransformType? type,
    bool? preserveMarkdown,
    String? error,
    bool clearError = false,
    int? documentVersion,
  }) {
    return EditorState(
      isLoading: isLoading ?? this.isLoading,
      documentText: documentText ?? this.documentText,
      undoStack: undoStack ?? this.undoStack,
      redoStack: redoStack ?? this.redoStack,
      models: models ?? this.models,
      selectedModel: clearSelectedModel
          ? null
          : (selectedModel ?? this.selectedModel),
      type: type ?? this.type,
      preserveMarkdown: preserveMarkdown ?? this.preserveMarkdown,
      error: clearError ? null : (error ?? this.error),
      documentVersion: documentVersion ?? this.documentVersion,
    );
  }

  @override
  List<Object?> get props => [
        isLoading,
        documentText,
        undoStack,
        redoStack,
        models,
        selectedModel,
        type,
        preserveMarkdown,
        error,
        documentVersion,
      ];
}
