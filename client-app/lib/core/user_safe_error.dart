import 'package:grpc/grpc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_error_info_reason.dart';
import 'package:gen/core/grpc_error_reason_message.dart';

String userSafeErrorMessage(
  Object? error, {
  String fallback = 'Произошла ошибка',
}) {
  if (error == null) {
    return fallback;
  }
  if (error is GrpcError) {
    final reason = grpcErrorInfoReason(error);
    final fromReason = messageForGrpcErrorInfoReason(reason);
    final m = error.message?.trim();
    if (m != null && m.isNotEmpty) {
      return m;
    }
    if (fromReason != null && fromReason.isNotEmpty) {
      return fromReason;
    }
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

String? userSafeErrorGrpcReason(Object? error) => error is GrpcError ? grpcErrorInfoReason(error) : null;
