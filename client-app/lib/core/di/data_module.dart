import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/grpc_channel_manager.dart';
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
import 'package:get_it/get_it.dart';

void registerDataModule(GetIt sl) {
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
    () => ChatRepositoryImpl(sl<IChatRemoteDataSource>()),
  );
  sl.registerLazySingleton<EditorRepository>(
    () => EditorRepositoryImpl(sl<IEditorRemoteDataSource>()),
  );
  sl.registerLazySingleton<AuthRepository>(
    () => AuthRepositoryImpl(sl<IAuthRemoteDataSource>()),
  );
  sl.registerLazySingleton<UserRepository>(
    () => UserRepositoryImpl(sl<IUserRemoteDataSource>()),
  );
  sl.registerLazySingleton<RunnersRepository>(
    () => RunnersRepositoryImpl(sl<IRunnersRemoteDataSource>()),
  );
}
