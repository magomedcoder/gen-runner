import 'package:flutter/material.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/core/mcp_connection_config.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:gen/domain/entities/mcp_server_entity.dart';
import 'package:gen/domain/repositories/runners_repository.dart';
import 'package:gen/presentation/widgets/mcp_connection_json_dialog_section.dart';
import 'package:gen/presentation/widgets/mcp_probe_result_dialog.dart';
class McpAdminScreen extends StatefulWidget {
  const McpAdminScreen({super.key});

  @override
  State<McpAdminScreen> createState() => _McpAdminScreenState();
}

class _McpAdminScreenState extends State<McpAdminScreen> {
  final _repo = di.sl<RunnersRepository>();

  bool _loading = true;
  String? _loadError;
  List<McpServerEntity> _servers = [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadError = null;
    });

    try {
      final list = await _repo.listMcpServers();
      if (!mounted) {
        return;
      }

      setState(() {
        _servers = list;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) {
        return;
      }

      setState(() {
        _loadError = userSafeErrorMessage(
          e,
          fallback: 'Не удалось загрузить список серверов',
        );
        _loading = false;
      });
    }
  }

  Future<void> _probe(McpServerEntity s) async {
    try {
      final r = await _repo.probeMcpServer(s.id);
      if (!mounted) {
        return;
      }

      await showMcpProbeResultDialog(context, r);
    } catch (e) {
      if (!mounted) {
        return;
      }
      showAppTopNotice('Проверка MCP: $e', error: true);
    }
  }

  Future<void> _delete(McpServerEntity s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Удалить MCP-сервер?'),
        content: Text(s.name.isNotEmpty ? s.name : 'id=${s.id}'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Отмена'),
          ),
          FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Удалить'),
          ),
        ],
      ),
    );

    if (ok != true) {
      return;
    }

    try {
      await _repo.deleteMcpServer(s.id);
      if (!mounted) {
        return;
      }

      showAppTopNotice('Сервер удалён');
      await _load();
    } catch (e) {
      if (!mounted) {
        return;
      }
      showAppTopNotice('Ошибка: $e', error: true);
    }
  }

  Future<void> _openEditor({McpServerEntity? existing}) async {
    final isNew = existing == null;
    final nameCtrl = TextEditingController(text: existing?.name ?? '');
    final jsonCtrl = TextEditingController(
      text: existing != null
        ? McpConnectionConfig.prettyFromEntity(existing)
        : McpConnectionConfig.defaultJsonPretty(),
    );
    var enabled = existing?.enabled ?? true;
    final editId = existing?.id ?? 0;

    final saved = await showDialog<bool>(
      context: context,
      builder: (ctx) {
        return AlertDialog(
          title: Text(
            isNew ? 'Новый MCP-сервер' : 'Редактировать MCP #$editId',
          ),
          content: SizedBox(
            width: 520,
            child: SingleChildScrollView(
              child: StatefulBuilder(
                builder: (ctx, setSt) => Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    TextField(
                      controller: nameCtrl,
                      decoration: const InputDecoration(
                        labelText: 'Имя',
                        border: OutlineInputBorder(),
                      ),
                    ),
                    const SizedBox(height: 8),
                    SwitchListTile(
                      contentPadding: EdgeInsets.zero,
                      title: const Text('Включён'),
                      value: enabled,
                      onChanged: (v) => setSt(() => enabled = v),
                    ),
                    McpConnectionJsonDialogSection(
                      jsonCtrl: jsonCtrl,
                      isNew: isNew,
                    ),
                  ],
                ),
              ),
            ),
          ),
          actions: [
            TextButton(
                onPressed: () => Navigator.pop(ctx, false),
                child: const Text('Отмена'),
            ),
            FilledButton(
              onPressed: () {
                try {
                  McpConnectionConfig.parse(jsonCtrl.text);
                } on FormatException catch (e) {
                  showAppTopNotice(e.message, error: true);
                  return;
                } catch (e) {
                  showAppTopNotice('Некорректный JSON: $e', error: true);
                  return;
                }
                Navigator.pop(ctx, true);
              },
              child: const Text('Сохранить'),
            ),
          ],
        );
      },
    );

    if (saved != true) {
      return;
    }

    late final McpConnectionConfig cfg;
    try {
      cfg = McpConnectionConfig.parse(jsonCtrl.text);
    } on FormatException catch (e) {
      showAppTopNotice(e.message, error: true);
      return;
    } catch (e) {
      showAppTopNotice('Некорректный JSON: $e', error: true);
      return;
    }

    final entity = cfg.toEntity(
      id: existing?.id ?? 0,
      name: nameCtrl.text.trim(),
      enabled: enabled,
      ownerUserId: existing?.ownerUserId ?? 0,
    );

    try {
      if (isNew) {
        await _repo.createMcpServer(entity);
      } else {
        await _repo.updateMcpServer(entity);
      }

      if (!mounted) {
        return;
      }

      showAppTopNotice(isNew ? 'Сервер создан' : 'Сервер обновлён');
      await _load();
    } catch (e) {
      if (!mounted) {
        return;
      }
      showAppTopNotice('Ошибка: $e', error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    if (_loadError != null) {
      return Scaffold(
        appBar: AppBar(title: const Text('MCP')),
        body: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text('Не удалось загрузить:\n$_loadError', textAlign: TextAlign.center),
                const SizedBox(height: 16),
                FilledButton(onPressed: _load, child: const Text('Повторить')),
              ],
            ),
          ),
        ),
      );
    }

    return Scaffold(
      appBar: AppBar(
        title: const Text('MCP серверы'),
        actions: [
          IconButton(onPressed: _load, icon: const Icon(Icons.refresh)),
        ],
      ),
      floatingActionButton: FloatingActionButton(
        onPressed: () => _openEditor(),
        child: const Icon(Icons.add),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          ...List.generate(_servers.length, (i) {
            final s = _servers[i];
            final title = s.name.isNotEmpty ? s.name : 'Сервер #${s.id}';
            return Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Card(
                child: ListTile(
                  title: Text(title),
                  subtitle: Text(
                    '${s.enabled ? "вкл" : "выкл"} ${s.transport} ${s.command.isNotEmpty ? s.command : s.url}',
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                  isThreeLine: true,
                  trailing: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      IconButton(
                        tooltip: 'Проверить подключение',
                        icon: const Icon(Icons.cable_rounded),
                        onPressed: () => _probe(s),
                      ),
                      IconButton(
                        icon: const Icon(Icons.edit_outlined),
                        onPressed: () => _openEditor(existing: s),
                      ),
                      IconButton(
                        icon: const Icon(Icons.delete_outline),
                        onPressed: () => _delete(s),
                      ),
                    ],
                  ),
                ),
              ),
            );
          }),
        ],
      ),
    );
  }
}
