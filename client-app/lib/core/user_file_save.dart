import 'dart:typed_data';

import 'package:gen/core/user_file_save_io.dart' as impl;

Future<bool> saveUserPickedFile(Uint8List bytes, String fileName) => impl.saveUserPickedFileImpl(bytes, fileName);
