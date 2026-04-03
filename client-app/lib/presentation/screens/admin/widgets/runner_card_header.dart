import 'package:flutter/material.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_status.dart';

class RunnerCardHeader extends StatelessWidget {
  final RunnerInfo runner;
  final RunnerStatus status;
  final VoidCallback? onRefresh;
  final VoidCallback? onSetAsDefault;
  final VoidCallback? onEdit;
  final VoidCallback? onDelete;

  const RunnerCardHeader({
    super.key,
    required this.runner,
    required this.status,
    this.onRefresh,
    this.onSetAsDefault,
    this.onEdit,
    this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final statusColor = runnerStatusColor(context, status);
    final connectionLabel =
        status == RunnerStatus.connected ? 'Подключён' : 'Отключён';

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: _StatusIndicator(color: statusColor),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (runner.name.isNotEmpty)
                Text(
                  runner.name,
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.w600,
                    color: runner.enabled
                        ? theme.colorScheme.onSurface
                        : theme.colorScheme.onSurfaceVariant,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              Padding(
                padding: EdgeInsets.only(top: runner.name.isNotEmpty ? 2 : 0),
                child: Wrap(
                  crossAxisAlignment: WrapCrossAlignment.center,
                  spacing: 8,
                  runSpacing: 4,
                  children: [
                    Text(
                      runner.address,
                      style: runner.name.isEmpty
                          ? theme.textTheme.titleSmall?.copyWith(
                              fontWeight: FontWeight.w600,
                              fontFamily: 'monospace',
                              color: runner.enabled
                                  ? theme.colorScheme.onSurface
                                  : theme.colorScheme.onSurfaceVariant,
                            )
                          : theme.textTheme.bodySmall?.copyWith(
                              fontFamily: 'monospace',
                              color: theme.colorScheme.onSurfaceVariant,
                            ),
                    ),
                    Text(
                      connectionLabel,
                      style: theme.textTheme.labelMedium?.copyWith(
                        fontWeight: FontWeight.w600,
                        color: statusColor,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        if (onRefresh != null)
          IconButton(
            tooltip: 'Обновить',
            icon: const Icon(Icons.refresh),
            onPressed: onRefresh,
          ),
        if (onEdit != null)
          IconButton(
            tooltip: 'Изменить',
            icon: const Icon(Icons.edit_outlined),
            onPressed: onEdit,
          ),
        if (onDelete != null)
          IconButton(
            tooltip: 'Удалить',
            icon: Icon(
              Icons.delete_outline,
              color: theme.colorScheme.error,
            ),
            onPressed: onDelete,
          ),
        if (onSetAsDefault != null)
          IconButton(
            tooltip: 'Сделать раннером по умолчанию',
            icon: const Icon(Icons.star_outline),
            onPressed: onSetAsDefault,
          ),
      ],
    );
  }
}

class _StatusIndicator extends StatelessWidget {
  final Color color;

  const _StatusIndicator({required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 12,
      height: 12,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        boxShadow: [
          BoxShadow(
            color: color.withValues(alpha: 0.4),
            blurRadius: 6,
            spreadRadius: 0,
          ),
        ],
      ),
    );
  }
}
