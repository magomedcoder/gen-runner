import 'package:grpc/grpc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/log/logs.dart';

const String kSessionExpiredMessage = 'Сессия истекла, войдите снова';

Never throwGrpcError(
  GrpcError e,
  String networkMessage, {
  String? unauthenticatedMessage,
}) {
  if (e.code == StatusCode.unauthenticated) {
    Logs().w('gRPC: не авторизован - ${unauthenticatedMessage ?? kSessionExpiredMessage}');
    throw UnauthorizedFailure(unauthenticatedMessage ?? kSessionExpiredMessage);
  }

  Logs().e('gRPC: ошибка ${e.code}: $networkMessage');
  throw NetworkFailure(networkMessage);
}
