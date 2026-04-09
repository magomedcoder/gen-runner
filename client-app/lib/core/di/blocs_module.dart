import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/data/data_sources/local/user_local_data_source.dart';
import 'package:gen/domain/repositories/editor_repository.dart';
import 'package:gen/presentation/screens/admin/bloc/runners_admin_bloc.dart';
import 'package:gen/presentation/screens/admin/bloc/users_admin_bloc.dart';
import 'package:gen/presentation/screens/auth/bloc/auth_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/widgets/app_top_notice/bloc/app_top_notice_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_bloc.dart';
import 'package:get_it/get_it.dart';

void registerBlocsModule(GetIt sl) {
  sl.registerLazySingleton<AuthBloc>(
    () => AuthBloc(
      loginUseCase: sl(),
      refreshTokenUseCase: sl(),
      logoutUseCase: sl(),
      tokenStorage: sl<UserLocalDataSourceImpl>(),
      channelManager: sl<GrpcChannelManager>(),
      authGuard: sl<AuthGuard>(),
    ),
  );

  sl.registerLazySingleton<AppTopNoticeBloc>(
    () => AppTopNoticeBloc(
      connectUseCase: sl(),
      authBloc: sl<AuthBloc>(),
    ),
  );

  sl.registerFactory(
    () => ChatBloc(
      authBloc: sl<AuthBloc>(),
      appTopNoticeBloc: sl<AppTopNoticeBloc>(),
      connectUseCase: sl(),
      getRunnersUseCase: sl(),
      getUserRunnersUseCase: sl(),
      getSessionSettingsUseCase: sl(),
      updateSessionSettingsUseCase: sl(),
      sendMessageUseCase: sl(),
      putSessionFileUseCase: sl(),
      getFileIngestionStatusUseCase: sl(),
      regenerateAssistantUseCase: sl(),
      continueAssistantUseCase: sl(),
      editUserMessageAndContinueUseCase: sl(),
      getUserMessageEditsUseCase: sl(),
      getSessionMessagesForUserMessageVersionUseCase: sl(),
      getAssistantMessageRegenerationsUseCase: sl(),
      getSessionMessagesForAssistantMessageVersionUseCase: sl(),
      createSessionUseCase: sl(),
      getSessionsUseCase: sl(),
      getSessionMessagesUseCase: sl(),
      deleteSessionUseCase: sl(),
      updateSessionTitleUseCase: sl(),
      getRunnersStatusUseCase: sl(),
      getSelectedRunnerUseCase: sl(),
      setSelectedRunnerUseCase: sl(),
      getWebSearchAvailabilityUseCase: sl(),
    ),
  );

  sl.registerFactory(
    () => EditorBloc(
      authBloc: sl<AuthBloc>(),
      getRunnersUseCase: sl(),
      getUserRunnersUseCase: sl(),
      getSelectedRunnerUseCase: sl(),
      setSelectedRunnerUseCase: sl(),
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
      createRunnerUseCase: sl(),
      updateRunnerUseCase: sl(),
      deleteRunnerUseCase: sl(),
      getSelectedRunnerUseCase: sl(),
      setSelectedRunnerUseCase: sl(),
    ),
  );
}
