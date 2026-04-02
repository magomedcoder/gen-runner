import 'package:grpc/grpc.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/data/mappers/auth_mapper.dart';
import 'package:gen/domain/entities/auth_result.dart';
import 'package:gen/domain/entities/auth_tokens.dart';
import 'package:gen/generated/grpc_pb/auth.pbgrpc.dart' as grpc;

abstract class IAuthRemoteDataSource {
  Future<AuthResult> login(String username, String password);

  Future<AuthTokens> refreshToken(String refreshToken);

  Future<void> logout();

  Future<void> changePassword(String oldPassword, String newPassword);
}

class AuthRemoteDataSource implements IAuthRemoteDataSource {
  final GrpcChannelManager _channelManager;

  AuthRemoteDataSource(this._channelManager);

  grpc.AuthServiceClient get _client => _channelManager.authClient;

  @override
  Future<AuthResult> login(String username, String password) async {
    Logs().d('AuthRemote: login username=$username');
    try {
      final request = grpc.LoginRequest(
        username: username,
        password: password,
      );

      final response = await _client.login(request);
      Logs().i('AuthRemote: login успешен');
      return AuthMapper.loginResponseFromProto(response);
    } on GrpcError catch (e) {
      throwGrpcError(
        e,
        'вход',
        unauthenticatedMessage: 'Неверное имя пользователя или пароль',
      );
    } catch (e) {
      Logs().e('AuthRemote: login', exception: e);
      throw ApiFailure('Ошибка входа');
    }
  }

  @override
  Future<AuthTokens> refreshToken(String refreshToken) async {
    Logs().d('AuthRemote: refreshToken');
    try {
      final request = grpc.RefreshTokenRequest(
        refreshToken: refreshToken
      );

      final response = await _client.refreshToken(request);
      Logs().i('AuthRemote: refreshToken успешен');
      return AuthMapper.refreshTokenResponseFromProto(response);
    } on GrpcError catch (e) {
      throwGrpcError(
        e,
        'обновление токена',
        unauthenticatedMessage: 'Недействительный refresh token',
      );
    } catch (e) {
      Logs().e('AuthRemote: refreshToken', exception: e);
      throw ApiFailure('Ошибка обновления токена');
    }
  }

  @override
  Future<void> logout() async {
    Logs().d('AuthRemote: logout');
    try {
      final request = grpc.LogoutRequest();

      await _client.logout(request);
    } on GrpcError catch (e) {
      Logs().w(
        'AuthRemote: logout gRPC code=${e.code} message=${e.message}',
        exception: e,
      );
      throw NetworkFailure('Ошибка выхода (код ${e.code})');
    } catch (e) {
      Logs().e('AuthRemote: logout', exception: e);
      throw ApiFailure('Ошибка выхода');
    }
  }

  @override
  Future<void> changePassword(String oldPassword, String newPassword) async {
    Logs().d('AuthRemote: changePassword');
    try {
      final request = grpc.ChangePasswordRequest(
        oldPassword: oldPassword,
        newPassword: newPassword
      );

      await _client.changePassword(request);
      Logs().i('AuthRemote: changePassword успешен');
    } on GrpcError catch (e) {
      if (e.code == StatusCode.invalidArgument) {
        Logs().w('AuthRemote: changePassword invalidArgument: ${e.message}');
        throw NetworkFailure('Неверные данные (код ${e.code})');
      }

      throwGrpcError(e, 'смена пароля');
    } catch (e) {
      Logs().e('AuthRemote: changePassword', exception: e);
      throw ApiFailure('Ошибка смены пароля');
    }
  }
}
