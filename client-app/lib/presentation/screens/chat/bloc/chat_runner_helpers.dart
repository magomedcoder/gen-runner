import 'package:gen/domain/entities/runner_info.dart';

List<String> extractAvailableRunners(List<RunnerInfo> runners) {
  final addresses = <String>{
    for (final runner in runners)
      if (runner.enabled && runner.address.isNotEmpty) runner.address,
  };
  final sorted = addresses.toList()..sort();

  return sorted;
}

Map<String, String> extractRunnerNames(List<RunnerInfo> runners) {
  final names = <String, String>{};
  for (final runner in runners) {
    if (!runner.enabled || runner.address.isEmpty) {
      continue;
    }

    final name = runner.name.trim();
    names[runner.address] = name.isNotEmpty ? name : runner.address;
  }

  return names;
}

(bool?, bool?) runnerHealthForSelection(
  String? selected,
  List<RunnerInfo> infos,
) {
  if (selected == null || selected.isEmpty || infos.isEmpty) {
    return (null, null);
  }

  for (final r in infos) {
    if (r.address == selected) {
      return (r.enabled, r.connected);
    }
  }

  return (null, null);
}
