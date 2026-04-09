import 'package:gen/core/failures.dart';
import 'package:grpc/grpc.dart';

bool _isUnavailableCode(int code) => code == StatusCode.unavailable;

bool isGrpcUnavailable(Object? error) {
  if (error is GrpcError && _isUnavailableCode(error.code)) {
    return true;
  }

  if (error is Failure) {
    final m = error.message;
    if (_failureMessageMeansUnavailable(m)) {
      return true;
    }
  }

  return false;
}

bool _failureMessageMeansUnavailable(String m) {
  if (m.contains('(код 14)')) {
    return true;
  }

  if (m.contains('код 14') || m.contains('code 14')) {
    return true;
  }

  return false;
}

bool isServerUnreachableToastText(String message) {
  final t = message.trim();
  if (t.isEmpty) {
    return false;
  }

  if (t.contains('(код 14)')) {
    return true;
  }

  if (t.contains('(code: 14)') || t.contains('code: 14,')) {
    return true;
  }

  return false;
}

bool isPersistentServerAvailabilityTopNoticeText(String message) {
  final t = message.trim();
  if (t.isEmpty) {
    return false;
  }

  if (isServerUnreachableToastText(t)) {
    return true;
  }

  if (t.contains('Проверьте доступность раннера')) {
    return true;
  }

  if (t.contains('Ошибка проверки подключения')) {
    return true;
  }

  return false;
}
