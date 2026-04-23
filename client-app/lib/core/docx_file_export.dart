import 'dart:typed_data';

import 'package:gen/core/docx_file_export_io.dart' as impl;

Future<bool> saveDocxToFile(Uint8List bytes, String fileName) => impl.saveDocxToFileImpl(bytes, fileName);
