import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/domain/usecases/chat/get_default_runner_model_usecase.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/set_default_runner_model_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/set_runner_enabled_usecase.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_event.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_state.dart';

class RunnersAdminBloc extends Bloc<RunnersAdminEvent, RunnersAdminState> {
  final GetRunnersUseCase getRunnersUseCase;
  final SetRunnerEnabledUseCase setRunnerEnabledUseCase;
  final GetSelectedRunnerUseCase getSelectedRunnerUseCase;
  final SetSelectedRunnerUseCase setSelectedRunnerUseCase;
  final GetDefaultRunnerModelUseCase getDefaultRunnerModelUseCase;
  final SetDefaultRunnerModelUseCase setDefaultRunnerModelUseCase;

  RunnersAdminBloc({
    required this.getRunnersUseCase,
    required this.setRunnerEnabledUseCase,
    required this.getSelectedRunnerUseCase,
    required this.setSelectedRunnerUseCase,
    required this.getDefaultRunnerModelUseCase,
    required this.setDefaultRunnerModelUseCase,
  }) : super(const RunnersAdminState()) {
    on<RunnersAdminLoadRequested>(_onLoad);
    on<RunnersAdminSetEnabledRequested>(_onSetEnabled);
    on<RunnersAdminClearError>(_onClearError);
    on<RunnersAdminDefaultRunnerChanged>(_onDefaultRunnerChanged);
    on<RunnersAdminDefaultModelChanged>(_onDefaultModelChanged);
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
      final defaultModelsByRunner = <String, String?>{};
      for (final runner in runners) {
        final models = runner.serverInfo?.models ?? const <String>[];
        if (models.isEmpty) {
          continue;
        }
        final savedDefault = await getDefaultRunnerModelUseCase(runner.address);
        if (savedDefault != null && models.contains(savedDefault)) {
          defaultModelsByRunner[runner.address] = savedDefault;
        } else {
          final pick = models.first;
          await setDefaultRunnerModelUseCase(runner.address, pick);
          defaultModelsByRunner[runner.address] = pick;
        }
      }

      Logs().i('RunnersAdminBloc: загружено раннеров: ${runners.length}');
      emit(state.copyWith(
        isLoading: false,
        runners: runners,
        defaultRunner: validDefault,
        defaultModelsByRunner: defaultModelsByRunner,
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

  Future<void> _onSetEnabled(
    RunnersAdminSetEnabledRequested event,
    Emitter<RunnersAdminState> emit,
  ) async {
    Logs().d('RunnersAdminBloc: setEnabled ${event.address} -> ${event.enabled}');
    emit(state.copyWith(isLoading: true, error: null));
    try {
      await setRunnerEnabledUseCase(event.address, event.enabled);
      Logs().i('RunnersAdminBloc: setEnabled успешен');
      add(const RunnersAdminLoadRequested());
    } catch (e) {
      Logs().e('RunnersAdminBloc: setEnabled', exception: e);
      emit(state.copyWith(
        isLoading: false,
        error: userSafeErrorMessage(
          e,
          fallback: 'Ошибка изменения раннера',
        ),
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

  Future<void> _onDefaultModelChanged(
    RunnersAdminDefaultModelChanged event,
    Emitter<RunnersAdminState> emit,
  ) async {
    try {
      await setDefaultRunnerModelUseCase(event.runnerAddress, event.model);
      final updated = Map<String, String?>.from(state.defaultModelsByRunner);
      updated[event.runnerAddress] = event.model;
      emit(state.copyWith(defaultModelsByRunner: updated));
    } catch (e) {
      Logs().e('RunnersAdminBloc: defaultModel', exception: e);
      emit(state.copyWith(
        error: userSafeErrorMessage(
          e,
          fallback: 'Ошибка выбора модели',
        ),
      ));
    }
  }
}
