import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/request_logout_on_unauthorized.dart';
import 'package:gen/domain/repositories/editor_repository.dart';
import 'package:gen/domain/usecases/chat/get_models_usecase.dart';
import 'package:gen/domain/usecases/editor/transform_text_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_event.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_state.dart';

class EditorBloc extends Bloc<EditorEvent, EditorState> {
  final AuthBloc authBloc;
  final GetModelsUseCase getModelsUseCase;
  final TransformTextUseCase transformTextUseCase;
  final EditorRepository editorRepository;

  EditorBloc({
    required this.authBloc,
    required this.getModelsUseCase,
    required this.transformTextUseCase,
    required this.editorRepository,
  }) : super(const EditorState()) {
    on<EditorStarted>(_onStarted);
    on<EditorDocumentChanged>(_onDocumentChanged);
    on<EditorTypeChanged>(_onTypeChanged);
    on<EditorModelChanged>(_onModelChanged);
    on<EditorPreserveMarkdownChanged>(_onPreserveChanged);
    on<EditorTransformPressed>(_onTransformPressed);
    on<EditorCancelTransform>(_onCancelTransform);
    on<EditorUndo>(_onUndo);
    on<EditorRedo>(_onRedo);
    on<EditorClearError>(_onClearError);
  }

  Future<void> _onStarted(
    EditorStarted event,
    Emitter<EditorState> emit,
  ) async {
    Logs().d('EditorBloc: старт');
    List<String> models = [];
    try {
      models = await getModelsUseCase();
    } catch (e) {
      Logs().w('EditorBloc: не удалось загрузить модели', exception: e);
    }

    final sm = state.selectedModel;
    final resolved = (sm != null && models.contains(sm))
        ? sm
        : (models.isNotEmpty ? models.first : null);

    emit(
      state.copyWith(
        models: models,
        selectedModel: resolved,
        clearSelectedModel: resolved == null,
        documentVersion: state.documentVersion == 0 ? 1 : state.documentVersion,
      ),
    );
  }

  void _onDocumentChanged(
    EditorDocumentChanged event,
    Emitter<EditorState> emit,
  ) {
    if (state.documentText == event.text) return;
    emit(state.copyWith(documentText: event.text));
  }

  void _onTypeChanged(EditorTypeChanged event, Emitter<EditorState> emit) {
    emit(state.copyWith(type: event.type));
  }

  void _onModelChanged(EditorModelChanged event, Emitter<EditorState> emit) {
    emit(
      state.copyWith(
        selectedModel: event.model,
        clearSelectedModel: event.model == null,
      ),
    );
  }

  void _onPreserveChanged(
    EditorPreserveMarkdownChanged event,
    Emitter<EditorState> emit,
  ) {
    emit(state.copyWith(preserveMarkdown: event.preserve));
  }

  Future<void> _onTransformPressed(
    EditorTransformPressed event,
    Emitter<EditorState> emit,
  ) async {
    final input = state.documentText.trim();
    if (input.isEmpty) {
      emit(state.copyWith(error: 'Введите текст', clearError: false));
      return;
    }

    emit(state.copyWith(isLoading: true, clearError: true));
    try {
      final out = await transformTextUseCase(
        text: input,
        type: state.type,
        model: state.selectedModel,
        preserveMarkdown: state.preserveMarkdown,
      );

      final newUndo = [...state.undoStack, state.documentText];
      emit(
        state.copyWith(
          isLoading: false,
          documentText: out,
          undoStack: newUndo,
          redoStack: const [],
          documentVersion: state.documentVersion + 1,
        ),
      );
    } catch (e) {
      if (e is ApiFailure && e.message == 'Обработка остановлена') {
        emit(state.copyWith(isLoading: false));
        return;
      }
      Logs().e('EditorBloc: ошибка transform', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      emit(
        state.copyWith(
          isLoading: false,
          error: 'Ошибка обработки текста',
          clearError: false,
        ),
      );
    }
  }

  Future<void> _onCancelTransform(
    EditorCancelTransform event,
    Emitter<EditorState> emit,
  ) async {
    if (!state.isLoading) return;
    await editorRepository.cancelTransform();
    emit(state.copyWith(isLoading: false));
  }

  void _onUndo(EditorUndo event, Emitter<EditorState> emit) {
    if (state.isLoading || state.undoStack.isEmpty) return;

    final prev = state.undoStack.last;
    final newUndo =
        state.undoStack.sublist(0, state.undoStack.length - 1);
    final newRedo = [state.documentText, ...state.redoStack];
    emit(
      state.copyWith(
        documentText: prev,
        undoStack: newUndo,
        redoStack: newRedo,
        documentVersion: state.documentVersion + 1,
      ),
    );
  }

  void _onRedo(EditorRedo event, Emitter<EditorState> emit) {
    if (state.isLoading || state.redoStack.isEmpty) return;

    final nxt = state.redoStack.first;
    final newRedo = state.redoStack.sublist(1);
    final newUndo = [...state.undoStack, state.documentText];
    emit(
      state.copyWith(
        documentText: nxt,
        undoStack: newUndo,
        redoStack: newRedo,
        documentVersion: state.documentVersion + 1,
      ),
    );
  }

  void _onClearError(EditorClearError event, Emitter<EditorState> emit) {
    emit(state.copyWith(clearError: true));
  }
}
