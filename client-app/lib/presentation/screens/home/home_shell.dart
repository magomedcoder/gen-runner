import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/admin/runners_admin_screen.dart';
import 'package:gen/presentation/screens/admin/users_admin_screen.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/chat/chat_screen.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_event.dart';
import 'package:gen/presentation/screens/editor/editor_screen.dart';

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _index = 0;
  late final Widget _editorPage = BlocProvider(
    create: (_) => di.sl<EditorBloc>()..add(const EditorStarted()),
    child: const EditorScreen(),
  );

  static const _mobileUserDestinations = <NavigationDestination>[
    NavigationDestination(
      icon: Icon(Icons.chat_bubble_outline),
      selectedIcon: Icon(Icons.chat_rounded),
      label: 'Чат',
    ),
    NavigationDestination(
      icon: Icon(Icons.edit_note_outlined),
      selectedIcon: Icon(Icons.edit_note_rounded),
      label: 'Редактор',
    ),
  ];

  static const _mobileAdminDestinations = <NavigationDestination>[
    NavigationDestination(
      icon: Icon(Icons.chat_bubble_outline),
      selectedIcon: Icon(Icons.chat_rounded),
      label: 'Чат',
    ),
    NavigationDestination(
      icon: Icon(Icons.edit_note_outlined),
      selectedIcon: Icon(Icons.edit_note_rounded),
      label: 'Редактор',
    ),
    NavigationDestination(
      icon: Icon(Icons.supervisor_account_outlined),
      selectedIcon: Icon(Icons.supervisor_account),
      label: 'Пользователи',
    ),
    NavigationDestination(
      icon: Icon(Icons.dns_outlined),
      selectedIcon: Icon(Icons.dns_rounded),
      label: 'Раннеры',
    ),
  ];

  static const _railUserDestinations = <NavigationRailDestination>[
    NavigationRailDestination(
      icon: Icon(Icons.chat_bubble_outline),
      selectedIcon: Icon(Icons.chat_rounded),
      label: Text('Чат'),
    ),
    NavigationRailDestination(
      icon: Icon(Icons.edit_note_outlined),
      selectedIcon: Icon(Icons.edit_note_rounded),
      label: Text('Редактор'),
    ),
  ];

  static const _railAdminExtraDestinations = <NavigationRailDestination>[
    NavigationRailDestination(
      icon: Icon(Icons.supervisor_account_outlined),
      selectedIcon: Icon(Icons.supervisor_account),
      label: Text('Пользователи'),
    ),
    NavigationRailDestination(
      icon: Icon(Icons.dns_outlined),
      selectedIcon: Icon(Icons.dns_rounded),
      label: Text('Раннеры'),
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return BlocConsumer<AuthBloc, AuthState>(
      listenWhen: (prev, curr) =>
          (prev.user?.isAdmin ?? false) != (curr.user?.isAdmin ?? false),
      listener: (context, state) {
        final isAdmin = state.user?.isAdmin ?? false;
        if (!isAdmin && _index > 1) {
          setState(() => _index = 0);
        }
      },
      builder: (context, authState) {
        final isAdmin = authState.user?.isAdmin ?? false;
        final mobile = Breakpoints.isMobile(context);

        final pages = <Widget>[
          const ChatScreen(),
          _editorPage,
          if (isAdmin) const UsersAdminScreen() else const SizedBox.shrink(),
          if (isAdmin) const RunnersAdminScreen() else const SizedBox.shrink(),
        ];

        void select(int i) => setState(() => _index = i);

        if (mobile) {
          return Scaffold(
            body: IndexedStack(index: _index, children: pages),
            bottomNavigationBar: NavigationBar(
              selectedIndex: _index,
              onDestinationSelected: select,
              destinations: isAdmin
                  ? _mobileAdminDestinations
                  : _mobileUserDestinations,
            ),
          );
        }

        return Scaffold(
          body: Row(
            children: [
              NavigationRail(
                extended: false,
                selectedIndex: _index,
                onDestinationSelected: select,
                destinations: [
                  ..._railUserDestinations,
                  if (isAdmin) ..._railAdminExtraDestinations,
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
