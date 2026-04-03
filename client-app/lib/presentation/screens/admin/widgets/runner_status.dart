import 'package:flutter/material.dart';
import 'package:gen/domain/entities/runner_info.dart';

enum RunnerStatus { connected, waiting, disabled }

extension RunnerStatusExtension on RunnerStatus {
  String get label {
    switch (this) {
      case RunnerStatus.connected:
        return 'Подключен';
      case RunnerStatus.waiting:
        return 'Ожидание подключения';
      case RunnerStatus.disabled:
        return 'Отключён';
    }
  }
}

RunnerStatus runnerStatusFromRunner(RunnerInfo runner) {
  if (!runner.enabled) {
    return RunnerStatus.disabled;
  }

  if (runner.connected) {
    return RunnerStatus.connected;
  }

  return RunnerStatus.waiting;
}

Color runnerStatusColor(BuildContext context, RunnerStatus status) {
  final scheme = Theme.of(context).colorScheme;
  switch (status) {
    case RunnerStatus.connected:
      return Theme.of(context).brightness == Brightness.dark
          ? const Color(0xFF81C784)
          : const Color(0xFF2E7D32);
    case RunnerStatus.waiting:
      return scheme.tertiary;
    case RunnerStatus.disabled:
      return scheme.outline;
  }
}
