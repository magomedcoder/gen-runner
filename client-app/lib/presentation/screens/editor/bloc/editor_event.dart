import 'package:equatable/equatable.dart';
import 'package:gen/generated/grpc_pb/editor.pb.dart' as grpc;

sealed class EditorEvent extends Equatable {
  const EditorEvent();

  @override
  List<Object?> get props => [];
}

final class EditorStarted extends EditorEvent {
  const EditorStarted();
}

final class EditorSelectRunner extends EditorEvent {
  final String runner;

  const EditorSelectRunner(this.runner);

  @override
  List<Object?> get props => [runner];
}

final class EditorDocumentChanged extends EditorEvent {
  final String text;

  const EditorDocumentChanged(this.text);

  @override
  List<Object?> get props => [text];
}

final class EditorTransformPressed extends EditorEvent {
  const EditorTransformPressed();
}

final class EditorCancelTransform extends EditorEvent {
  const EditorCancelTransform();
}

final class EditorUndo extends EditorEvent {
  const EditorUndo();
}

final class EditorRedo extends EditorEvent {
  const EditorRedo();
}

final class EditorTypeChanged extends EditorEvent {
  final grpc.TransformType type;
  const EditorTypeChanged(this.type);

  @override
  List<Object?> get props => [type];
}

final class EditorPreserveMarkdownChanged extends EditorEvent {
  final bool preserve;

  const EditorPreserveMarkdownChanged(this.preserve);

  @override
  List<Object?> get props => [preserve];
}

final class EditorClearError extends EditorEvent {
  const EditorClearError();
}
