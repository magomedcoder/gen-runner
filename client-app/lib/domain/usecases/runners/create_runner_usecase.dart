import 'package:gen/domain/repositories/runners_repository.dart';

class CreateRunnerUseCase {
  final RunnersRepository _repo;

  CreateRunnerUseCase(this._repo);

  Future<void> call({
    required String name,
    required String host,
    required int port,
    required bool enabled,
  }) => _repo.createRunner(
    name: name,
    host: host,
    port: port,
    enabled: enabled,
  );
}
