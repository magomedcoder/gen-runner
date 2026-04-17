// ignore_for_file: implementation_imports

import 'package:grpc/grpc.dart';
import 'package:grpc/src/generated/google/rpc/error_details.pb.dart';

String? grpcErrorInfoReason(GrpcError e) {
  final list = e.details;
  if (list == null) {
    return null;
  }
  for (final d in list) {
    if (d is ErrorInfo && d.reason.isNotEmpty) {
      return d.reason;
    }
  }
  return null;
}
