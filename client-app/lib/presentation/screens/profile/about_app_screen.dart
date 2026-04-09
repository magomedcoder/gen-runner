import 'package:flutter/material.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/widgets/app_logs_dialog.dart';
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:url_launcher/url_launcher.dart';

abstract final class GenProjectLinks {
  static const genServer = 'https://github.com/magomedcoder/gen';
  static const genRunner = 'https://github.com/magomedcoder/gen-runner';
}

class AboutAppScreen extends StatelessWidget {
  const AboutAppScreen({super.key});

  Future<void> _openUrl(BuildContext context, String url) async {
    final uri = Uri.parse(url);
    final ok = await launchUrl(uri, mode: LaunchMode.externalApplication);
    if (!ok && context.mounted) {
      showAppTopNotice('Не удалось открыть ссылку', error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final horizontal = Breakpoints.isMobile(context) ? 16.0 : 24.0;
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final linkStyle = textTheme.bodyMedium!.copyWith(
      color: scheme.onSurfaceVariant,
      decoration: TextDecoration.underline,
      decorationColor: scheme.primary,
    );

    return Scaffold(
      appBar: AppBar(title: const Text('О приложении')),
      body: SafeArea(
        top: false,
        child: LayoutBuilder(
          builder: (context, constraints) {
            return SingleChildScrollView(
              padding: EdgeInsets.fromLTRB(horizontal, 20, horizontal, 24),
              child: ConstrainedBox(
                constraints: BoxConstraints(minHeight: constraints.maxHeight),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 440),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        Tooltip(
                          message: 'Показать журнал приложения',
                          child: InkWell(
                            onTap: () => showAppLogsDialog(context),
                            borderRadius: BorderRadius.circular(10),
                            child: Padding(
                              padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.center,
                                children: [
                                  Text(
                                    'Gen',
                                    textAlign: TextAlign.center,
                                    style: textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
                                  ),
                                  const SizedBox(height: 6),
                                  Text(
                                    'Версия 1.0.0',
                                    textAlign: TextAlign.center,
                                    style: textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant),
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ),
                        const SizedBox(height: 24),
                        Text(
                          'Проект с открытым исходным кодом',
                          textAlign: TextAlign.center,
                          style: textTheme.bodyMedium,
                        ),
                        const SizedBox(height: 16),
                        Text(
                          'Репозиторий Gen',
                          textAlign: TextAlign.center,
                        ),
                        const SizedBox(height: 4),
                        _ClickableUrl(
                          url: GenProjectLinks.genServer,
                          style: linkStyle,
                          onTap: () => _openUrl(context, GenProjectLinks.genServer),
                        ),
                        const SizedBox(height: 16),
                        Text(
                          'gen-runner - запуск и работа с LLM',
                          textAlign: TextAlign.center,
                        ),
                        const SizedBox(height: 4),
                        _ClickableUrl(
                          url: GenProjectLinks.genRunner,
                          style: linkStyle,
                          onTap: () => _openUrl(context, GenProjectLinks.genRunner),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }
}

class _ClickableUrl extends StatelessWidget {
  const _ClickableUrl({
    required this.url,
    required this.style,
    required this.onTap,
  });

  final String url;
  final TextStyle style;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(4),
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: Text(
            url,
            style: style,
            textAlign: TextAlign.center,
          ),
        ),
      ),
    );
  }
}
