import 'package:grpc/grpc.dart';
import 'package:gen/core/auth_guard.dart';
import 'package:gen/core/failures.dart';
import 'package:gen/core/grpc_channel_manager.dart';
import 'package:gen/core/grpc_error_handler.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/data/mappers/user_mapper.dart';
import 'package:gen/domain/entities/user.dart';
import 'package:gen/generated/grpc_pb/user.pbgrpc.dart' as grpc;

abstract class IUserRemoteDataSource {
  Future<List<User>> getUsers({required int page, required int pageSize});

  Future<User> createUser({
    required String username,
    required String password,
    required String name,
    required String surname,
    required int role,
  });

  Future<User> editUser({
    required String id,
    required String username,
    required String password,
    required String name,
    required String surname,
    required int role,
  });
}

class UserRemoteDataSource implements IUserRemoteDataSource {
  final GrpcChannelManager _channelManager;
  final AuthGuard _authGuard;

  UserRemoteDataSource(this._channelManager, this._authGuard);

  grpc.UserServiceClient get _client => _channelManager.userClient;

  @override
  Future<List<User>> getUsers({required int page, required int pageSize}) async {
    Logs().d('UserRemote: getUsers page=$page pageSize=$pageSize');
    try {
      final req = grpc.GetUsersRequest(
        page: page,
        pageSize: pageSize,
      );
      final resp = await _authGuard.execute(() => _client.getUsers(req));
      Logs().i('UserRemote: getUsers получено ${resp.users.length}');
      return UserMapper.listFromProto(resp.users);
    } on GrpcError catch (e) {
      if (e.code == StatusCode.permissionDenied) {
        throw NetworkFailure('Доступ разрешён только администратору');
      }

      throwGrpcError(e, 'Ошибка gRPC');
    } catch (e) {
      Logs().e('UserRemote: getUsers', exception: e);
      throw ApiFailure('Ошибка получения пользователей');
    }
  }

  @override
  Future<User> createUser({
    required String username,
    required String password,
    required String name,
    required String surname,
    required int role,
  }) async {
    Logs().d('UserRemote: createUser username=$username');
    try {
      final req = grpc.CreateUserRequest(
        username: username,
        password: password,
        name: name,
        surname: surname,
        role: role,
      );
      final resp = await _authGuard.execute(() => _client.createUser(req));
      Logs().i('UserRemote: createUser успешен');
      return UserMapper.fromProto(resp.user);
    } on GrpcError catch (e) {
      if (e.code == StatusCode.invalidArgument) {
        throw NetworkFailure(e.message ?? 'Неверные данные');
      }

      if (e.code == StatusCode.permissionDenied) {
        throw NetworkFailure('Доступ разрешён только администратору');
      }

      throwGrpcError(e, 'Ошибка gRPC');
    } catch (e) {
      Logs().e('UserRemote: createUser', exception: e);
      throw ApiFailure('Ошибка создания пользователя');
    }
  }

  @override
  Future<User> editUser({
    required String id,
    required String username,
    required String password,
    required String name,
    required String surname,
    required int role,
  }) async {
    Logs().d('UserRemote: editUser id=$id');
    try {
      final req = grpc.EditUserRequest(
        id: id,
        username: username,
        password: password,
        name: name,
        surname: surname,
        role: role,
      );
      final resp = await _authGuard.execute(() => _client.editUser(req));
      Logs().i('UserRemote: editUser успешен');
      return UserMapper.fromProto(resp.user);
    } on GrpcError catch (e) {
      if (e.code == StatusCode.invalidArgument) {
        throw NetworkFailure(e.message ?? 'Неверные данные');
      }

      if (e.code == StatusCode.permissionDenied) {
        throw NetworkFailure('Доступ разрешён только администратору');
      }

      throwGrpcError(e, 'Ошибка gRPC');
    } catch (e) {
      Logs().e('UserRemote: editUser', exception: e);
      throw ApiFailure('Ошибка обновления пользователя');
    }
  }
}
