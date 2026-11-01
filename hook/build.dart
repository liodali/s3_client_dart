import 'dart:ffi';
import 'dart:io';

import 'package:code_assets/code_assets.dart';
import 'package:hooks/hooks.dart';

void main(List<String> args) async {
  await build(args, (input, output) async {
    if (!input.config.buildCodeAssets) return;
    final target = input.config.code;
    final os = switch (target.targetOS) {
      OS.macOS => 'darwin',
      OS.linux => 'linux',
      OS.windows => 'windows',
      _ => throw UnsupportedError(
        'S3 client does not support ${target.targetOS}.',
      ),
    };
    final arch = switch (target.targetArchitecture) {
      Architecture.x64 => 'amd64',
      Architecture.arm64 => 'arm64',
      _ => throw UnsupportedError(
        'S3 client does not support ${target.targetArchitecture}.',
      ),
    };
    final hostArch = switch (Abi.current()) {
      Abi.macosArm64 || Abi.linuxArm64 || Abi.windowsArm64 => 'arm64',
      Abi.macosX64 || Abi.linuxX64 || Abi.windowsX64 => 'amd64',
      _ => 'unsupported',
    };
    if (Platform.operatingSystem != os &&
            !(Platform.isMacOS && os == 'darwin') ||
        hostArch != arch) {
      throw UnsupportedError(
        'Cross-compilation is not supported by the Go build hook: target $os/$arch. Build on a matching host.',
      );
    }
    final extension = switch (os) {
      'darwin' => 'dylib',
      'windows' => 'dll',
      _ => 'so',
    };
    final out = input.outputDirectory.resolve('s3_client_dart.$extension');
    await Directory.fromUri(input.outputDirectory).create(recursive: true);
    final sourceDir = input.packageRoot.resolve('go_ffi/');
    for (final name in ['main.go', 'go.mod', 'go.sum']) {
      output.dependencies.add(sourceDir.resolve(name));
    }
    final ProcessResult result;
    try {
      result = await Process.run(
        'go',
        [
          'build',
          '-buildmode=c-shared',
          '-trimpath',
          if (os == 'darwin')
            '-ldflags=-extldflags=-Wl,-headerpad_max_install_names',
          '-o',
          out.toFilePath(),
          '.',
        ],
        workingDirectory: sourceDir.toFilePath(),
        environment: {
          'CGO_ENABLED': '1',
          'GOOS': os,
          'GOARCH': arch,
          'GOTOOLCHAIN': 'local',
        },
      );
    } on ProcessException catch (e) {
      throw StateError(
        'Go 1.25.5 or newer and a C compiler are required to build s3_client_dart: $e',
      );
    }
    if (result.exitCode != 0) {
      throw StateError(
        'Could not build the Go S3 library. Install Go 1.25.5+ and a C compiler.\n${result.stdout}\n${result.stderr}',
      );
    }
    output.assets.code.add(
      CodeAsset(
        package: input.packageName,
        name: 'src/s3_ffi_bindings.dart',
        linkMode: DynamicLoadingBundled(),
        file: out,
      ),
    );
  });
}
