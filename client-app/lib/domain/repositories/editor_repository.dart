import 'package:gen/generated/grpc_pb/editor.pb.dart' as grpc;

abstract interface class EditorRepository {
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
