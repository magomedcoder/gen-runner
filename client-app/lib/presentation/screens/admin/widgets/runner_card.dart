import 'package:flutter/material.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_card_controls_section.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_card_header.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_gpu_section.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_loaded_model_section.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_server_info_section.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_status.dart';

class RunnerCard extends StatelessWidget {
  final RunnerInfo runner;
  final VoidCallback? onRefresh;
  final VoidCallback? onSetAsDefault;
  final VoidCallback? onEdit;
  final VoidCallback? onDelete;
  final bool showAdminControls;
  final void Function(bool enabled)? onRunnerEnabledChanged;
  final VoidCallback? onAdminOperationDone;

  const RunnerCard({
    super.key,
    required this.runner,
    this.onRefresh,
    this.onSetAsDefault,
    this.onEdit,
    this.onDelete,
    this.showAdminControls = false,
    this.onRunnerEnabledChanged,
    this.onAdminOperationDone,
  });

  @override
  Widget build(BuildContext context) {
    final status = runnerStatusFromRunner(runner);
    final hasServerInfo = runner.serverInfo != null;
    final hasGpus = runner.gpus.isNotEmpty;
    final hasLoadedModel = runner.loadedModel != null;

    return Card(
      clipBehavior: Clip.antiAlias,
      elevation: 1,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      margin: EdgeInsets.zero,
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            RunnerCardHeader(
              runner: runner,
              status: status,
              onRefresh: onRefresh,
              onSetAsDefault: onSetAsDefault,
              onEdit: onEdit,
              onDelete: onDelete,
            ),
            if (showAdminControls &&
                onRunnerEnabledChanged != null &&
                onAdminOperationDone != null) ...[
              RunnerCardControlsSection(
                key: ValueKey('runner_controls_${runner.id}'),
                runner: runner,
                onRunnerEnabledChanged: onRunnerEnabledChanged!,
                onAfterOperation: onAdminOperationDone!,
              ),
            ],
            if (hasLoadedModel) ...[
              const SizedBox(height: 12),
              const Divider(height: 1),
              const SizedBox(height: 8),
              RunnerLoadedModelSection(status: runner.loadedModel!),
            ],
            if (hasServerInfo) ...[
              const SizedBox(height: 12),
              const Divider(height: 1),
              const SizedBox(height: 8),
              RunnerServerInfoSection(serverInfo: runner.serverInfo!),
            ],
            if (hasGpus) ...[
              const SizedBox(height: 12),
              const Divider(height: 1),
              const SizedBox(height: 8),
              RunnerGpuSection(gpus: runner.gpus),
            ],
          ],
        ),
      ),
    );
  }
}
