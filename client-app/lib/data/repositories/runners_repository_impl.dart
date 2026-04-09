import 'package:gen/domain/entities/mcp_probe_result_entity.dart';
import 'package:gen/domain/entities/mcp_server_entity.dart';
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
  Future<void> updateWebSearchSettings(WebSearchSettingsEntity settings) => _remote.updateWebSearchSettings(settings);

  @override
  Future<bool> getWebSearchGloballyEnabled() => _remote.getWebSearchGloballyEnabled();

  @override
  Future<List<McpServerEntity>> listMcpServers() => _remote.listMcpServers();

  @override
  Future<McpServerEntity> createMcpServer(McpServerEntity server) => _remote.createMcpServer(server);

  @override
  Future<McpServerEntity> updateMcpServer(McpServerEntity server) => _remote.updateMcpServer(server);

  @override
  Future<void> deleteMcpServer(int id) => _remote.deleteMcpServer(id);

  @override
  Future<List<McpServerEntity>> listUserMcpServers() => _remote.listUserMcpServers();

  @override
  Future<McpServerEntity> createUserMcpServer(McpServerEntity server) => _remote.createUserMcpServer(server);

  @override
  Future<McpServerEntity> updateUserMcpServer(McpServerEntity server) => _remote.updateUserMcpServer(server);

  @override
  Future<void> deleteUserMcpServer(int id) => _remote.deleteUserMcpServer(id);

  @override
  Future<McpProbeResultEntity> probeUserMcpServer(int id) => _remote.probeUserMcpServer(id);

  @override
  Future<McpProbeResultEntity> probeMcpServer(int id) => _remote.probeMcpServer(id);
}
