import 'package:flutter_test/flutter_test.dart';
import 'package:gen/core/chat_backend_user_error.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:grpc/grpc.dart';

void main() {
  test('chatBackendErrorMessage maps deadline to runner/gen hint', () {
    final s = chatBackendErrorMessage(
      const GrpcError.deadlineExceeded('timeout'),
    );
    expect(s, contains('раннер'));
    expect(s, contains('gen'));
  });

  test('chatHeadlineIfBackendReachable null on unavailable', () {
    expect(
      chatHeadlineIfBackendReachable(
        const GrpcError.unavailable('14'),
        'Заголовок',
      ),
      isNull,
    );
  });

  test('Failure passes message through', () {
    expect(
      chatBackendErrorMessage(const ApiFailure('bad')),
      'bad',
    );
  });

  test('userSafeErrorMessage falls back to chatBackend when no detail', () {
    expect(
      userSafeErrorMessage(const GrpcError.deadlineExceeded('')),
      contains('раннер'),
    );
  });
}
