import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/presentation/screens/admin/bloc/runners_admin_bloc.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_event.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_state.dart';
import 'package:gen/presentation/screens/admin/widgets/runner_card.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';

class RunnersAdminScreen extends StatefulWidget {
  const RunnersAdminScreen({super.key});

  @override
  State<RunnersAdminScreen> createState() => _RunnersAdminScreenState();
}

class _RunnersAdminScreenState extends State<RunnersAdminScreen> {
  late final RunnersAdminBloc _bloc;

  @override
  void initState() {
    super.initState();
    _bloc = di.sl<RunnersAdminBloc>()..add(const RunnersAdminLoadRequested());
  }

  @override
  void dispose() {
    _bloc.close();
    super.dispose();
  }

  bool _isAdmin(AuthState authState) {
    return authState.isAuthenticated && (authState.user?.isAdmin ?? false);
  }

  void _showAccessDenied() {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: const Text('Недостаточно прав'),
        backgroundColor: Theme.of(context).colorScheme.error,
        behavior: SnackBarBehavior.floating,
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
              if (state.error != null) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(
                    content: Text(state.error!),
                    backgroundColor: Theme.of(context).colorScheme.error,
                    behavior: SnackBarBehavior.floating,
                  ),
                );
                _bloc.add(const RunnersAdminClearError());
              }
            },
            builder: (context, runnersState) {
              return Scaffold(
                appBar: AppBar(
                  title: const Text('Раннеры'),
                  actions: [
                    if (isAdminUser)
                      IconButton(
                        icon: const Icon(Icons.refresh),
                        onPressed: () {
                          _bloc.add(const RunnersAdminLoadRequested());
                        },
                        tooltip: 'Обновить',
                      ),
                  ],
                ),
                body: Padding(
                  padding: const EdgeInsets.all(16),
                  child: runnersState.isLoading && runnersState.runners.isEmpty
                      ? const Center(child: CircularProgressIndicator())
                      : runnersState.runners.isEmpty
                          ? const Center(
                              child: Text('Нет зарегистрированных раннеров'),
                            )
                          : ListView.separated(
                              itemCount: runnersState.runners.length,
                              separatorBuilder: (_, _) =>
                                  const SizedBox(height: 8),
                              itemBuilder: (ctx, index) {
                                final runner = runnersState.runners[index];
                                return RunnerCard(
                                  runner: runner,
                                  onToggleEnabled: () {
                                    if (!_isAdmin(authState)) {
                                      _showAccessDenied();
                                      return;
                                    }
                                    _bloc.add(
                                      RunnersAdminSetEnabledRequested(
                                        address: runner.address,
                                        enabled: !runner.enabled,
                                      ),
                                    );
                                  },
                                );
                              },
                            ),
                ),
              );
            },
          );
        },
      ),
    );
  }
}
