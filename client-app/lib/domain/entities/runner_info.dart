import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/gpu_info.dart';
import 'package:gen/domain/entities/loaded_model_status.dart';
import 'package:gen/domain/entities/server_info.dart';

class RunnerInfo extends Equatable {
  final int id;
  final String address;
  final String name;
  final String host;
  final int port;
  final bool enabled;
  final bool connected;
  final List<GpuInfo> gpus;
  final ServerInfo? serverInfo;
  final LoadedModelStatus? loadedModel;
  final String selectedModel;

  const RunnerInfo({
    this.id = 0,
    required this.address,
    this.name = '',
    this.host = '',
    this.port = 0,
    required this.enabled,
    this.connected = false,
    this.gpus = const [],
    this.serverInfo,
    this.loadedModel,
    this.selectedModel = '',
  });

  @override
  List<Object?> get props => [
    id,
    address,
    name,
    host,
    port,
    enabled,
    connected,
    gpus,
    serverInfo,
    loadedModel,
    selectedModel,
  ];
}
