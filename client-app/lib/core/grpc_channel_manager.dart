import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_interceptor.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/generated/grpc_pb/auth.pbgrpc.dart' as grpc_auth;
import 'package:gen/generated/grpc_pb/chat.pbgrpc.dart' as grpc_chat;
import 'package:gen/generated/grpc_pb/runner.pbgrpc.dart' as grpc_runner;
import 'package:gen/generated/grpc_pb/user.pbgrpc.dart' as grpc_user;
import 'package:gen/generated/grpc_pb/editor.pbgrpc.dart' as grpc_editor;

class GrpcChannelManager {
  final ServerConfig _config;
  final AuthInterceptor _authInterceptor;

  ClientChannel? _channel;
  grpc_auth.AuthServiceClient? _authClient;
  grpc_chat.ChatServiceClient? _chatClient;
  grpc_user.UserServiceClient? _userClient;
  grpc_runner.RunnerServiceClient? _runnerClient;
  grpc_editor.EditorServiceClient? _editorClient;

  GrpcChannelManager(this._config, this._authInterceptor);

  ClientChannel get channel {
    if (_channel == null) {
      Logs().d('Создание канала ${_config.host}:${_config.port}');
      _channel = ClientChannel(
        _config.host,
        port: _config.port,
        options: const ChannelOptions(
          credentials: ChannelCredentials.insecure(),
          idleTimeout: Duration(seconds: 30),
        ),
      );
    }
    return _channel!;
  }

  grpc_auth.AuthServiceClient get authClient {
    _authClient ??= grpc_auth.AuthServiceClient(
      channel,
      interceptors: [_authInterceptor],
    );
    return _authClient!;
  }

  grpc_auth.AuthServiceClient get authClientForVersionCheck {
    return grpc_auth.AuthServiceClient(channel);
  }

  grpc_chat.ChatServiceClient get chatClient {
    _chatClient ??= grpc_chat.ChatServiceClient(
      channel,
      interceptors: [_authInterceptor],
    );
    return _chatClient!;
  }

  grpc_user.UserServiceClient get userClient {
    _userClient ??= grpc_user.UserServiceClient(
      channel,
      interceptors: [_authInterceptor],
    );
    return _userClient!;
  }

  grpc_runner.RunnerServiceClient get runnerClient {
    _runnerClient ??= grpc_runner.RunnerServiceClient(
      channel,
      interceptors: [_authInterceptor],
    );
    return _runnerClient!;
  }

  grpc_editor.EditorServiceClient get editorClient {
    _editorClient ??= grpc_editor.EditorServiceClient(
      channel,
      interceptors: [_authInterceptor],
    );
    return _editorClient!;
  }

  Future<void> setServer(String host, int port) async {
    Logs().i('Установка сервера: $host:$port');
    await _config.setServer(host, port);
    await _closeChannel();
  }

  Future<void> _closeChannel() async {
    final ch = _channel;
    _channel = null;
    _authClient = null;
    _chatClient = null;
    _userClient = null;
    _runnerClient = null;
    _editorClient = null;
    if (ch != null) {
      Logs().d('Закрытие канала');
      await ch.shutdown();
    }
  }
}
