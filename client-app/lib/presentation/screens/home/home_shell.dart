import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/admin/admin_screen.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/chat/chat_screen.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_event.dart';
import 'package:gen/presentation/screens/editor/editor_screen.dart';
import 'package:gen/presentation/screens/profile/profile_screen.dart';
import 'package:gen/presentation/widgets/app_side_navigation_rail.dart';

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  static const double _sideNavIndicatorSize = 48;

  int _index = 0;
  late final Widget _editorPage = BlocProvider(
    create: (_) => di.sl<EditorBloc>()..add(const EditorStarted()),
    child: const EditorScreen(),
  );

  static const _userNav = <AppSideNavDestination>[
    AppSideNavDestination(
      icon: Icons.chat_bubble_outline,
      selectedIcon: Icons.chat_rounded,
      label: 'Чат',
    ),
    AppSideNavDestination(
      icon: Icons.edit_note_outlined,
      selectedIcon: Icons.edit_note_rounded,
      label: 'Редактор',
    ),
    AppSideNavDestination(
      icon: Icons.person_outline,
      selectedIcon: Icons.person,
      label: 'Профиль',
      alignBottom: true,
    ),
  ];

  static const _adminNavExtra = <AppSideNavDestination>[
    AppSideNavDestination(
      icon: Icons.admin_panel_settings_outlined,
      selectedIcon: Icons.admin_panel_settings,
      label: 'Админ',
    ),
  ];

  static List<NavigationDestination> _mobileDestinations(
    List<AppSideNavDestination> items,
  ) {
    return [
      for (final d in items)
        NavigationDestination(
          icon: Icon(d.icon),
          selectedIcon: Icon(d.selectedIcon),
          label: d.label,
        ),
    ];
  }

  @override
  Widget build(BuildContext context) {
    return BlocConsumer<AuthBloc, AuthState>(
      listenWhen: (prev, curr) => (prev.user?.isAdmin ?? false) != (curr.user?.isAdmin ?? false),
      listener: (context, state) {
        final isAdmin = state.user?.isAdmin ?? false;
        if (!isAdmin && _index > 2) {
          setState(() => _index = 0);
        }
      },
      builder: (context, authState) {
        final isAdmin = authState.user?.isAdmin ?? false;
        final mobile = Breakpoints.isMobile(context);

        final pages = <Widget>[
          const ChatScreen(),
          _editorPage,
          const ProfileScreen(),
          if (isAdmin) const AdminScreen() else const SizedBox.shrink(),
        ];

        void select(int i) => setState(() => _index = i);

        if (mobile) {
          return Scaffold(
            body: IndexedStack(index: _index, children: pages),
            bottomNavigationBar: NavigationBar(
              selectedIndex: _index,
              onDestinationSelected: select,
              destinations: _mobileDestinations(
                isAdmin ? [
                  ..._userNav,
                  ..._adminNavExtra
                ] : _userNav,
              ),
            ),
          );
        }

        return Scaffold(
          body: Row(
            children: [
              AppSideNavigationRail(
                selectedIndex: _index,
                onDestinationSelected: select,
                indicatorSize: _sideNavIndicatorSize,
                destinations: [
                  ..._userNav,
                  if (isAdmin)
                    ..._adminNavExtra
                ],
              ),
              const VerticalDivider(width: 1, thickness: 1),
              Expanded(
                child: IndexedStack(index: _index, children: pages),
              ),
            ],
          ),
        );
      },
    );
  }
}
