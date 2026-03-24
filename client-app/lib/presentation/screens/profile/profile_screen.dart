import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/core/util.dart';
import 'package:gen/domain/entities/runner_info.dart';
import 'package:gen/domain/usecases/auth/change_password_usecase.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_event.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/auth/login_form_decoration.dart';

class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  final _formKey = GlobalKey<FormState>();
  final _oldPasswordController = TextEditingController();
  final _newPasswordController = TextEditingController();
  final _confirmPasswordController = TextEditingController();
  bool _obscureOld = true;
  bool _obscureNew = true;
  bool _obscureConfirm = true;
  bool _savingPassword = false;
  bool _loadingRunnerPrefs = true;
  bool _savingRunnerPref = false;
  List<String> _availableRunners = const [];
  Map<String, String> _runnerNames = const {};
  String? _selectedRunner;

  List<String> _extractAvailableRunners(List<RunnerInfo> runners) {
    final addresses = <String>{
      for (final runner in runners)
        if (runner.enabled && runner.address.isNotEmpty) runner.address,
    };
    final sorted = addresses.toList()..sort();
    return sorted;
  }

  Map<String, String> _extractRunnerNames(List<RunnerInfo> runners) {
    final names = <String, String>{};
    for (final runner in runners) {
      if (!runner.enabled || runner.address.isEmpty) {
        continue;
      }

      final name = runner.name.trim();
      names[runner.address] = name.isNotEmpty ? name : runner.address;
    }

    return names;
  }

  Future<void> _loadRunnerPreferences() async {
    setState(() => _loadingRunnerPrefs = true);
    try {
      final runners = await sl<GetRunnersUseCase>()();
      final selected = await sl<GetSelectedRunnerUseCase>()();
      final available = _extractAvailableRunners(runners);
      final runnerNames = _extractRunnerNames(runners);
      String? effectiveSelected = selected != null && available.contains(selected) ? selected : null;
      if (effectiveSelected == null && available.isNotEmpty) {
        effectiveSelected = available.first;
        await sl<SetSelectedRunnerUseCase>()(effectiveSelected);
      }

      setState(() {
        _availableRunners = available;
        _runnerNames = runnerNames;
        _selectedRunner = effectiveSelected;
      });
    } catch (_) {
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: const Text('Не удалось загрузить раннеры'),
          backgroundColor: Theme.of(context).colorScheme.error,
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        ),
      );
    } finally {
      if (mounted) {
        setState(() => _loadingRunnerPrefs = false);
      }
    }
  }

  Future<void> _setSelectedRunner(String? runner) async {
    setState(() => _savingRunnerPref = true);
    try {
      await sl<SetSelectedRunnerUseCase>()(runner);
      if (!mounted) {
        return;
      }
      setState(() => _selectedRunner = runner);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: const Text('Раннер по умолчанию сохранён'),
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        ),
      );
    } catch (e) {
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(e.toString().replaceAll('Exception: ', '')),
          backgroundColor: Theme.of(context).colorScheme.error,
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        ),
      );
    } finally {
      if (mounted) {
        setState(() => _savingRunnerPref = false);
      }
    }
  }

  String _serverLabel(ServerConfig c) {
    final s = formatServerAddressForField(c);
    if (s.isEmpty) {
      return 'не задан';
    }

    return s;
  }

  @override
  void dispose() {
    _oldPasswordController.dispose();
    _newPasswordController.dispose();
    _confirmPasswordController.dispose();
    super.dispose();
  }

  @override
  void initState() {
    super.initState();
    _loadRunnerPreferences();
  }

  Future<void> _submitChangePassword() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    setState(() => _savingPassword = true);
    try {
      await sl<ChangePasswordUseCase>()(
        _oldPasswordController.text,
        _newPasswordController.text,
      );
      if (!mounted) {
        return;
      }

      _oldPasswordController.clear();
      _newPasswordController.clear();
      _confirmPasswordController.clear();

      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: const Text('Пароль успешно изменён'),
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        ),
      );
    } catch (e) {
      if (!mounted) {
        return;
      }

      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(e.toString().replaceAll('Exception: ', '')),
          backgroundColor: Theme.of(context).colorScheme.error,
          behavior: SnackBarBehavior.floating,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
        ),
      );
    } finally {
      if (mounted) {
        setState(() => _savingPassword = false);
      }
    }
  }

  Future<void> _confirmLogout() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Выход'),
        content: const Text('Завершить сеанс и выйти из приложения?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Отмена'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('Выйти'),
          ),
        ],
      ),
    );

    if (ok == true && mounted) {
      context.read<AuthBloc>().add(const AuthLogoutRequested());
    }
  }

  String? _validateNewPassword(String? value) {
    if (value == null || value.isEmpty) {
      return 'Введите новый пароль';
    }

    if (value.length < 8) {
      return 'Пароль должен содержать минимум 8 символов';
    }

    return null;
  }

  @override
  Widget build(BuildContext context) {
    final config = sl<ServerConfig>();
    final horizontal = Breakpoints.isMobile(context) ? 16.0 : 24.0;
    final scheme = Theme.of(context).colorScheme;

    return BlocBuilder<AuthBloc, AuthState>(
      builder: (context, auth) {
        final user = auth.user;
        final displayName = user == null
          ? '-'
          : [
            user.name.trim(),
            user.surname.trim(),
          ].where((s) => s.isNotEmpty).join(' ');

        return Scaffold(
          appBar: AppBar(title: const Text('Профиль')),
          body: SafeArea(
            child: ListView(
              padding: EdgeInsets.fromLTRB(horizontal, 16, horizontal, 24),
              children: [
                Text(
                  'Аккаунт',
                  style: Theme.of(
                    context,
                  ).textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 8),
                Text(
                  user == null
                    ? 'Пользователь'
                    : (displayName.isEmpty ? user.username : displayName),
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 4),
                Text(
                  user?.username ?? '-',
                  style: Theme.of(
                    context,
                  ).textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 20),
                Divider(color: scheme.outlineVariant),
                const SizedBox(height: 20),
                Text(
                  'Сервер',
                  style: Theme.of(
                    context,
                  ).textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 8),
                SelectableText(
                  _serverLabel(config),
                  style: Theme.of(context).textTheme.bodyLarge,
                ),
                const SizedBox(height: 20),
                Divider(color: scheme.outlineVariant),
                const SizedBox(height: 20),
                Text(
                  'Раннер по умолчанию',
                  style: Theme.of(
                    context,
                  ).textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 8),
                DropdownButtonFormField<String?>(
                  initialValue: _selectedRunner,
                  isExpanded: true,
                  decoration: const InputDecoration(
                    border: OutlineInputBorder(),
                    isDense: true,
                    labelText: 'Раннер',
                  ),
                  items: [
                    const DropdownMenuItem<String?>(
                      value: null,
                      child: Text('Не выбран'),
                    ),
                    for (final address in _availableRunners)
                      DropdownMenuItem<String?>(
                        value: address,
                        child: Text(_runnerNames[address] ?? address),
                      ),
                  ],
                  onChanged: _loadingRunnerPrefs || _savingRunnerPref
                      ? null
                      : (value) => _setSelectedRunner(value),
                ),
                const SizedBox(height: 20),
                Divider(color: scheme.outlineVariant),
                const SizedBox(height: 20),
                Text(
                  'Смена пароля',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 12),
                Form(
                  key: _formKey,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      TextFormField(
                        controller: _oldPasswordController,
                        obscureText: _obscureOld,
                        textInputAction: TextInputAction.next,
                        decoration: LoginFormDecoration.field(
                          labelText: 'Текущий пароль',
                          hintText: 'Введите текущий пароль',
                          prefixIcon: const Icon(Icons.lock_outline),
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscureOld
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            ),
                            onPressed: () =>
                                setState(() => _obscureOld = !_obscureOld),
                          ),
                        ),
                        validator: (value) {
                          if (value == null || value.isEmpty) {
                            return 'Введите текущий пароль';
                          }

                          return null;
                        },
                      ),
                      const SizedBox(height: 12),
                      TextFormField(
                        controller: _newPasswordController,
                        obscureText: _obscureNew,
                        textInputAction: TextInputAction.next,
                        decoration: LoginFormDecoration.field(
                          labelText: 'Новый пароль',
                          hintText: 'Не менее 8 символов',
                          prefixIcon: const Icon(Icons.lock_outlined),
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscureNew
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            ),
                            onPressed: () => setState(() => _obscureNew = !_obscureNew),
                          ),
                        ),
                        validator: _validateNewPassword,
                      ),
                      const SizedBox(height: 12),
                      TextFormField(
                        controller: _confirmPasswordController,
                        obscureText: _obscureConfirm,
                        textInputAction: TextInputAction.done,
                        onFieldSubmitted: (_) => _submitChangePassword(),
                        decoration: LoginFormDecoration.field(
                          labelText: 'Повторите новый пароль',
                          hintText: 'Тот же пароль ещё раз',
                          prefixIcon: const Icon(Icons.lock_outlined),
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscureConfirm
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            ),
                            onPressed: () => setState(() => _obscureConfirm = !_obscureConfirm),
                          ),
                        ),
                        validator: (value) {
                          if (value == null || value.isEmpty) {
                            return 'Повторите новый пароль';
                          }

                          if (value != _newPasswordController.text) {
                            return 'Пароли не совпадают';
                          }

                          return null;
                        },
                      ),
                      const SizedBox(height: 20),
                      FilledButton(
                        onPressed: _savingPassword  ? null : _submitChangePassword,
                        style: FilledButton.styleFrom(
                          minimumSize: const Size.fromHeight(48),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(12),
                          ),
                        ),
                        child: _savingPassword
                          ? SizedBox(
                            height: 20,
                            width: 20,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              valueColor: AlwaysStoppedAnimation<Color>(Theme.of(context).colorScheme.onPrimary),
                            ),
                          )
                          : const Text('Сохранить новый пароль'),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                Divider(color: scheme.outlineVariant),
                const SizedBox(height: 20),
                OutlinedButton.icon(
                  onPressed: auth.isLoading ? null : _confirmLogout,
                  icon: const Icon(Icons.logout),
                  label: const Text('Выйти из аккаунта'),
                  style: OutlinedButton.styleFrom(
                    minimumSize: const Size.fromHeight(48),
                    foregroundColor: scheme.onSurface,
                    side: BorderSide(color: scheme.outlineVariant),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}
