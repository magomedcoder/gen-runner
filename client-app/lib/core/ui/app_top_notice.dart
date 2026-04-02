import 'package:gen/core/injector.dart';
import 'package:gen/core/ui/app_top_notice_controller.dart';

void showAppTopNotice(
  String message, {
  bool error = false,
  Duration? duration,
}) {
  sl<AppTopNoticeController>().show(
    message,
    error: error,
    duration: duration,
  );
}
