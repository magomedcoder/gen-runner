import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_event.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/profile/about_app_screen.dart';
import 'package:gen/presentation/screens/profile/change_password_screen.dart';
import 'package:gen/presentation/screens/profile/profile_account_server_screen.dart';
import 'package:gen/presentation/screens/profile/user_mcp_screen.dart';
import 'package:gen/presentation/widgets/desktop_side_menu_tile.dart';

class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  int _sectionIndex = 0;

  static const _sections = <({String label, String subtitle, IconData icon, Widget page})>[
    (
      label: 'Аккаунт',
      subtitle: '',
      icon: Icons.person_outline,
      page: ProfileAccountServerScreen(),
    ),
    (
      label: 'MCP-серверы',
      subtitle: '',
      icon: Icons.extension_outlined,
      page: UserMcpScreen(),
    ),
    (
      label: 'Смена пароля',
      subtitle: '',
      icon: Icons.password_outlined,
      page: ChangePasswordScreen(),
    ),
    (
      label: 'О приложении',
      subtitle: '',
      icon: Icons.info_outline,
      page: AboutAppScreen(),
    ),
  ];

  Future<void> _confirmLogout(BuildContext context) async {
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

    if (ok == true && context.mounted) {
      context.read<AuthBloc>().add(const AuthLogoutRequested());
    }
  }

  @override
  Widget build(BuildContext context) {
    final mobile = Breakpoints.isMobile(context);
    final theme = Theme.of(context);

    return BlocBuilder<AuthBloc, AuthState>(
      builder: (context, auth) {
        if (mobile) {
          return Scaffold(
            appBar: AppBar(
              title: const Text('Профиль'),
              actions: [
                IconButton(
                  onPressed: auth.isLoading ? null : () => _confirmLogout(context),
                  tooltip: 'Выйти из аккаунта',
                  icon: const Icon(Icons.logout),
                ),
              ],
            ),
            body: ListView.separated(
              padding: const EdgeInsets.all(16),
              itemCount: _sections.length,
              separatorBuilder: (_, _) => const SizedBox(height: 12),
              itemBuilder: (context, index) {
                final section = _sections[index];
                return Card(
                  clipBehavior: Clip.antiAlias,
                  child: ListTile(
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 10,
                    ),
                    leading: Icon(section.icon),
                    title: Text(
                      section.label,
                      style: theme.textTheme.titleMedium?.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    subtitle: section.subtitle.isEmpty ? null : Text(section.subtitle),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () {
                      final authBloc = context.read<AuthBloc>();
                      Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (_) => BlocProvider<AuthBloc>.value(
                            value: authBloc,
                            child: section.page,
                          ),
                        ),
                      );
                    },
                  ),
                );
              },
            ),
          );
        }

        return Scaffold(
          body: Row(
            children: [
              Container(
                width: 300,
                color: theme.colorScheme.surfaceContainerLow,
                padding: const EdgeInsets.fromLTRB(16, 20, 16, 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: Text(
                        'Профиль',
                        style: theme.textTheme.headlineSmall?.copyWith(
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    const SizedBox(height: 20),
                    for (var i = 0; i < _sections.length; i++) ...[
                      DesktopSideMenuTile(
                        icon: _sections[i].icon,
                        title: _sections[i].label,
                        selected: _sectionIndex == i,
                        onTap: () => setState(() => _sectionIndex = i),
                      ),
                      const SizedBox(height: 8),
                    ],
                    const Spacer(),
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: TextButton.icon(
                        onPressed: auth.isLoading ? null : () => _confirmLogout(context),
                        icon: const Icon(Icons.logout),
                        label: const Text('Выйти'),
                      ),
                    ),
                  ],
                ),
              ),
              const VerticalDivider(width: 1, thickness: 1),
              Expanded(
                child: IndexedStack(
                  index: _sectionIndex,
                  children: [for (final s in _sections) s.page],
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
