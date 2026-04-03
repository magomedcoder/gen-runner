import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/runners/create_runner_usecase.dart';
import 'package:gen/domain/usecases/runners/delete_runner_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/update_runner_usecase.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_event.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_state.dart';

class RunnersAdminBloc extends Bloc<RunnersAdminEvent, RunnersAdminState> {
  final GetRunnersUseCase getRunnersUseCase;
  final CreateRunnerUseCase createRunnerUseCase;
  final UpdateRunnerUseCase updateRunnerUseCase;
  final DeleteRunnerUseCase deleteRunnerUseCase;
  final GetSelectedRunnerUseCase getSelectedRunnerUseCase;
  final SetSelectedRunnerUseCase setSelectedRunnerUseCase;

  RunnersAdminBloc({
    required this.getRunnersUseCase,
    required this.createRunnerUseCase,
    required this.updateRunnerUseCase,
    required this.deleteRunnerUseCase,
    required this.getSelectedRunnerUseCase,
    required this.setSelectedRunnerUseCase,
  }) : super(const RunnersAdminState()) {
    on<RunnersAdminLoadRequested>(_onLoad);
    on<RunnersAdminCreateRequested>(_onCreate);
    on<RunnersAdminUpdateRequested>(_onUpdate);
    on<RunnersAdminDeleteRequested>(_onDelete);
    on<RunnersAdminClearError>(_onClearError);
    on<RunnersAdminDefaultRunnerChanged>(_onDefaultRunnerChanged);
  }

  Future<void> _onLoad(
    RunnersAdminLoadRequested event,
    Emitter<RunnersAdminState> emit,
  ) async {
    Logs().d('RunnersAdminBloc: загрузка раннеров');
    emit(state.copyWith(isLoading: true, error: null));
    try {
      final runners = await getRunnersUseCase();
      final defaultRunner = await getSelectedRunnerUseCase();
      final availableAddresses = {
        for (final runner in runners)
          if (runner.enabled && runner.address.isNotEmpty) runner.address,
      };
      var validDefault = defaultRunner != null && availableAddresses.contains(defaultRunner)
          ? defaultRunner
          : null;
      if (validDefault == null && availableAddresses.isNotEmpty) {
        final sorted = availableAddresses.toList()..sort();
        validDefault = sorted.first;
        await setSelectedRunnerUseCase(validDefault);
      }

      Logs().i('RunnersAdminBloc: загружено раннеров: ${runners.length}');
      emit(state.copyWith(
        isLoading: false,
        runners: runners,
        defaultRunner: validDefault,
        error: null,
      ));
    } catch (e) {
      Logs().e('RunnersAdminBloc: ошибка загрузки', exception: e);
      emit(state.copyWith(
        isLoading: false,
        error: userSafeErrorMessage(
          e,
          fallback: 'Ошибка загрузки раннеров',
        ),
      ));
    }
  }

  Future<void> _onCreate(
    RunnersAdminCreateRequested event,
    Emitter<RunnersAdminState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));
    try {
      await createRunnerUseCase(
        name: event.name,
        host: event.host,
        port: event.port,
        enabled: event.enabled,
      );
      add(const RunnersAdminLoadRequested());
    } catch (e) {
      emit(state.copyWith(
        isLoading: false,
        error: userSafeErrorMessage(e, fallback: 'Не удалось добавить раннер'),
      ));
    }
  }

  Future<void> _onUpdate(
    RunnersAdminUpdateRequested event,
    Emitter<RunnersAdminState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));
    try {
      await updateRunnerUseCase(
        id: event.id,
        name: event.name,
        host: event.host,
        port: event.port,
        enabled: event.enabled,
      );
      add(const RunnersAdminLoadRequested());
    } catch (e) {
      emit(state.copyWith(
        isLoading: false,
        error: userSafeErrorMessage(e, fallback: 'Не удалось сохранить раннер'),
      ));
    }
  }

  Future<void> _onDelete(
    RunnersAdminDeleteRequested event,
    Emitter<RunnersAdminState> emit,
  ) async {
    emit(state.copyWith(isLoading: true, error: null));
    try {
      await deleteRunnerUseCase(event.id);
      add(const RunnersAdminLoadRequested());
    } catch (e) {
      emit(state.copyWith(
        isLoading: false,
        error: userSafeErrorMessage(e, fallback: 'Не удалось удалить раннер'),
      ));
    }
  }

  void _onClearError(
    RunnersAdminClearError event,
    Emitter<RunnersAdminState> emit,
  ) {
    emit(state.copyWith(error: null));
  }

  Future<void> _onDefaultRunnerChanged(
    RunnersAdminDefaultRunnerChanged event,
    Emitter<RunnersAdminState> emit,
  ) async {
    try {
      await setSelectedRunnerUseCase(event.address);
      emit(state.copyWith(defaultRunner: event.address));
    } catch (e) {
      Logs().e('RunnersAdminBloc: defaultRunner', exception: e);
      emit(state.copyWith(
        error: userSafeErrorMessage(
          e,
          fallback: 'Ошибка выбора раннера по умолчанию',
        ),
      ));
    }
  }
}
