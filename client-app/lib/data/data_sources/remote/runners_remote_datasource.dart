import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/domain/entities/gpu_info.dart' as gpu_ent;
import 'package:gen/domain/entities/runner_info.dart' as domain;
import 'package:gen/domain/entities/server_info.dart' as srv_ent;
import 'package:gen/generated/grpc_pb/runner.pb.dart' as pb;

abstract class IRunnersRemoteDataSource {
  Future<List<domain.RunnerInfo>> getRunners();

  Future<void> setRunnerEnabled(String address, bool enabled);

  Future<bool> getRunnersStatus();
}

class RunnersRemoteDataSource implements IRunnersRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  RunnersRemoteDataSource(this._channelManager, this._authGuard);

  @override
  Future<List<domain.RunnerInfo>> getRunners() async {
    Logs().d('RunnersRemote: getRunners');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerAdminClient.getRunners(pb.Empty()),
      );
      Logs().i('RunnersRemote: getRunners получено ${resp.runners.length}');
      return resp.runners.map((r) {
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
        return domain.RunnerInfo(
          address: r.address,
          enabled: r.enabled,
          connected: r.connected,
          gpus: gpus,
          serverInfo: server,
        );
      }).toList();
    } catch (e) {
      Logs().e('RunnersRemote: getRunners', exception: e);
      rethrow;
    }
  }

  @override
  Future<void> setRunnerEnabled(String address, bool enabled) async {
    Logs().d('RunnersRemote: setRunnerEnabled address=$address enabled=$enabled');
    try {
      await _authGuard.execute(() => _channelManager.runnerAdminClient.setRunnerEnabled(
        pb.SetRunnerEnabledRequest(address: address, enabled: enabled),
      ));
      Logs().i('RunnersRemote: setRunnerEnabled успешен');
    } catch (e) {
      Logs().e('RunnersRemote: setRunnerEnabled', exception: e);
      rethrow;
    }
  }

  @override
  Future<bool> getRunnersStatus() async {
    Logs().d('RunnersRemote: getRunnersStatus');
    try {
      final resp = await _authGuard.execute(
        () => _channelManager.runnerAdminClient.getRunnersStatus(pb.Empty()),
      );
      Logs().i('RunnersRemote: getRunnersStatus hasActive=${resp.hasActiveRunners}');
      return resp.hasActiveRunners;
    } catch (e) {
      Logs().e('RunnersRemote: getRunnersStatus', exception: e);
      rethrow;
    }
  }
}
