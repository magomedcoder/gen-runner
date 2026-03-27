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
import 'package:get_it/get_it.dart';

void registerUseCasesModule(GetIt sl) {
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
}
