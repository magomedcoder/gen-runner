import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/admin/runners_admin_screen.dart';
import 'package:gen/presentation/screens/admin/web_search_admin_screen.dart';
import 'package:gen/presentation/screens/admin/users_admin_screen.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';

class AdminScreen extends StatefulWidget {
  const AdminScreen({super.key});

  @override
  State<AdminScreen> createState() => _AdminScreenState();
}

class _AdminScreenState extends State<AdminScreen> {
  int _sectionIndex = 0;

  static const _sections = <({String label, IconData icon, Widget page})>[
    (
      label: 'Раннеры',
      icon: Icons.dns_outlined,
      page: RunnersAdminScreen(),
    ),
    (
      label: 'Поиск',
      icon: Icons.travel_explore_outlined,
      page: WebSearchAdminScreen(),
    ),
    (
      label: 'Пользователи',
      icon: Icons.supervisor_account_outlined,
      page: UsersAdminScreen(),
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final mobile = Breakpoints.isMobile(context);
    final theme = Theme.of(context);

    if (mobile) {
      return Scaffold(
        appBar: AppBar(title: const Text('Админка')),
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
                    'Админ',
                    style: theme.textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                const SizedBox(height: 20),
                for (var i = 0; i < _sections.length; i++) ...[
                  _DesktopMenuTile(
                    icon: _sections[i].icon,
                    title: _sections[i].label,
                    selected: _sectionIndex == i,
                    onTap: () => setState(() => _sectionIndex = i),
                  ),
                  const SizedBox(height: 8),
                ],
                const Spacer(),
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
  }
}

class _DesktopMenuTile extends StatelessWidget {
  const _DesktopMenuTile({
    required this.icon,
    required this.title,
    required this.selected,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final selectedBg = theme.colorScheme.primaryContainer;
    final selectedFg = theme.colorScheme.onPrimaryContainer;
    final normalFg = theme.colorScheme.onSurfaceVariant;

    return Material(
      color: selected ? selectedBg : Colors.transparent,
      borderRadius: BorderRadius.circular(14),
      child: InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          child: Row(
            children: [
              Icon(
                icon,
                color: selected ? selectedFg : normalFg,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  title,
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                    color: selected ? selectedFg : null,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
