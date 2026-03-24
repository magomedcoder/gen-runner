import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/generated/grpc_pb/editor.pbgrpc.dart' as grpc;

abstract class IEditorRemoteDataSource {
  Future<String> transform({
    required String text,
    required grpc.TransformType type,
    bool preserveMarkdown,
  });

  Future<void> cancelTransform();
  Future<void> saveHistory({
    required String text,
    String? runner,
  });
}

class EditorRemoteDataSource implements IEditorRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  EditorRemoteDataSource(this._channelManager, this._authGuard);

  grpc.EditorServiceClient get _client => _channelManager.editorClient;

  ResponseFuture<grpc.TransformResponse>? _activeTransform;

  @override
  Future<void> cancelTransform() async {
    final t = _activeTransform;
    if (t != null) {
      _activeTransform = null;
      await t.cancel();
    }
  }

  @override
  Future<String> transform({
    required String text,
    required grpc.TransformType type,
    bool preserveMarkdown = false,
  }) async {
    Logs().d('EditorRemoteDataSource: transform type=$type');
    await cancelTransform();

    final request = grpc.TransformRequest(
      text: text,
      type: type,
      preserveMarkdown: preserveMarkdown,
    );

    Future<grpc.TransformResponse> invokeOnce() async {
      final rf = _client.transform(request);
      _activeTransform = rf;
      try {
        return await rf;
      } finally {
        if (_activeTransform == rf) {
          _activeTransform = null;
        }
      }
    }

    try {
      final resp = await _authGuard.execute(invokeOnce);
      return resp.text;
    } on GrpcError catch (e) {
      if (e.code == StatusCode.cancelled) {
        throw ApiFailure('Обработка остановлена');
      }
      Logs().e('EditorRemoteDataSource: ошибка transform', exception: e);
      throwGrpcError(e, 'Ошибка обработки текста');
    } catch (e) {
      if (e is Failure) rethrow;
      Logs().e('EditorRemoteDataSource: ошибка transform', exception: e);
      throw ApiFailure('Ошибка обработки текста');
    }
  }

  @override
  Future<void> saveHistory({
    required String text,
    String? runner,
  }) async {
    try {
      await _authGuard.execute(
        () => _client.saveHistory(
          grpc.SaveHistoryRequest(
            text: text,
            runner: runner ?? '',
          ),
        ),
      );
    } catch (_) {}
  }
}
