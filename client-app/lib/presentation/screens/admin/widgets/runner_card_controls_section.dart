import 'package:flutter/material.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/repositories/runners_repository.dart';

class RunnerCardControlsSection extends StatefulWidget {
  final RunnerInfo runner;
  final void Function(bool enabled) onRunnerEnabledChanged;
  final VoidCallback onAfterOperation;

  const RunnerCardControlsSection({
    super.key,
    required this.runner,
    required this.onRunnerEnabledChanged,
    required this.onAfterOperation,
  });

  @override
  State<RunnerCardControlsSection> createState() =>
      _RunnerCardControlsSectionState();
}

class _RunnerCardControlsSectionState extends State<RunnerCardControlsSection> {
  late bool _modelWorkEnabled;
  List<String>? _models;
  String? _modelsError;
  bool _loadingModels = false;
  String? _selectedModel;
  bool _modelBusy = false;
  bool _memoryResetBusy = false;

  RunnersRepository get _repo => di.sl<RunnersRepository>();

  @override
  void initState() {
    super.initState();
    _syncFromRunner(widget.runner, forceLoad: true);
  }

  @override
  void didUpdateWidget(RunnerCardControlsSection oldWidget) {
    super.didUpdateWidget(oldWidget);
    final r = widget.runner;
    if (oldWidget.runner.id != r.id) {
      _syncFromRunner(r, forceLoad: true);
      return;
    }
    if (oldWidget.runner.enabled != r.enabled ||
        oldWidget.runner.loadedModel?.loaded != r.loadedModel?.loaded) {
      _syncFromRunner(r, forceLoad: false);
    }
  }

  void _syncFromRunner(RunnerInfo r, {required bool forceLoad}) {
    if (!r.enabled) {
      _modelWorkEnabled = false;
    } else {
      final hasLoadedModel = r.loadedModel?.loaded == true;
      if (hasLoadedModel) {
        _modelWorkEnabled = true;
      } else if (forceLoad) {
        _modelWorkEnabled = true;
      }
    }
    if (forceLoad) {
      _models = null;
      _modelsError = null;
      _selectedModel = null;
    } else if (!r.enabled) {
      _models = null;
      _modelsError = null;
    }
    if (r.enabled && _modelWorkEnabled && (forceLoad || _models == null)) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _loadModels();
      });
    }
  }

  String? _defaultModelSelection(List<String> list, RunnerInfo r) {
    if (list.isEmpty) return null;
    final prev = _selectedModel;
    if (prev != null && list.contains(prev)) return prev;
    final lm = r.loadedModel;
    if (lm != null && lm.loaded) {
      final b = lm.ggufBasename.trim();
      final d = lm.displayName.trim();
      for (final m in list) {
        final t = m.trim();
        if (t.isEmpty) continue;
        if (b.isNotEmpty && (t == b || t.endsWith(b) || b.endsWith(t))) {
          return m;
        }
        if (d.isNotEmpty && (t == d || t.contains(d) || d.contains(t))) {
          return m;
        }
      }
    }
    return list.first;
  }

  Future<void> _loadModels() async {
    final r = widget.runner;
    if (!mounted) return;
    setState(() {
      _loadingModels = true;
      _modelsError = null;
      _models = null;
    });
    try {
      final list = await _repo.getRunnerModels(r.id);
      if (!mounted) return;
      setState(() {
        _models = list;
        _loadingModels = false;
        _selectedModel = _defaultModelSelection(list, widget.runner);
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _loadingModels = false;
        _modelsError = userSafeErrorMessage(
          e,
          fallback: 'Не удалось получить список моделей',
        );
      });
    }
  }

  Future<void> _applyLoadModel() async {
    final r = widget.runner;
    final m = _selectedModel;
    if (m == null || m.isEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Выберите модель')),
      );
      return;
    }
    setState(() => _modelBusy = true);
    try {
      await _repo.runnerLoadModel(r.id, m);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Модель загружена')),
      );
      widget.onAfterOperation();
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            userSafeErrorMessage(e, fallback: 'Не удалось загрузить модель'),
          ),
        ),
      );
    } finally {
      if (mounted) setState(() => _modelBusy = false);
    }
  }

  Future<void> _applyUnloadModel() async {
    final r = widget.runner;
    setState(() => _modelBusy = true);
    try {
      await _repo.runnerUnloadModel(r.id);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Модель выгружена')),
      );
      widget.onAfterOperation();
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            userSafeErrorMessage(e, fallback: 'Не удалось выгрузить модель'),
          ),
        ),
      );
    } finally {
      if (mounted) setState(() => _modelBusy = false);
    }
  }

  Future<void> _onConnectionChanged(bool v) async {
    final r = widget.runner;
    if (!v) {
      try {
        if (_modelWorkEnabled) {
          await _repo.runnerUnloadModel(r.id);
        }
      } catch (_) {}
      if (!mounted) return;
      setState(() {
        _modelWorkEnabled = false;
        _models = null;
        _modelsError = null;
        _selectedModel = null;
      });
      widget.onRunnerEnabledChanged(false);
      widget.onAfterOperation();
      return;
    }
    setState(() {
      _modelWorkEnabled = true;
    });
    widget.onRunnerEnabledChanged(true);
    widget.onAfterOperation();
  }

  Future<void> _onModelWorkChanged(bool v) async {
    final r = widget.runner;
    if (!v) {
      setState(() => _modelBusy = true);
      try {
        await _repo.runnerUnloadModel(r.id);
        if (!mounted) return;
        setState(() => _modelWorkEnabled = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Модель выгружена, работа остановлена')),
        );
        widget.onAfterOperation();
      } catch (e) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              userSafeErrorMessage(e, fallback: 'Не удалось выгрузить модель'),
            ),
          ),
        );
      } finally {
        if (mounted) setState(() => _modelBusy = false);
      }
      return;
    }
    setState(() => _modelWorkEnabled = true);
    if (r.enabled) {
      _loadModels();
    }
  }

  Future<void> _resetRunnerMemory() async {
    final r = widget.runner;
    if (_memoryResetBusy || !r.enabled || !_modelWorkEnabled) return;
    final messenger = ScaffoldMessenger.of(context);
    setState(() => _memoryResetBusy = true);
    try {
      await _repo.runnerResetMemory(r.id);
      if (!mounted) return;
      setState(() => _modelWorkEnabled = false);
      messenger.showSnackBar(
        const SnackBar(
          content: Text(
            'Память сброшена, соединение обновится при следующем обращении',
          ),
        ),
      );
      widget.onAfterOperation();
    } catch (e) {
      if (!mounted) return;
      messenger.showSnackBar(
        SnackBar(
          content: Text(
            userSafeErrorMessage(
              e,
              fallback: 'Не удалось сбросить память',
            ),
          ),
        ),
      );
    } finally {
      if (mounted) setState(() => _memoryResetBusy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final r = widget.runner;
    final canResetMemory = r.enabled && _modelWorkEnabled && !_memoryResetBusy;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 12),
        const Divider(height: 1),
        const SizedBox(height: 8),
        Text(
          'Управление',
          style: theme.textTheme.titleSmall?.copyWith(
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: 8),
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('Подключение к раннеру'),
          subtitle: Text(
            'Выключите, чтобы сервер перестал использовать этот раннер.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
          value: r.enabled,
          onChanged: _memoryResetBusy ? null : (v) => _onConnectionChanged(v),
        ),
        const SizedBox(height: 4),
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('Загрузка и работа модели'),
          subtitle: Text(
            'Выключите, чтобы выгрузить модель и освободить память GPU.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
          value: _modelWorkEnabled,
          onChanged: !r.enabled || _modelBusy || _memoryResetBusy
              ? null
              : (v) => _onModelWorkChanged(v),
        ),
        Align(
          alignment: Alignment.centerLeft,
          child: Text(
            'Сброс памяти раннера',
            style: theme.textTheme.titleSmall?.copyWith(
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
        const SizedBox(height: 4),
        Text(
          canResetMemory
            ? 'Выгрузка модели на раннере и сброс кэша соединения'
            : !r.enabled
              ? 'Сначала включите подключение к раннеру.'
              : 'Доступно при включённой «Загрузка и работа модели».',
          style: theme.textTheme.bodySmall?.copyWith(
            color: theme.colorScheme.onSurfaceVariant,
          ),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: canResetMemory ? _resetRunnerMemory : null,
          icon: _memoryResetBusy
              ? SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: theme.colorScheme.primary,
                  ),
                )
              : const Icon(Icons.restart_alt_outlined),
          label: Text(_memoryResetBusy ? 'Сброс…' : 'Сбросить память'),
        ),
        const SizedBox(height: 8),
        Text(
          'Модель',
          style: theme.textTheme.titleSmall?.copyWith(
            fontWeight: FontWeight.w600,
          ),
        ),
        const SizedBox(height: 8),
        if (!r.enabled)
          Text(
            'Включите подключение к раннеру, чтобы работать с моделями.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          )
        else if (!_modelWorkEnabled)
          Text(
            'Включите «Загрузка и работа модели», чтобы выбрать и загрузить.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          )
        else if (_loadingModels)
          const Row(
            children: [
              SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
              SizedBox(width: 12),
              Text('Загрузка списка…'),
            ],
          )
        else if (_modelsError != null)
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                _modelsError!,
                style: TextStyle(
                  color: theme.colorScheme.error,
                  fontSize: 13,
                ),
              ),
              TextButton(
                onPressed: _loadingModels ? null : _loadModels,
                child: const Text('Повторить'),
              ),
            ],
          )
        else if (_models != null) ...[
          if (_models!.isEmpty)
            Text(
              'Нет доступных моделей на раннере.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            )
          else ...[
            InputDecorator(
              decoration: const InputDecoration(
                labelText: 'Модель',
                border: OutlineInputBorder(),
              ),
              child: DropdownButtonHideUnderline(
                child: DropdownButton<String>(
                  isExpanded: true,
                  value: _selectedModel,
                  hint: const Text('Нет моделей'),
                  items: [
                    for (final m in _models!)
                      DropdownMenuItem<String>(value: m, child: Text(m)),
                  ],
                  onChanged: _modelBusy
                      ? null
                      : (v) => setState(() => _selectedModel = v),
                ),
              ),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                FilledButton.tonal(
                  onPressed: _modelBusy ? null : _applyLoadModel,
                  child: const Text('Загрузить'),
                ),
                const SizedBox(width: 8),
                OutlinedButton(
                  onPressed: _modelBusy ? null : _applyUnloadModel,
                  child: const Text('Выгрузить'),
                ),
              ],
            ),
          ],
        ],
      ],
    );
  }
}
