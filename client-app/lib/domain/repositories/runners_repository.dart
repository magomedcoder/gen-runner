import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/entities/web_search_settings.dart';

abstract class RunnersRepository {
  Future<List<RunnerInfo>> getRunners();

  Future<List<RunnerInfo>> getUserRunners();

  Future<void> createRunner({
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  });

  Future<void> updateRunner({
    required int id,
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  });

  Future<void> deleteRunner(int id);

  Future<bool> getRunnersStatus();

  Future<List<String>> getRunnerModels(int runnerId);

  Future<void> runnerLoadModel(int runnerId, String model);

  Future<void> runnerUnloadModel(int runnerId);

  Future<void> runnerResetMemory(int runnerId);

  Future<WebSearchSettingsEntity> getWebSearchSettings();

  Future<void> updateWebSearchSettings(WebSearchSettingsEntity settings);
}
