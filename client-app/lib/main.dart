import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:gen/core/injector.dart' as di;
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/theme/app_theme.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_event.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_state.dart';
import 'package:gen/presentation/screens/auth/login_screen.dart';
import 'package:gen/presentation/screens/auth/update_required_screen.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/home/home_shell.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  Logs().i('Запуск приложения');
  await di.init();
  Logs().i('Инициализация завершена');
  runApp(const App());
}

class App extends StatelessWidget {
  const App({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'Gen',
      theme: AppTheme.dark,
      darkTheme: AppTheme.dark,
      themeMode: ThemeMode.dark,
      localizationsDelegates: const [
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: const [Locale('ru')],
      home: MultiBlocProvider(
        providers: [
          BlocProvider(
            create: (context) => di.sl<AuthBloc>()..add(const AuthCheckRequested()),
          ),
          BlocProvider(
            create: (context) => di.sl<ChatBloc>(),
          ),
        ],
        child: BlocBuilder<AuthBloc, AuthState>(
          builder: (context, authState) {
            if (authState.needsUpdate) {
              return const UpdateRequiredScreen();
            }

            if (authState.isLoading && !authState.isAuthenticated) {
              return const Scaffold(
                body: Center(child: CircularProgressIndicator()),
              );
            }

            if (authState.isAuthenticated) {
              return const HomeShell();
            }

            return const LoginScreen();
          },
        ),
      ),
    );
  }
}