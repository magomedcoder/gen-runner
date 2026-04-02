import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/auth_interceptor.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/server_config.dart';
import 'package:gen/core/ui/app_top_notice_controller.dart';
import 'package:gen/data/data_sources/local/user_local_data_source.dart';
import 'package:gen/domain/usecases/auth/refresh_token_usecase.dart';
import 'package:get_it/get_it.dart';

Future<void> registerCoreModule(GetIt sl) async {
  sl.registerLazySingleton<AppTopNoticeController>(AppTopNoticeController.new);

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
        if (refreshToken == null || refreshToken.isEmpty) {
          return false;
        }
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
}
