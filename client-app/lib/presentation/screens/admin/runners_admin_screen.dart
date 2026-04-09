import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_bloc.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_event.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_state.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_admin_form_dialog.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_card.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_status.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';

class RunnersAdminScreen extends StatefulWidget {
  const RunnersAdminScreen({super.key});

  @override
  State<RunnersAdminScreen> createState() => _RunnersAdminScreenState();
}

class _RunnersAdminScreenState extends State<RunnersAdminScreen> {
  late final RunnersAdminBloc _bloc;
  bool _runnerSheetOpen = false;

  BuildContext? _runnerSheetSnackContext;

  @override
  void initState() {
    super.initState();
    _bloc = di.sl<RunnersAdminBloc>()..add(const RunnersAdminLoadRequested());
  }

  String? _effectiveDefaultAddress(RunnersAdminState s) {
    final enabled = s.runners.where((r) => r.enabled && r.address.isNotEmpty).toList();
    if (enabled.isEmpty) {
      return null;
    }

    final saved = s.defaultRunner;
    if (saved != null && enabled.any((r) => r.address == saved)) {
      return saved;
    }

    final sorted = enabled.map((r) => r.address).toList()..sort();
    return sorted.first;
  }

  Future<void> _openCreateDialog(BuildContext context) async {
    final v = await showRunnerAdminFormDialog(context);
    if (!context.mounted || v == null) {
      return;
    }

    _bloc.add(
      RunnersAdminCreateRequested(
        name: v.name,
        host: v.host,
        port: v.port,
        enabled: v.enabled,
        selectedModel: '',
      ),
    );
  }

  Future<void> _openEditDialog(BuildContext context, RunnerInfo runner) async {
    final v = await showRunnerAdminFormDialog(context, existing: runner);
    if (!context.mounted || v == null) {
      return;
    }

    _bloc.add(
      RunnersAdminUpdateRequested(
        id: runner.id,
        name: v.name,
        host: v.host,
        port: v.port,
        enabled: v.enabled,
        selectedModel: runner.selectedModel,
      ),
    );
  }

  Future<void> _confirmDelete(BuildContext context, RunnerInfo runner) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Удалить раннер?'),
        content: Text(runner.name.isNotEmpty ? runner.name : runner.address),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Отмена')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Удалить')),
        ],
      ),
    );
    if (ok == true && context.mounted) {
      _bloc.add(RunnersAdminDeleteRequested(runner.id));
    }
  }

  @override
  void dispose() {
    _bloc.close();
    super.dispose();
  }

  bool _isAdmin(AuthState authState) {
    return authState.isAuthenticated && (authState.user?.isAdmin ?? false);
  }

  String _runnerDisplayName(RunnerInfo r) {
    return r.name.trim().isNotEmpty ? r.name.trim() : r.address;
  }

  String _runnerHostPort(RunnerInfo r) {
    if (r.address.trim().isNotEmpty) return r.address.trim();
    if (r.host.isEmpty) return '-';
    if (r.port > 0) return '${r.host}:${r.port}';
    return r.host;
  }

  String _runnerModelCell(RunnerInfo r) {
    final lm = r.loadedModel;
    if (lm == null) return '-';
    if (!lm.loaded) return 'Нет';
    final n = lm.displayName.trim().isNotEmpty
        ? lm.displayName.trim()
        : lm.ggufBasename.trim();
    return n.isNotEmpty ? n : '-';
  }

  void _showRunnerCardSheet(
    BuildContext context, {
    required RunnerInfo runner,
    required bool isDefaultRunner,
    required bool isAdminUser,
  }) {
    _runnerSheetOpen = true;
    _runnerSheetSnackContext = null;
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (sheetContext) {
        return Scaffold(
          backgroundColor: Colors.transparent,
          body: Builder(
            builder: (scaffoldContext) {
              WidgetsBinding.instance.addPostFrameCallback((_) {
                if (!mounted || !_runnerSheetOpen) return;
                _runnerSheetSnackContext = scaffoldContext;
              });
              return BlocBuilder<RunnersAdminBloc, RunnersAdminState>(
                bloc: _bloc,
                buildWhen: (prev, next) =>
                    prev.runners != next.runners || prev.isLoading != next.isLoading,
                builder: (context, state) {
                  var r = runner;
                  for (final x in state.runners) {
                    if (x.id == runner.id) {
                      r = x;
                      break;
                    }
                  }
                  return SafeArea(
                    child: SingleChildScrollView(
                      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
                      child: RunnerCard(
                        key: ValueKey('runner_sheet_${r.id}'),
                        runner: r,
                        onRefresh: () => _bloc.add(const RunnersAdminLoadRequested()),
                        showAdminControls: isAdminUser,
                        onRunnerEnabledChanged: isAdminUser
                            ? (enabled) {
                                _bloc.add(
                                  RunnersAdminUpdateRequested(
                                    id: r.id,
                                    name: r.name,
                                    host: r.host,
                                    port: r.port,
                                    enabled: enabled,
                                    selectedModel: r.selectedModel,
                                  ),
                                );
                              }
                            : null,
                        onAdminOperationDone: isAdminUser
                            ? () => _bloc.add(const RunnersAdminLoadRequested())
                            : null,
                        onSetAsDefault: isAdminUser &&
                                r.enabled &&
                                r.address.isNotEmpty &&
                                !isDefaultRunner
                            ? () {
                                _bloc.add(
                                  RunnersAdminDefaultRunnerChanged(r.address),
                                );
                                Navigator.pop(sheetContext);
                              }
                            : null,
                        onEdit: isAdminUser
                            ? () => _openEditDialog(context, r)
                            : null,
                        onDelete:
                            isAdminUser ? () => _confirmDelete(context, r) : null,
                      ),
                    ),
                  );
                },
              );
            },
          ),
        );
      },
    ).whenComplete(() {
      _runnerSheetSnackContext = null;
      _runnerSheetOpen = false;
      if (mounted) setState(() {});
    });
  }

  void _presentRunnersBlocError(String msg) {
    final sheetCtx = _runnerSheetSnackContext;
    if (_runnerSheetOpen && sheetCtx != null && sheetCtx.mounted) {
      final scheme = Theme.of(sheetCtx).colorScheme;
      final messenger = ScaffoldMessenger.of(sheetCtx);
      messenger.clearSnackBars();
      messenger.showSnackBar(
        SnackBar(
          content: Text(msg),
          behavior: SnackBarBehavior.floating,
          margin: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          backgroundColor: scheme.errorContainer,
          action: SnackBarAction(
            label: 'OK',
            textColor: scheme.onErrorContainer,
            onPressed: messenger.hideCurrentSnackBar,
          ),
        ),
      );
    } else {
      showAppTopNotice(msg, error: true);
    }
    _bloc.add(const RunnersAdminClearError());
  }

  static const double _wideTableBreakpoint = 560;

  Widget _buildRunnersTable({
    required BuildContext context,
    required RunnersAdminState runnersState,
    required bool isAdminUser,
    required double bottomInset,
  }) {
    final theme = Theme.of(context);
    final runners = runnersState.runners;
    final defAddr = _effectiveDefaultAddress(runnersState);

    final headerStyle = theme.textTheme.labelLarge?.copyWith(
      fontWeight: FontWeight.w600,
      color: theme.colorScheme.onSurface,
    );

    Widget cell(String text, {int flex = 2, bool mono = false, TextStyle? style}) {
      return Expanded(
        flex: flex,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 10),
          child: Text(
            text,
            style: style ??
              (mono ? theme.textTheme.bodySmall : theme.textTheme.bodyMedium)?.copyWith(fontFamily: mono ? 'monospace' : null),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      );
    }

    void openSheet(RunnerInfo runner) {
      final isDefault = runner.enabled && runner.address.isNotEmpty && defAddr == runner.address;
      _showRunnerCardSheet(
        context,
        runner: runner,
        isDefaultRunner: isDefault,
        isAdminUser: isAdminUser,
      );
    }

    return SizedBox.expand(
      child: LayoutBuilder(
        builder: (context, constraints) {
          var h = constraints.maxHeight;
          if (!h.isFinite || h <= 0) {
            final mq = MediaQuery.of(context);
            h = mq.size.height - mq.padding.vertical;
          }

          if (h <= 0) {
            return const SizedBox.shrink();
          }

          final maxW = constraints.maxWidth;
          final horizontalPad = maxW < _wideTableBreakpoint ? 10.0 : 16.0;
          final useWideTable = maxW >= _wideTableBreakpoint;

          if (!useWideTable) {
            return Padding(
              padding: EdgeInsets.fromLTRB(horizontalPad, 0, horizontalPad, bottomInset + 12),
              child: ListView.separated(
                padding: EdgeInsets.zero,
                primary: false,
                physics: const ClampingScrollPhysics(),
                itemCount: runners.length,
                separatorBuilder: (context, _) => Divider(
                  height: 1,
                  color: theme.dividerColor.withValues(alpha: 0.45),
                ),
                itemBuilder: (context, i) {
                  final runner = runners[i];
                  final status = runnerStatusFromRunner(runner);
                  final statusColor = runnerStatusColor(context, status);
                  return Material(
                    color: i.isOdd
                      ? theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35)
                      : Colors.transparent,
                    borderRadius: BorderRadius.circular(10),
                    clipBehavior: Clip.antiAlias,
                    child: InkWell(
                      onTap: () => openSheet(runner),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              _runnerDisplayName(runner),
                              style: theme.textTheme.titleSmall?.copyWith(
                                fontWeight: FontWeight.w600,
                              ),
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                            ),
                            const SizedBox(height: 4),
                            Text(
                              _runnerHostPort(runner),
                              style: theme.textTheme.bodySmall?.copyWith(
                                fontFamily: 'monospace',
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                            const SizedBox(height: 8),
                            Wrap(
                              spacing: 8,
                              runSpacing: 6,
                              crossAxisAlignment: WrapCrossAlignment.center,
                              children: [
                                Text(
                                  status.label,
                                  style: theme.textTheme.bodySmall?.copyWith(
                                    color: statusColor,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                                Text(
                                  _runnerModelCell(runner),
                                  style: theme.textTheme.bodySmall?.copyWith(
                                    color: theme.colorScheme.onSurfaceVariant,
                                  ),
                                ),
                              ],
                            ),
                          ],
                        ),
                      ),
                    ),
                  );
                },
              ),
            );
          }

          return Padding(
            padding: EdgeInsets.fromLTRB(horizontalPad, 0, horizontalPad, bottomInset + 16),
            child: SizedBox(
              width: maxW,
              height: h,
              child: ListView(
                padding: EdgeInsets.zero,
                primary: false,
                physics: const ClampingScrollPhysics(),
                children: [
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(
                        flex: 22,
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 10),
                          child: Text('Имя', style: headerStyle),
                        ),
                      ),
                      Expanded(
                        flex: 24,
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 10),
                          child: Text('Хост:порт', style: headerStyle),
                        ),
                      ),
                      Expanded(
                        flex: 20,
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 10),
                          child: Text('Статус', style: headerStyle),
                        ),
                      ),
                      Expanded(
                        flex: 22,
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 10),
                          child: Text('Модель', style: headerStyle),
                        ),
                      ),
                    ],
                  ),
                  Divider(height: 1, color: theme.dividerColor),
                  for (var i = 0; i < runners.length; i++) ...[
                    () {
                      final runner = runners[i];
                      final status = runnerStatusFromRunner(runner);
                      return Material(
                        color: i.isOdd
                          ? theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35)
                          : Colors.transparent,
                        child: InkWell(
                          onTap: () => openSheet(runner),
                          child: Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              cell(_runnerDisplayName(runner), flex: 22),
                              cell(_runnerHostPort(runner), mono: true, flex: 24),
                              cell(
                                status.label,
                                flex: 20,
                                style: theme.textTheme.bodyMedium?.copyWith(
                                  color: runnerStatusColor(context, status),
                                  fontWeight: FontWeight.w500,
                                ),
                              ),
                              cell(_runnerModelCell(runner), flex: 22),
                            ],
                          ),
                        ),
                      );
                    }(),
                    if (i < runners.length - 1)
                      Divider(
                        height: 1,
                        indent: 8,
                        endIndent: 8,
                        color: theme.dividerColor.withValues(alpha: 0.5),
                      ),
                  ],
                ],
              ),
            ),
          );
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return BlocProvider.value(
      value: _bloc,
      child: BlocBuilder<AuthBloc, AuthState>(
        builder: (context, authState) {
          final isAdminUser = _isAdmin(authState);
          return BlocConsumer<RunnersAdminBloc, RunnersAdminState>(
            listener: (context, state) {
              if (state.error == null) return;
              _presentRunnersBlocError(state.error!);
            },
            builder: (context, runnersState) {
              return Scaffold(
                appBar: AppBar(
                  title: const Text('Раннеры'),
                  actions: [
                    if (isAdminUser) ...[
                      IconButton(
                        icon: const Icon(Icons.add),
                        onPressed: () => _openCreateDialog(context),
                        tooltip: 'Добавить раннер',
                      ),
                      IconButton(
                        icon: const Icon(Icons.refresh),
                        onPressed: () {
                          _bloc.add(const RunnersAdminLoadRequested());
                        },
                        tooltip: 'Обновить',
                      ),
                    ],
                  ],
                ),
                body: runnersState.isLoading && runnersState.runners.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : runnersState.runners.isEmpty
                    ? Center(
                        child: Padding(
                          padding: const EdgeInsets.all(24),
                          child: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Icon(
                                Icons.cloud_off_outlined,
                                size: 56,
                                color: Theme.of(context).colorScheme.outline,
                              ),
                              const SizedBox(height: 16),
                              Text(
                                'Нет раннеров',
                                style: Theme.of(context).textTheme.titleMedium,
                                textAlign: TextAlign.center,
                              ),
                            ],
                          ),
                        ),
                      )
                    : _buildRunnersTable(
                        context: context,
                        runnersState: runnersState,
                        isAdminUser: isAdminUser,
                        bottomInset: MediaQuery.paddingOf(context).bottom + 16,
                      ),
              );
            },
          );
        },
      ),
    );
  }
}
