import 'package:flutter/material.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/admin/runners_admin_screen.dart';
import 'package:gen/presentation/screens/admin/users_admin_screen.dart';

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
      page: const RunnersAdminScreen(),
    ),
    (
      label: 'Пользователи',
      icon: Icons.supervisor_account_outlined,
      page: const UsersAdminScreen(),
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final mobile = Breakpoints.isMobile(context);
    final theme = Theme.of(context);

    if (mobile) {
      return Scaffold(
        body: IndexedStack(
          index: _sectionIndex,
          children: [for (final s in _sections) s.page],
        ),
        bottomNavigationBar: NavigationBar(
          selectedIndex: _sectionIndex,
          onDestinationSelected: (i) => setState(() => _sectionIndex = i),
          destinations: [
            for (final s in _sections)
              NavigationDestination(icon: Icon(s.icon), label: s.label),
          ],
        ),
      );
    }

    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _sectionIndex,
            onDestinationSelected: (i) => setState(() => _sectionIndex = i),
            extended: true,
            minExtendedWidth: 280,
            backgroundColor: theme.colorScheme.surfaceContainerLow,
            selectedLabelTextStyle: theme.textTheme.titleMedium?.copyWith(
              fontSize: 17,
              fontWeight: FontWeight.w700,
            ),
            unselectedLabelTextStyle: theme.textTheme.titleMedium?.copyWith(
              fontSize: 16,
              fontWeight: FontWeight.w500,
            ),
            leading: const Padding(
              padding: EdgeInsets.fromLTRB(16, 12, 16, 8),
              child: Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  'Админ',
                  style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
                ),
              ),
            ),
            destinations: [
              for (final s in _sections)
                NavigationRailDestination(
                  icon: Icon(s.icon),
                  selectedIcon: Icon(s.icon),
                  label: Text(s.label),
                ),
            ],
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
