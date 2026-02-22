import 'package:gen/core/failures.dart';
import 'package:gen/data/data_sources/remote/auth_remote_datasource.dart';
import 'package:gen/domain/entities/auth_result.dart';
import 'package:gen/domain/entities/auth_tokens.dart';
import 'package:gen/domain/repositories/auth_repository.dart';

class AuthRepositoryImpl implements AuthRepository {
  final IAuthRemoteDataSource dataSource;

  AuthRepositoryImpl(this.dataSource);

  @override
  Future<AuthResult> login(String username, String password) async {
    try {
      return await dataSource.login(username, password);
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка входа: $e');
    }
  }

  @override
  Future<AuthTokens> refreshToken(String refreshToken) async {
    try {
      return await dataSource.refreshToken(refreshToken);
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка обновления токена: $e');
    }
  }

  @override
  Future<void> logout() async {
    try {
      await dataSource.logout();
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка выхода: $e');
    }
  }

  @override
  Future<void> changePassword(String oldPassword, String newPassword) async {
    try {
      await dataSource.changePassword(oldPassword, newPassword);
    } catch (e) {
      if (e is Failure) rethrow;
      throw ApiFailure('Ошибка смены пароля: $e');
    }
  }
}
