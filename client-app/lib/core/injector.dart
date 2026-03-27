import 'package:gen/core/di/blocs_module.dart';
import 'package:gen/core/di/core_module.dart';
import 'package:gen/core/di/data_module.dart';
import 'package:gen/core/di/usecases_module.dart';
import 'package:get_it/get_it.dart';

final sl = GetIt.instance;

Future<void> init() async {
  await registerCoreModule(sl);
  registerDataModule(sl);
  registerUseCasesModule(sl);
  registerBlocsModule(sl);
}
