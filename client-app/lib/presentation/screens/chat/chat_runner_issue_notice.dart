import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_level.dart';

String? chatRunnerIssueNoticeMessage(ChatState state) {
  if (!state.hasCompletedInitialConnection || state.isLoading) {
    return null;
  }

  if (!state.isConnected) {
    return null;
  }

  final sel = state.selectedRunner;
  if (sel != null && sel.isNotEmpty) {
    if (state.selectedRunnerEnabled == false) {
      return 'Выбранный раннер выключен.';
    }

    if (state.selectedRunnerConnected == false) {
      return 'Выбранный раннер недоступен (нет подключения).';
    }
  }

  if (state.hasActiveRunners == false) {
    return 'На сервере нет активных раннеров.';
  }

  if (state.runners.isEmpty) {
    return 'Нет доступных раннеров: список пуст или все отключены.';
  }

  if (sel != null && sel.isNotEmpty && !state.runners.contains(sel)) {
    return 'Выбранный раннер недоступен.';
  }

  return null;
}

bool isChatRunnerIssueTopNoticePersistentText(String message) {
  final t = message.trim();
  return const {
    'Выбранный раннер выключен.',
    'Выбранный раннер недоступен (нет подключения).',
    'На сервере нет активных раннеров.',
    'Нет доступных раннеров: список пуст или все отключены.',
    'Выбранный раннер недоступен.',
  }.contains(t);
}

AppTopNoticeLevel chatRunnerIssueNoticeLevel(ChatState state) {
  final sel = state.selectedRunner;
  if (sel != null && sel.isNotEmpty) {
    if (state.selectedRunnerEnabled == false) {
      return AppTopNoticeLevel.error;
    }

    if (state.selectedRunnerConnected == false) {
      return AppTopNoticeLevel.error;
    }
  }

  if (sel != null && sel.isNotEmpty && state.runners.isNotEmpty && !state.runners.contains(sel)) {
    return AppTopNoticeLevel.error;
  }

  return AppTopNoticeLevel.warning;
}

bool shouldEmitChatRunnerIssueNotice(ChatState previous, ChatState current) {
  final a = chatRunnerIssueNoticeMessage(previous);
  final b = chatRunnerIssueNoticeMessage(current);

  return a != b && b != null;
}
