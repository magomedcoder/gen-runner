import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/core/util.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/profile/profile_section_card.dart';

class ProfileAccountServerScreen extends StatelessWidget {
  const ProfileAccountServerScreen({super.key});

  String _serverLabel(ServerConfig c) {
    final s = formatServerAddressForField(c);
    if (s.isEmpty) {
      return 'не задан';
    }

    return s;
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
            title: const Text('Аккаунт'),
          ),
          body: SafeArea(
            top: false,
            child: ListView(
              padding: EdgeInsets.fromLTRB(horizontal, 16, horizontal, 24),
              children: [
                profileSectionCard(
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
                profileSectionCard(
                  context,
                  title: 'Сервер',
                  child: SelectableText(
                    _serverLabel(config),
                    style: textTheme.bodyLarge,
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
