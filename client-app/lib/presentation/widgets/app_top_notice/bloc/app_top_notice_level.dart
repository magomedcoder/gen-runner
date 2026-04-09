enum AppTopNoticeLevel {
  info,
  warning,
  error,
}

extension AppTopNoticeLevelX on AppTopNoticeLevel {
  int get weight => switch (this) {
    AppTopNoticeLevel.info => 0,
    AppTopNoticeLevel.warning => 1,
    AppTopNoticeLevel.error => 2,
  };
}
