import 'package:grpc/grpc.dart';
import 'package:gen/core/failures.dart';

String userSafeErrorMessage(
  Object? error, {
  String fallback = 'Произошла ошибка',
}) {
  if (error == null) {
    return fallback;
  }
  if (error is GrpcError) {
    return 'Ошибка сервера (код ${error.code})';
  }
  if (error is UnauthorizedFailure) {
    return error.message;
  }
  if (error is Failure) {
    return error.message;
  }
  return fallback;
}
