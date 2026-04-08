import 'package:fixnum/fixnum.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/domain/entities/gpu_info.dart' as gpu_ent;
import 'package:gen/domain/entities/loaded_model_status.dart' as lm_ent;
import 'package:gen/domain/entities/runner_info.dart' as domain;
import 'package:gen/domain/entities/web_search_settings.dart';
import 'package:gen/domain/entities/server_info.dart' as srv_ent;
import 'package:gen/generated/grpc_pb/common.pb.dart' as common;
import 'package:gen/generated/grpc_pb/runner.pb.dart' as pb;

abstract class IRunnersRemoteDataSource {
  Future<List<domain.RunnerInfo>> getRunners();

  Future<List<domain.RunnerInfo>> getUserRunners();

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

class RunnersRemoteDataSource implements IRunnersRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  RunnersRemoteDataSource(this._channelManager, this._authGuard);

  domain.RunnerInfo _mapRunner(pb.RunnerInfo r) {
    final gpus = r.gpus
        .map(
          (g) => gpu_ent.GpuInfo(
            name: g.name,
            temperatureC: g.temperatureC,
            memoryTotalMb: g.memoryTotalMb.toInt(),
            memoryUsedMb: g.memoryUsedMb.toInt(),
            utilizationPercent: g.utilizationPercent,
          ),
        )
        .toList();
    srv_ent.ServerInfo? server;
    if (r.hasServerInfo()) {
      final s = r.serverInfo;
      server = srv_ent.ServerInfo(
        hostname: s.hostname,
        os: s.os,
        arch: s.arch,
        cpuCores: s.cpuCores,
        memoryTotalMb: s.memoryTotalMb.toInt(),
        models: List<String>.from(s.models),
      );
    }
    lm_ent.LoadedModelStatus? loaded;
    if (r.hasLoadedModel()) {
      final lm = r.loadedModel;
      loaded = lm_ent.LoadedModelStatus(
        loaded: lm.loaded,
        displayName: lm.displayName,
        ggufBasename: lm.ggufBasename,
      );
    }
    return domain.RunnerInfo(
      id: r.id.toInt(),
      address: r.address,
      name: r.name,
      host: r.host,
      port: r.port,
      enabled: r.enabled,
      connected: r.connected,
      gpus: gpus,
      serverInfo: server,
      loadedModel: loaded,
      selectedModel: r.selectedModel,
    );
  }

  @override
  Future<List<domain.RunnerInfo>> getRunners() async {
    Logs().d('RunnersRemote: getRunners');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerClient.getRunners(common.Empty()),
      );
      Logs().i('RunnersRemote: getRunners получено ${resp.runners.length}');
      return resp.runners.map(_mapRunner).toList();
    } catch (e) {
      Logs().e('RunnersRemote: getRunners', exception: e);
      rethrow;
    }
  }

  @override
  Future<List<domain.RunnerInfo>> getUserRunners() async {
    Logs().d('RunnersRemote: getUserRunners');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerClient.getUserRunners(common.Empty()),
      );
      Logs().i('RunnersRemote: getUserRunners получено ${resp.runners.length}');
      return resp.runners
          .map(
            (r) => domain.RunnerInfo(
              address: r.address,
              name: r.name,
              enabled: true,
              connected: false,
              gpus: const [],
              serverInfo: null,
              loadedModel: null,
              selectedModel: r.selectedModel,
            ),
          )
          .toList();
    } catch (e) {
      Logs().e('RunnersRemote: getUserRunners', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> createRunner({
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  }) async {
    Logs().d('RunnersRemote: createRunner $host:$port');
    try {
      await _authGuard.execute(() => _channelManager.runnerClient.createRunner(
        pb.CreateRunnerRequest(
          name: name,
          host: host,
          port: port,
          enabled: enabled,
          selectedModel: selectedModel,
        ),
      ));
    } catch (e) {
      Logs().e('RunnersRemote: createRunner', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> updateRunner({
    required int id,
    required String name,
    required String host,
    required int port,
    required bool enabled,
    String selectedModel = '',
  }) async {
    Logs().d('RunnersRemote: updateRunner id=$id');
    try {
      await _authGuard.execute(() => _channelManager.runnerClient.updateRunner(
        pb.UpdateRunnerRequest(
          id: Int64(id),
          name: name,
          host: host,
          port: port,
          enabled: enabled,
          selectedModel: selectedModel,
        ),
      ));
    } catch (e) {
      Logs().e('RunnersRemote: updateRunner', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> deleteRunner(int id) async {
    Logs().d('RunnersRemote: deleteRunner id=$id');
    try {
      await _authGuard.execute(() => _channelManager.runnerClient.deleteRunner(
        pb.DeleteRunnerRequest(id: Int64(id)),
      ));
    } catch (e) {
      Logs().e('RunnersRemote: deleteRunner', exception: e);
      rethrow;
    }
  }

  @override
  Future<bool> getRunnersStatus() async {
    Logs().d('RunnersRemote: getRunnersStatus');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerClient.getRunnersStatus(common.Empty()),
      );
      Logs().i('RunnersRemote: getRunnersStatus hasActive=${resp.hasActiveRunners}');
      return resp.hasActiveRunners;
    } catch (e) {
      Logs().e('RunnersRemote: getRunnersStatus', exception: e);
      rethrow;
    }
  }

  @override
  Future<List<String>> getRunnerModels(int runnerId) async {
    Logs().d('RunnersRemote: getRunnerModels id=$runnerId');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerClient.getRunnerModels(
          pb.GetRunnerModelsRequest(runnerId: Int64(runnerId)),
        ),
      );
      return List<String>.from(resp.models);
    } catch (e) {
      Logs().e('RunnersRemote: getRunnerModels', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> runnerLoadModel(int runnerId, String model) async {
    Logs().d('RunnersRemote: runnerLoadModel id=$runnerId');
    try {
      await _authGuard.execute(
        () => _channelManager.runnerClient.runnerLoadModel(
          pb.RunnerLoadModelRequest(runnerId: Int64(runnerId), model: model),
        ),
      );
    } catch (e) {
      Logs().e('RunnersRemote: runnerLoadModel', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> runnerUnloadModel(int runnerId) async {
    Logs().d('RunnersRemote: runnerUnloadModel id=$runnerId');
    try {
      await _authGuard.execute(
        () => _channelManager.runnerClient.runnerUnloadModel(
          pb.RunnerUnloadModelRequest(runnerId: Int64(runnerId)),
        ),
      );
    } catch (e) {
      Logs().e('RunnersRemote: runnerUnloadModel', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> runnerResetMemory(int runnerId) async {
    Logs().d('RunnersRemote: runnerResetMemory id=$runnerId');
    try {
      await _authGuard.execute(
        () => _channelManager.runnerClient.runnerResetMemory(
          pb.RunnerResetMemoryRequest(runnerId: Int64(runnerId)),
        ),
      );
    } catch (e) {
      Logs().e('RunnersRemote: runnerResetMemory', exception: e);
      rethrow;
    }
  }

  WebSearchSettingsEntity _mapWebSearch(pb.WebSearchSettings s) {
    return WebSearchSettingsEntity(
      enabled: s.enabled,
      maxResults: s.maxResults,
      braveApiKey: s.braveApiKey,
      googleApiKey: s.googleApiKey,
      googleSearchEngineId: s.googleSearchEngineId,
      yandexUser: s.yandexUser,
      yandexKey: s.yandexKey,
    );
  }

  @override
  Future<WebSearchSettingsEntity> getWebSearchSettings() async {
    Logs().d('RunnersRemote: getWebSearchSettings');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerClient.getWebSearchSettings(common.Empty()),
      );
      final s = resp.hasSettings() ? resp.settings : pb.WebSearchSettings();
      return _mapWebSearch(s);
    } catch (e) {
      Logs().e('RunnersRemote: getWebSearchSettings', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> updateWebSearchSettings(WebSearchSettingsEntity settings) async {
    Logs().d('RunnersRemote: updateWebSearchSettings');
    try {
      await _authGuard.execute(
        () => _channelManager.runnerClient.updateWebSearchSettings(
          pb.UpdateWebSearchSettingsRequest(
            enabled: settings.enabled,
            maxResults: settings.maxResults,
            braveApiKey: settings.braveApiKey,
            googleApiKey: settings.googleApiKey,
            googleSearchEngineId: settings.googleSearchEngineId,
            yandexUser: settings.yandexUser,
            yandexKey: settings.yandexKey,
          ),
        ),
      );
    } catch (e) {
      Logs().e('RunnersRemote: updateWebSearchSettings', exception: e);
      rethrow;
    }
  }
}
