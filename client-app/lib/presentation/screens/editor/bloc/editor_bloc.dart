import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/request_logout_on_unauthorized.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/repositories/editor_repository.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/editor/transform_text_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/get_user_runners_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_event.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_state.dart';

class EditorBloc extends Bloc<EditorEvent, EditorState> {
  final AuthBloc authBloc;
  final GetRunnersUseCase getRunnersUseCase;
  final GetUserRunnersUseCase getUserRunnersUseCase;
  final GetSelectedRunnerUseCase getSelectedRunnerUseCase;
  final SetSelectedRunnerUseCase setSelectedRunnerUseCase;
  final TransformTextUseCase transformTextUseCase;
  final EditorRepository editorRepository;

  EditorBloc({
    required this.authBloc,
    required this.getRunnersUseCase,
    required this.getUserRunnersUseCase,
    required this.getSelectedRunnerUseCase,
    required this.setSelectedRunnerUseCase,
    required this.transformTextUseCase,
    required this.editorRepository,
  }) : super(const EditorState()) {
    on<EditorStarted>(_onStarted);
    on<EditorSelectRunner>(_onSelectRunner);
    on<EditorDocumentChanged>(_onDocumentChanged);
    on<EditorTypeChanged>(_onTypeChanged);
    on<EditorPreserveMarkdownChanged>(_onPreserveChanged);
    on<EditorTransformPressed>(_onTransformPressed);
    on<EditorCancelTransform>(_onCancelTransform);
    on<EditorUndo>(_onUndo);
    on<EditorRedo>(_onRedo);
    on<EditorClearError>(_onClearError);
  }

  List<String> _extractAvailableRunners(List<RunnerInfo> runners) {
    final addresses = <String>{
      for (final runner in runners)
        if (runner.enabled && runner.address.isNotEmpty) runner.address,
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

  Future<void> _onStarted(
    EditorStarted event,
    Emitter<EditorState> emit,
  ) async {
    Logs().d('EditorBloc: старт');
    emit(state.copyWith(runnersLoading: true));
    try {
      final isAdmin = authBloc.state.user?.isAdmin ?? false;
      final runnerInfos = isAdmin
        ? await getRunnersUseCase()
        : await getUserRunnersUseCase();
      final runners = _extractAvailableRunners(runnerInfos);
      final runnerNames = _extractRunnerNames(runnerInfos);

      String? saved;
      try {
        saved = await getSelectedRunnerUseCase();
      } catch (e) {
        Logs().w('EditorBloc: не удалось загрузить раннер по умолчанию', exception: e);
      }

      String? effective = saved != null && runners.contains(saved) ? saved : null;
      if (effective == null && runners.isNotEmpty) {
        effective = runners.first;
        try {
          await setSelectedRunnerUseCase(effective);
        } catch (e) {
          Logs().w('EditorBloc: не удалось сохранить раннер по умолчанию', exception: e);
        }
      }

      emit(
        state.copyWith(
          runners: runners,
          runnerNames: runnerNames,
          selectedRunner: effective,
          clearSelectedRunner: effective == null,
          runnersLoading: false,
          documentVersion: state.documentVersion == 0 ? 1 : state.documentVersion,
        ),
      );
    } catch (e) {
      Logs().e('EditorBloc: не удалось загрузить раннеры', exception: e);
      emit(
        state.copyWith(
          runnersLoading: false,
          error: 'Не удалось загрузить раннеры',
          clearError: false,
        ),
      );
    }
  }

  Future<void> _onSelectRunner(
    EditorSelectRunner event,
    Emitter<EditorState> emit,
  ) async {
    if (state.savingRunner || !state.runners.contains(event.runner)) {
      return;
    }

    emit(state.copyWith(savingRunner: true));
    try {
      await setSelectedRunnerUseCase(event.runner);
      emit(state.copyWith(selectedRunner: event.runner, savingRunner: false));
    } catch (e) {
      Logs().e('EditorBloc: не удалось сохранить раннер', exception: e);
      requestLogoutIfUnauthorized(e, authBloc);
      emit(
        state.copyWith(
          savingRunner: false,
          error: 'Не удалось сменить раннер',
          clearError: false,
        ),
      );
    }
  }

  Future<void> _onDocumentChanged(
    EditorDocumentChanged event,
    Emitter<EditorState> emit,
  ) async {
    if (state.documentText == event.text) return;
    emit(state.copyWith(documentText: event.text));
    await editorRepository.saveHistory(
      text: event.text,
      runner: state.selectedRunner,
    );
  }

  void _onTypeChanged(EditorTypeChanged event, Emitter<EditorState> emit) {
    emit(state.copyWith(type: event.type));
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
      await editorRepository.saveHistory(
        text: out,
        runner: state.selectedRunner,
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
    if (!state.isLoading) {
      return;
    }
    await editorRepository.cancelTransform();
    emit(state.copyWith(isLoading: false));
  }

  Future<void> _onUndo(EditorUndo event, Emitter<EditorState> emit) async {
    if (state.isLoading || state.undoStack.isEmpty) {
      return;
    }

    final prev = state.undoStack.last;
    final newUndo = state.undoStack.sublist(0, state.undoStack.length - 1);
    final newRedo = [state.documentText, ...state.redoStack];
    emit(state.copyWith(
      documentText: prev,
      undoStack: newUndo,
      redoStack: newRedo,
      documentVersion: state.documentVersion + 1,
    ));
    await editorRepository.saveHistory(
      text: prev,
      runner: state.selectedRunner,
    );
  }

  Future<void> _onRedo(EditorRedo event, Emitter<EditorState> emit) async {
    if (state.isLoading || state.redoStack.isEmpty) {
      return;
    }

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
    await editorRepository.saveHistory(
      text: nxt,
      runner: state.selectedRunner,
    );
  }

  void _onClearError(EditorClearError event, Emitter<EditorState> emit) {
    emit(state.copyWith(clearError: true));
  }
}
