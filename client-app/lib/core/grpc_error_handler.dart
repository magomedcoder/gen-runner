import 'package:grpc/grpc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/log/logs.dart';

const String kSessionExpiredMessage = 'Сессия истекла, войдите снова';

Never throwGrpcError(
  GrpcError e,
  String logContext, {
  String? unauthenticatedMessage,
}) {
  if (e.code == StatusCode.unauthenticated) {
    Logs().w(
      'gRPC не авторизован [$logContext]: ${e.message}',
    );
    throw UnauthorizedFailure(unauthenticatedMessage ?? kSessionExpiredMessage);
  }

  Logs().e(
    'gRPC [$logContext] code=${e.code} serverMessage=${e.message}',
  );
  throw NetworkFailure('Ошибка сервера (код ${e.code})');
}
