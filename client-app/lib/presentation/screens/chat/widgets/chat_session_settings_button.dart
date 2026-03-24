import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/domain/entities/chat_session_settings.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class ChatSessionSettingsButton extends StatelessWidget {
  const ChatSessionSettingsButton({super.key, required this.state});

  final ChatState state;

  static double _creativityFromTemperature(double? temperature) {
    final t = (temperature ?? 0.8).clamp(0.0, 2.0);
    return t / 2.0;
  }

  static double _responseLengthFromTokens(int? maxTokens) {
    final tokens = (maxTokens ?? 512).clamp(64, 4096);
    return (tokens - 64) / (4096 - 64);
  }

  static double _temperatureFromCreativity(double creativity) {
    return creativity.clamp(0.0, 1.0) * 2.0;
  }

  static int _maxTokensFromResponseLength(double length) {
    final raw = 64 + (length.clamp(0.0, 1.0) * (4096 - 64));
    return raw.round();
  }

  static const Map<String, String> _profileTitles = {
    'code': 'Код',
    'analytics': 'Аналитика',
    'docs': 'Документация',
    'translate': 'Перевод',
  };

  Widget _settingsSection(
    BuildContext context, {
    required String title,
    required Widget child,
  }) {
    final theme = Theme.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: theme.colorScheme.inversePrimary.withValues(alpha: 0.16),
        border: Border.all(color: theme.colorScheme.outlineVariant),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 10),
          child,
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final disabled = state.currentSessionId == null || state.isStreaming;
    return IconButton(
      tooltip: 'Настройки чата',
      onPressed: disabled ? null : () => _openSettings(context),
      icon: const Icon(Icons.tune_rounded, size: 20),
    );
  }

  void _openSettings(BuildContext context) {
    final current = state.sessionSettings ?? ChatSessionSettings(sessionId: state.currentSessionId ?? 0);
    final promptController = TextEditingController(text: current.systemPrompt);
    final stopController = TextEditingController(text: current.stopSequences.join('\n'),);
    final timeoutController = TextEditingController(text: current.timeoutSeconds > 0 ? current.timeoutSeconds.toString() : '');
    final temperatureController = TextEditingController(text: current.temperature?.toString() ?? '');
    final maxTokensController = TextEditingController(text: current.maxTokens?.toString() ?? '');
    final topKController = TextEditingController(text: current.topK?.toString() ?? '');
    final topPController = TextEditingController(text: current.topP?.toString() ?? '');
    final jsonSchemaController = TextEditingController(text: current.jsonSchema);
    final toolsJsonController = TextEditingController(text: current.toolsJson);
    var jsonMode = current.jsonMode;
    var expertMode = false;
    var selectedProfile = current.profile;
    var creativity = _creativityFromTemperature(current.temperature);
    var responseLength = _responseLengthFromTokens(current.maxTokens);

    void applyProfile(String profileKey) {
      selectedProfile = profileKey;
      switch (profileKey) {
        case 'code':
          promptController.text = 'Ты опытный инженер-программист. Давай короткие и точные ответы, показывай рабочие примеры кода, отмечай риски и edge cases.';
          timeoutController.text = '120';
          temperatureController.text = '0.2';
          maxTokensController.text = '1400';
          topPController.text = '0.9';
          topKController.text = '40';
          break;
        case 'analytics':
          promptController.text = 'Ты аналитик данных. Структурируй вывод: предпосылки, расчеты, выводы, ограничения. Используй списки и таблицы, где уместно.';
          timeoutController.text = '120';
          temperatureController.text = '0.3';
          maxTokensController.text = '1200';
          topPController.text = '0.9';
          topKController.text = '40';
          break;
        case 'docs':
          promptController.text = 'Ты технический писатель. Пиши ясно и последовательно, с заголовками, шагами и примерами. Сохраняй единый стиль.';
          timeoutController.text = '90';
          temperatureController.text = '0.4';
          maxTokensController.text = '1100';
          topPController.text = '0.92';
          topKController.text = '40';
          break;
        case 'translate':
          promptController.text = 'Ты профессиональный переводчик. Сохраняй смысл, терминологию и стиль оригинала. При неоднозначности предложи варианты.';
          timeoutController.text = '90';
          temperatureController.text = '0.2';
          maxTokensController.text = '900';
          topPController.text = '0.9';
          topKController.text = '40';
          break;
      }
      final t = double.tryParse(temperatureController.text.trim());
      final mt = int.tryParse(maxTokensController.text.trim());
      creativity = _creativityFromTemperature(t);
      responseLength = _responseLengthFromTokens(mt);
    }

    showDialog<void>(
      context: context,
      builder: (ctx) {
        return AlertDialog(
          title: const Text('Настройки текущего чата'),
          content: SizedBox(
            width: 760,
            child: SingleChildScrollView(
              child: StatefulBuilder(
                builder: (ctx, setStateDialog) => Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    _settingsSection(
                      ctx,
                      title: 'Режим настроек',
                      child: Row(
                        children: [
                          ChoiceChip(
                            label: const Text('Обычные'),
                            selected: !expertMode,
                            onSelected: (_) => setStateDialog(() => expertMode = false),
                          ),
                          const SizedBox(width: 8),
                          ChoiceChip(
                            label: const Text('Эксперт'),
                            selected: expertMode,
                            onSelected: (_) => setStateDialog(() => expertMode = true),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 12),
                    _settingsSection(
                      ctx,
                      title: 'Профиль чата',
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: _profileTitles.entries.map((entry) {
                              return ChoiceChip(
                                label: Text(entry.value),
                                selected: selectedProfile == entry.key,
                                onSelected: (_) => setStateDialog(() {
                                  applyProfile(entry.key);
                                }),
                              );
                            }).toList(),
                          ),
                          const SizedBox(height: 8),
                          const Text('Профиль подставляет готовые промпт и параметры'),
                        ],
                      ),
                    ),
                    const SizedBox(height: 12),
                    _settingsSection(
                      ctx,
                      title: 'Системный промпт',
                      child: TextField(
                        controller: promptController,
                        maxLines: 6,
                        decoration: const InputDecoration(
                          labelText: 'Системный промпт',
                          helperText: 'Инструкции для модели в рамках этого чата. Применяются ко всем следующим ответам',
                          helperMaxLines: 3,
                          border: OutlineInputBorder(),
                        ),
                      ),
                    ),
                    const SizedBox(height: 12),
                    _settingsSection(
                      ctx,
                      title: 'Таймаут',
                      child: TextField(
                        controller: timeoutController,
                        keyboardType: TextInputType.number,
                        decoration: const InputDecoration(
                          labelText: 'Таймаут (секунды)',
                          helperText: 'Максимальное время ожидания ответа. 0 - без дополнительного ограничения',
                          helperMaxLines: 3,
                          border: OutlineInputBorder(),
                        ),
                      ),
                    ),
                    const SizedBox(height: 12),
                    if (!expertMode) ...[
                      _settingsSection(
                        ctx,
                        title: 'Быстрая настройка ответа',
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text('Креативность: ${_temperatureFromCreativity(creativity).toStringAsFixed(2)}'),
                            Slider(
                              value: creativity,
                              onChanged: (v) => setStateDialog(() => creativity = v),
                            ),
                            const Text('Ниже - более предсказуемо, выше - более разнообразно',),
                            const SizedBox(height: 8),
                            Text('Максимальная длина ответа: ${_maxTokensFromResponseLength(responseLength)} токенов',),
                            Slider(
                              value: responseLength,
                              onChanged: (v) => setStateDialog(() => responseLength = v),
                            ),
                          ],
                        ),
                      ),
                    ] else ...[
                      _settingsSection(
                        ctx,
                        title: 'Экспертные параметры',
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            TextField(
                              controller: stopController,
                              maxLines: 4,
                              decoration: const InputDecoration(
                                labelText: 'Стоп-последовательности (каждая с новой строки)',
                                helperText: 'Если модель встретит одну из этих последовательностей, генерация будет остановлена',
                                helperMaxLines: 3,
                                border: OutlineInputBorder(),
                              ),
                            ),
                            const SizedBox(height: 12),
                            Row(
                              children: [
                                Expanded(
                                  child: TextField(
                                    controller: temperatureController,
                                    keyboardType: const TextInputType.numberWithOptions(decimal: true),
                                    decoration: const InputDecoration(
                                      labelText: 'Температура',
                                      helperText: 'Контроль случайности. Ниже - стабильнее, выше - креативнее',
                                      helperMaxLines: 3,
                                      border: OutlineInputBorder(),
                                    ),
                                  ),
                                ),
                                const SizedBox(width: 12),
                                Expanded(
                                  child: TextField(
                                    controller: topPController,
                                    keyboardType: const TextInputType.numberWithOptions(decimal: true),
                                    decoration: const InputDecoration(
                                      labelText: 'Top P',
                                      helperText: 'Ограничение по суммарной вероятности токенов (nucleus sampling)',
                                      helperMaxLines: 3,
                                      border: OutlineInputBorder(),
                                    ),
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 12),
                            Row(
                              children: [
                                Expanded(
                                  child: TextField(
                                    controller: topKController,
                                    keyboardType: TextInputType.number,
                                    decoration: const InputDecoration(
                                      labelText: 'Top K',
                                      helperText: 'Оставляет только K самых вероятных токенов на каждом шаге',
                                      helperMaxLines: 3,
                                      border: OutlineInputBorder(),
                                    ),
                                  ),
                                ),
                                const SizedBox(width: 12),
                                Expanded(
                                  child: TextField(
                                    controller: maxTokensController,
                                    keyboardType: TextInputType.number,
                                    decoration: const InputDecoration(
                                      labelText: 'Максимальное количество токенов',
                                      helperText: 'Максимальная длина ответа в токенах. Пусто - значение по умолчанию раннера',
                                      helperMaxLines: 3,
                                      border: OutlineInputBorder(),
                                    ),
                                  ),
                                ),
                              ],
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 12),
                      _settingsSection(
                        ctx,
                        title: 'Формат и инструменты',
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            SwitchListTile(
                              value: jsonMode,
                              contentPadding: EdgeInsets.zero,
                              title: const Text('Режим json'),
                              subtitle: const Text('Модель возвращать ответ в json-формате'),
                              onChanged: (v) => setStateDialog(() => jsonMode = v),
                            ),
                            if (jsonMode) ...[
                              const SizedBox(height: 8),
                              TextField(
                                controller: jsonSchemaController,
                                maxLines: 6,
                                decoration: const InputDecoration(
                                  labelText: 'json схема (опциональная)',
                                  helperText: 'Опциональная схема/грамматика JSON для более строгой структуры ответа',
                                  helperMaxLines: 3,
                                  border: OutlineInputBorder(),
                                ),
                              ),
                            ],
                            const SizedBox(height: 8),
                            TextField(
                              controller: toolsJsonController,
                              maxLines: 6,
                              decoration: const InputDecoration(
                                labelText: 'Инструменты json (опциональная)',
                                helperText: 'json-массив инструментов. Пример: [{"name":"search","description":"Поиск","parameters_json":"{\\"type\\":\\"object\\",\\"properties\\":{}}"}]',
                                helperMaxLines: 4,
                                border: OutlineInputBorder(),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(),
              child: const Text('Отмена'),
            ),
            FilledButton(
              onPressed: () {
                final timeout = int.tryParse(timeoutController.text.trim()) ?? 0;
                final temperature = expertMode
                  ? double.tryParse(temperatureController.text.trim())
                  : _temperatureFromCreativity(creativity);
                final maxTokens = expertMode
                  ? int.tryParse(maxTokensController.text.trim())
                  : _maxTokensFromResponseLength(responseLength);
                final topK = expertMode
                  ? int.tryParse(topKController.text.trim())
                  : current.topK;
                final topP = expertMode
                  ? double.tryParse(topPController.text.trim())
                  : current.topP;
                final stop = expertMode
                  ? stopController.text
                    .split('\n')
                    .map((e) => e.trim())
                    .where((e) => e.isNotEmpty)
                    .toList()
                  : current.stopSequences;
                context.read<ChatBloc>().add(
                  ChatUpdateSessionSettings(
                    systemPrompt: promptController.text.trim(),
                    stopSequences: stop,
                    timeoutSeconds: timeout,
                    temperature: temperature,
                    maxTokens: maxTokens,
                    topK: topK,
                    topP: topP,
                    jsonMode: jsonMode,
                    jsonSchema: jsonSchemaController.text.trim(),
                    toolsJson: toolsJsonController.text.trim(),
                    profile: selectedProfile,
                  ),
                );
                Navigator.of(ctx).pop();
              },
              child: const Text('Сохранить'),
            ),
          ],
        );
      },
    );
  }
}
