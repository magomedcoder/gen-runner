import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/entities/web_search_settings.dart';
import 'package:gen/domain/repositories/runners_repository.dart';
import 'package:gen/data/data_sources/remote/runners_remote_datasource.dart';

class RunnersRepositoryImpl implements RunnersRepository {
  final IRunnersRemoteDataSource _remote;

  RunnersRepositoryImpl(this._remote);

  @override
  Future<List<RunnerInfo>> getRunners() => _remote.getRunners();

  @override
  Future<List<RunnerInfo>> getUserRunners() => _remote.getUserRunners();

  @override
  Future<void> createRunner({
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  }) => _remote.createRunner(
    name: name,
    host: host,
    port: port,
    enabled: enabled,
    selectedModel: selectedModel,
  );

  @override
  Future<void> updateRunner({
    required int id,
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  }) => _remote.updateRunner(
    id: id,
    name: name,
    host: host,
    port: port,
    enabled: enabled,
    selectedModel: selectedModel,
  );

  @override
  Future<void> deleteRunner(int id) => _remote.deleteRunner(id);

  @override
  Future<bool> getRunnersStatus() => _remote.getRunnersStatus();

  @override
  Future<List<String>> getRunnerModels(int runnerId) => _remote.getRunnerModels(runnerId);

  @override
  Future<void> runnerLoadModel(int runnerId, String model) =>
      _remote.runnerLoadModel(runnerId, model);

  @override
  Future<void> runnerUnloadModel(int runnerId) => _remote.runnerUnloadModel(runnerId);

  @override
  Future<void> runnerResetMemory(int runnerId) => _remote.runnerResetMemory(runnerId);

  @override
  Future<WebSearchSettingsEntity> getWebSearchSettings() => _remote.getWebSearchSettings();

  @override
  Future<void> updateWebSearchSettings(WebSearchSettingsEntity settings) =>
      _remote.updateWebSearchSettings(settings);
}
