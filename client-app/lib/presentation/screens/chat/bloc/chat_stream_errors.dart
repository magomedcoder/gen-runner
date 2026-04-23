import 'package:gen/core/chat_backend_user_error.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_unavailable.dart';

const String kChatEmptyAssistantResponseMessage = 'Сервер не вернул ответ. Проверьте доступность раннера и попробуйте снова.';

String chatStreamFailureMessage(Object error, {required String lead}) {
  if (error is Failure) {
    return '$lead: ${error.message}';
  }

  return '$lead: ${chatBackendErrorMessage(error)}';
}

String? chatStreamErrorForState(Object e, {required String lead}) {
  if (isGrpcUnavailable(e)) {
    return null;
  }

  return chatStreamFailureMessage(e, lead: lead);
}
