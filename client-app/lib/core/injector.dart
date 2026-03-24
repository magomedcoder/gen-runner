import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/auth_interceptor.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/data/data_sources/local/user_local_data_source.dart';
import 'package:gen/data/data_sources/remote/auth_remote_datasource.dart';
import 'package:gen/data/data_sources/remote/chat_remote_datasource.dart';
import 'package:gen/data/data_sources/remote/editor_remote_datasource.dart';
import 'package:gen/data/data_sources/remote/runners_remote_datasource.dart';
import 'package:gen/data/data_sources/remote/user_remote_datasource.dart';
import 'package:gen/data/repositories/auth_repository_impl.dart';
import 'package:gen/data/repositories/chat_repository_impl.dart';
import 'package:gen/data/repositories/editor_repository_impl.dart';
import 'package:gen/data/repositories/runners_repository_impl.dart';
import 'package:gen/data/repositories/user_repository_impl.dart';
import 'package:gen/domain/repositories/auth_repository.dart';
import 'package:gen/domain/repositories/chat_repository.dart';
import 'package:gen/domain/repositories/editor_repository.dart';
import 'package:gen/domain/repositories/runners_repository.dart';
import 'package:gen/domain/repositories/user_repository.dart';
import 'package:gen/domain/usecases/auth/change_password_usecase.dart';
import 'package:gen/domain/usecases/auth/login_usecase.dart';
import 'package:gen/domain/usecases/auth/logout_usecase.dart';
import 'package:gen/domain/usecases/auth/refresh_token_usecase.dart';
import 'package:gen/domain/usecases/chat/connect_usecase.dart';
import 'package:gen/domain/usecases/chat/create_session_usecase.dart';
import 'package:gen/domain/usecases/chat/delete_session_usecase.dart';
import 'package:gen/domain/usecases/chat/get_default_runner_model_usecase.dart';
import 'package:gen/domain/usecases/chat/get_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_messages_usecase.dart';
import 'package:gen/domain/usecases/chat/get_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/get_sessions_usecase.dart';
import 'package:gen/domain/usecases/chat/send_message_usecase.dart';
import 'package:gen/domain/usecases/chat/set_default_runner_model_usecase.dart';
import 'package:gen/domain/usecases/chat/set_selected_runner_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_settings_usecase.dart';
import 'package:gen/domain/usecases/chat/update_session_title_usecase.dart';
import 'package:gen/domain/usecases/editor/transform_text_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_status_usecase.dart';
import 'package:gen/domain/usecases/runners/get_runners_usecase.dart';
import 'package:gen/domain/usecases/runners/set_runner_enabled_usecase.dart';
import 'package:gen/domain/usecases/users/create_user_usecase.dart';
import 'package:gen/domain/usecases/users/edit_user_usecase.dart';
import 'package:gen/domain/usecases/users/get_users_usecase.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_bloc.dart';
import 'package:gen/presentation/screens/admin/bloc/users_admin_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_bloc.dart';
import 'package:get_it/get_it.dart';

final sl = GetIt.instance;

Future<void> init() async {
  sl.registerLazySingleton<UserLocalDataSourceImpl>(() => UserLocalDataSourceImpl());
  await sl<UserLocalDataSourceImpl>().init();

  sl.registerLazySingleton<ServerConfig>(() => ServerConfig());
  await sl<ServerConfig>().init();

  sl.registerLazySingleton<AuthInterceptor>(
    () => AuthInterceptor(sl<UserLocalDataSourceImpl>()),
  );

  sl.registerLazySingleton<AuthGuard>(
    () => AuthGuard(
      () async {
        final storage = sl<UserLocalDataSourceImpl>();
        final refreshToken = storage.refreshToken;
        if (refreshToken == null || refreshToken.isEmpty) return false;
        try {
          final tokens = await sl<RefreshTokenUseCase>()(refreshToken);
          storage.saveTokens(tokens.accessToken, tokens.refreshToken);
          return true;
        } catch (_) {
          return false;
        }
      },
    ),
  );

  sl.registerLazySingleton<GrpcChannelManager>(
    () => GrpcChannelManager(sl<ServerConfig>(), sl<AuthInterceptor>()),
  );

  sl.registerLazySingleton<IChatRemoteDataSource>(
    () => ChatRemoteDataSource(sl<GrpcChannelManager>(), sl<AuthGuard>()),
  );

  sl.registerLazySingleton<IEditorRemoteDataSource>(
    () => EditorRemoteDataSource(sl<GrpcChannelManager>(), sl<AuthGuard>()),
  );

  sl.registerLazySingleton<IAuthRemoteDataSource>(
    () => AuthRemoteDataSource(sl<GrpcChannelManager>()),
  );

  sl.registerLazySingleton<IUserRemoteDataSource>(
    () => UserRemoteDataSource(sl<GrpcChannelManager>(), sl<AuthGuard>()),
  );

  sl.registerLazySingleton<IRunnersRemoteDataSource>(
    () => RunnersRemoteDataSource(sl<GrpcChannelManager>(), sl<AuthGuard>()),
  );

  sl.registerLazySingleton<ChatRepository>(
    () => ChatRepositoryImpl(sl()),
  );
  sl.registerLazySingleton<EditorRepository>(
    () => EditorRepositoryImpl(sl<IEditorRemoteDataSource>()),
  );
  sl.registerLazySingleton<AuthRepository>(() => AuthRepositoryImpl(sl()));
  sl.registerLazySingleton<UserRepository>(() => UserRepositoryImpl(sl()));
  sl.registerLazySingleton<RunnersRepository>(
    () => RunnersRepositoryImpl(sl<IRunnersRemoteDataSource>()),
  );

  sl.registerFactory(() => ConnectUseCase(sl()));
  sl.registerFactory(() => SendMessageUseCase(sl()));
  sl.registerFactory(() => CreateSessionUseCase(sl()));
  sl.registerFactory(() => GetSessionsUseCase(sl()));
  sl.registerFactory(() => GetSessionMessagesUseCase(sl()));
  sl.registerFactory(() => GetSessionSettingsUseCase(sl()));
  sl.registerFactory(() => UpdateSessionSettingsUseCase(sl()));
  sl.registerFactory(() => GetSelectedRunnerUseCase(sl()));
  sl.registerFactory(() => SetSelectedRunnerUseCase(sl()));
  sl.registerFactory(() => GetDefaultRunnerModelUseCase(sl()));
  sl.registerFactory(() => SetDefaultRunnerModelUseCase(sl()));
  sl.registerFactory(() => DeleteSessionUseCase(sl()));
  sl.registerFactory(() => UpdateSessionTitleUseCase(sl()));
  sl.registerFactory(() => TransformTextUseCase(sl()));
  sl.registerFactory(() => GetRunnersUseCase(sl()));
  sl.registerFactory(() => SetRunnerEnabledUseCase(sl()));
  sl.registerFactory(() => GetRunnersStatusUseCase(sl()));

  sl.registerFactory(() => LoginUseCase(sl()));
  sl.registerFactory(() => RefreshTokenUseCase(sl()));
  sl.registerFactory(() => LogoutUseCase(sl()));
  sl.registerFactory(() => ChangePasswordUseCase(sl()));

  sl.registerFactory(() => GetUsersUseCase(sl()));
  sl.registerFactory(() => CreateUserUseCase(sl()));
  sl.registerFactory(() => EditUserUseCase(sl()));

  sl.registerLazySingleton<AuthBloc>(
    () => AuthBloc(
      loginUseCase: sl(),
      refreshTokenUseCase: sl(),
      logoutUseCase: sl(),
      tokenStorage: sl<UserLocalDataSourceImpl>(),
      channelManager: sl(),
      authGuard: sl<AuthGuard>(),
    ),
  );

  sl.registerFactory(
    () => ChatBloc(
      authBloc: sl<AuthBloc>(),
      connectUseCase: sl(),
      getRunnersUseCase: sl(),
      getSessionSettingsUseCase: sl(),
      updateSessionSettingsUseCase: sl(),
      sendMessageUseCase: sl(),
      createSessionUseCase: sl(),
      getSessionsUseCase: sl(),
      getSessionMessagesUseCase: sl(),
      deleteSessionUseCase: sl(),
      updateSessionTitleUseCase: sl(),
      getRunnersStatusUseCase: sl(),
      getSelectedRunnerUseCase: sl(),
      setSelectedRunnerUseCase: sl(),
    ),
  );

  sl.registerFactory(
    () => EditorBloc(
      authBloc: sl<AuthBloc>(),
      getSelectedRunnerUseCase: sl(),
      transformTextUseCase: sl(),
      editorRepository: sl<EditorRepository>(),
    ),
  );

  sl.registerFactory(
    () => UsersAdminBloc(
      authBloc: sl<AuthBloc>(),
      getUsersUseCase: sl(),
      createUserUseCase: sl(),
      editUserUseCase: sl(),
    ),
  );

  sl.registerFactory(
    () => RunnersAdminBloc(
      getRunnersUseCase: sl(),
      setRunnerEnabledUseCase: sl(),
      getSelectedRunnerUseCase: sl(),
      setSelectedRunnerUseCase: sl(),
      getDefaultRunnerModelUseCase: sl(),
      setDefaultRunnerModelUseCase: sl(),
    ),
  );
}
