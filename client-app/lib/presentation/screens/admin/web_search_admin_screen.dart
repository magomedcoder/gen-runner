import 'package:flutter/material.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/domain/entities/web_search_settings.dart';
import 'package:gen/domain/repositories/runners_repository.dart';

class WebSearchAdminScreen extends StatefulWidget {
  const WebSearchAdminScreen({super.key});

  @override
  State<WebSearchAdminScreen> createState() => _WebSearchAdminScreenState();
}

class _WebSearchAdminScreenState extends State<WebSearchAdminScreen> {
  final _repo = di.sl<RunnersRepository>();

  bool _loading = true;
  String? _loadError;
  bool _saving = false;

  bool _enabled = false;
  bool _yandexEnabled = false;
  bool _googleEnabled = false;
  bool _braveEnabled = false;
  final _maxResultsCtrl = TextEditingController();
  final _braveCtrl = TextEditingController();
  final _googleKeyCtrl = TextEditingController();
  final _googleCxCtrl = TextEditingController();
  final _yandexUserCtrl = TextEditingController();
  final _yandexKeyCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _maxResultsCtrl.dispose();
    _braveCtrl.dispose();
    _googleKeyCtrl.dispose();
    _googleCxCtrl.dispose();
    _yandexUserCtrl.dispose();
    _yandexKeyCtrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadError = null;
    });
    try {
      final s = await _repo.getWebSearchSettings();
      if (!mounted) {
        return;
      }
      setState(() {
        _enabled = s.enabled;
        _yandexEnabled = s.yandexEnabled;
        _googleEnabled = s.googleEnabled;
        _braveEnabled = s.braveEnabled;
        _maxResultsCtrl.text = s.maxResults > 0 ? '${s.maxResults}' : '20';
        _braveCtrl.text = s.braveApiKey;
        _googleKeyCtrl.text = s.googleApiKey;
        _googleCxCtrl.text = s.googleSearchEngineId;
        _yandexUserCtrl.text = s.yandexUser;
        _yandexKeyCtrl.text = s.yandexKey;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _loadError = userSafeErrorMessage(
          e,
          fallback: 'Не удалось загрузить настройки',
        );
        _loading = false;
      });
    }
  }

  Future<void> _save() async {
    final maxRaw = _maxResultsCtrl.text.trim();
    final maxParsed = int.tryParse(maxRaw);
    if (maxParsed == null || maxParsed <= 0) {
      showAppTopNotice('Укажите целое число результатов (1-50)', error: true);
      return;
    }

    setState(() => _saving = true);
    try {
      await _repo.updateWebSearchSettings(
        WebSearchSettingsEntity(
          enabled: _enabled,
          maxResults: maxParsed,
          braveApiKey: _braveCtrl.text,
          googleApiKey: _googleKeyCtrl.text,
          googleSearchEngineId: _googleCxCtrl.text,
          yandexUser: _yandexUserCtrl.text,
          yandexKey: _yandexKeyCtrl.text,
          yandexEnabled: _yandexEnabled,
          googleEnabled: _googleEnabled,
          braveEnabled: _braveEnabled,
        ),
      );

      if (!mounted) {
        return;
      }
      showAppTopNotice('Настройки поиска сохранены');
    } catch (e) {
      if (!mounted) {
        return;
      }

      showAppTopNotice('Ошибка: $e', error: true);
    } finally {
      if (mounted) {
        setState(() => _saving = false);
      }
    }
  }

  Widget _sectionCard({
    String? title,
    String? subtitle,
    required List<Widget> children,
  }) {
    final theme = Theme.of(context);
    final hasTitle = title != null && title.isNotEmpty;
    final hasSubtitle = subtitle != null && subtitle.isNotEmpty;

    return Card(
      clipBehavior: Clip.antiAlias,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null && title.isNotEmpty)
              Text(
                title,
                style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
            if (hasTitle && hasSubtitle) const SizedBox(height: 6),
            if (subtitle != null && subtitle.isNotEmpty)
              Text(
                subtitle,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            if (hasTitle || hasSubtitle) const SizedBox(height: 12),
            ...children,
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(
        body: Center(child: CircularProgressIndicator()),
      );
    }

    if (_loadError != null) {
      return Scaffold(
        appBar: AppBar(title: const Text('Поиск')),
        body: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  'Не удалось загрузить настройки:\n$_loadError',
                  textAlign: TextAlign.center,
                ),
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
        title: const Text('Поиск'),
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          _sectionCard(
            children: [
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Веб-поиск'),
                subtitle: const Text(
                  'Полностью отключает поиск для всех пользователей. Ниже можно отключить отдельные провайдеры.',
                ),
                value: _enabled,
                onChanged: (v) => setState(() => _enabled = v),
              ),
              TextField(
                controller: _maxResultsCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(
                  labelText: 'Максимальное количество результатов на запрос',
                  border: OutlineInputBorder(),
                  helperText: 'Не более 50',
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _sectionCard(
            title: 'Яндекс поиск',
            children: [
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Использовать Яндекс'),
                value: _yandexEnabled,
                onChanged: (v) => setState(() => _yandexEnabled = v),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _yandexUserCtrl,
                decoration: const InputDecoration(
                  labelText: 'Пользователь',
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _yandexKeyCtrl,
                decoration: const InputDecoration(
                  labelText: 'Ключ',
                  border: OutlineInputBorder(),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _sectionCard(
            title: 'Google поиск',
            children: [
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Использовать Google'),
                value: _googleEnabled,
                onChanged: (v) => setState(() => _googleEnabled = v),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _googleKeyCtrl,
                decoration: const InputDecoration(
                  labelText: 'Ключ',
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _googleCxCtrl,
                decoration: const InputDecoration(
                  labelText: 'Идентификатор поисковой системы',
                  border: OutlineInputBorder(),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _sectionCard(
            title: 'Brave поиск',
            children: [
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Использовать Brave'),
                value: _braveEnabled,
                onChanged: (v) => setState(() => _braveEnabled = v),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _braveCtrl,
                decoration: const InputDecoration(
                  labelText: 'Ключ',
                  border: OutlineInputBorder(),
                ),
              ),
            ],
          ),
          const SizedBox(height: 28),
          FilledButton(
            onPressed: _saving ? null : _save,
            child: _saving
              ? const SizedBox(
                height: 22,
                width: 22,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
              : const Text('Сохранить'),
          ),
        ],
      ),
    );
  }
}
