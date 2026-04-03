import 'package:flutter/material.dart';
import 'package:gen/core/runner_host_parse.dart';
import 'package:gen/domain/entities/runner_info.dart';

String _initialAddress(RunnerInfo? existing) {
  if (existing == null) {
    return '';
  }

  if (existing.address.isNotEmpty) {
    return existing.address;
  }

  if (existing.host.isEmpty) {
    return '';
  }

  if (existing.port <= 0) {
    return existing.host;
  }

  if (existing.host.contains(']:')) {
    return existing.host;
  }

  if (existing.host.startsWith('[')) {
    return '${existing.host}:${existing.port}';
  }

  if (existing.host.contains(':')) {
    return '[${existing.host}]:${existing.port}';
  }

  return '${existing.host}:${existing.port}';
}

class RunnerFormValues {
  final String name;
  final String host;
  final int port;
  final bool enabled;

  const RunnerFormValues({
    required this.name,
    required this.host,
    required this.port,
    required this.enabled,
  });
}

Future<RunnerFormValues?> showRunnerAdminFormDialog(
  BuildContext context, {
  RunnerInfo? existing,
}) {
  return showDialog<RunnerFormValues>(
    context: context,
    builder: (ctx) => _RunnerAdminFormDialog(existing: existing),
  );
}

class _RunnerAdminFormDialog extends StatefulWidget {
  final RunnerInfo? existing;

  const _RunnerAdminFormDialog({this.existing});

  @override
  State<_RunnerAdminFormDialog> createState() => _RunnerAdminFormDialogState();
}

class _RunnerAdminFormDialogState extends State<_RunnerAdminFormDialog> {
  late final TextEditingController _nameCtl;
  late final TextEditingController _addressCtl;

  @override
  void initState() {
    super.initState();
    _nameCtl = TextEditingController(text: widget.existing?.name ?? '');
    _addressCtl = TextEditingController(text: _initialAddress(widget.existing));
  }

  @override
  void dispose() {
    _nameCtl.dispose();
    _addressCtl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final existing = widget.existing;
    final isCreate = existing == null;

    return AlertDialog(
      title: Text(isCreate ? 'Новый раннер' : 'Редактирование'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: _nameCtl,
              decoration: const InputDecoration(
                labelText: 'Имя',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _addressCtl,
              decoration: const InputDecoration(
                labelText: 'Хост',
                hintText: 'хост:порт',
                border: OutlineInputBorder(),
              ),
              keyboardType: TextInputType.url,
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Отмена'),
        ),
        FilledButton(
          onPressed: () {
            const fallbackPort = 50052;
            final parsed = parseRunnerHostInput(_addressCtl.text, fallbackPort);
            if (parsed == null) {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(
                  content: Text(
                    'Укажите хост раннера (host, host:port или [IPv6]:port)',
                  ),
                ),
              );
              return;
            }
            if (isCreate) {
              Navigator.pop(
                context,
                RunnerFormValues(
                  name: _nameCtl.text.trim(),
                  host: parsed.host,
                  port: parsed.port,
                  enabled: true,
                ),
              );
            } else {
              Navigator.pop(
                context,
                RunnerFormValues(
                  name: _nameCtl.text.trim(),
                  host: parsed.host,
                  port: parsed.port,
                  enabled: existing.enabled,
                ),
              );
            }
          },
          child: const Text('Сохранить'),
        ),
      ],
    );
  }
}
