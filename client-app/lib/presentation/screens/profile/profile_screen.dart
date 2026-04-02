import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/core/ui/app_top_notice.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/core/util.dart';
import 'package:gen/domain/usecases/auth/change_password_usecase.dart';
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

      showAppTopNotice('Пароль успешно изменён');
    } catch (e) {
      if (!mounted) {
        return;
      }

      showAppTopNotice(
        userSafeErrorMessage(e, fallback: 'Не удалось сменить пароль'),
        error: true,
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
    final config = sl<ServerConfig>();
    final horizontal = Breakpoints.isMobile(context) ? 16.0 : 24.0;
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;

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
          appBar: AppBar(
            title: const Text('Профиль'),
            actions: [
              IconButton(
                onPressed: auth.isLoading ? null : _confirmLogout,
                tooltip: 'Выйти из аккаунта',
                icon: const Icon(Icons.logout),
              ),
            ],
          ),
          body: SafeArea(
            top: false,
            child: ListView(
              padding: EdgeInsets.fromLTRB(horizontal, 16, horizontal, 24),
              children: [
                _settingsSection(
                  context,
                  title: 'Аккаунт',
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        user == null
                          ? 'Пользователь'
                          : (displayName.isEmpty ? user.username : displayName),
                        style: textTheme.titleLarge,
                      ),
                      const SizedBox(height: 4),
                      Text(
                        '@${user?.username ?? '-'}',
                        style: textTheme.bodyMedium?.copyWith(
                          color: scheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 12),
                _settingsSection(
                  context,
                  title: 'Сервер',
                  child: SelectableText(
                    _serverLabel(config),
                    style: textTheme.bodyLarge,
                  ),
                ),
                const SizedBox(height: 12),
                _settingsSection(
                  context,
                  title: 'Смена пароля',
                  child: Form(
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
                              onPressed: () => setState(() => _obscureOld = !_obscureOld),
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
                          onPressed: _savingPassword ? null : _submitChangePassword,
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
                                valueColor: AlwaysStoppedAnimation<Color>(
                                  Theme.of(context).colorScheme.onPrimary,
                                ),
                              ),
                            )
                            : const Text('Сохранить'),
                        ),
                      ],
                    ),
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
