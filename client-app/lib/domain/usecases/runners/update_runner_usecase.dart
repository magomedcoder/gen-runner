import 'package:gen/domain/repositories/runners_repository.dart';

class UpdateRunnerUseCase {
  final RunnersRepository _repo;

  UpdateRunnerUseCase(this._repo);

  Future<void> call({
    required int id,
    required String name,
    required String host,
    required int port,
    required bool enabled,
  }) => _repo.updateRunner(
    id: id,
    name: name,
    host: host,
    port: port,
    enabled: enabled,
  );
}
