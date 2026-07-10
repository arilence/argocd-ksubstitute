{
  description = "Argo CD config management plugin for substituting variables in manifests";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.gomod2nix.url = "github:nix-community/gomod2nix";
  inputs.gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.gomod2nix.inputs.flake-utils.follows = "flake-utils";

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      gomod2nix,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        callPackage = pkgs.callPackage;

        gomod2nixPkgs = gomod2nix.legacyPackages.${system};

        ksubstituteApp = callPackage ./. {
          inherit (gomod2nixPkgs) buildGoApplication;
        };

        cmpConfig = pkgs.runCommand "ksubstitute-cmp-config" { } ''
          install -Dm444 ${./config.yaml} \
            $out/home/argocd/cmp-server/config/plugin.yaml
        '';

        # Lint check that reuses gomod2nix's vendor setup.
        go-lint = pkgs.stdenvNoCC.mkDerivation {
          name = "go-lint";
          src = ./.;

          dontBuild = true;
          doCheck = true;

          nativeBuildInputs = [
            pkgs.golangci-lint
            ksubstituteApp.passthru.go
            pkgs.writableTmpDirAsHomeHook

            # This is the important bit: reuse gomod2nix's hook.
            ksubstituteApp.passthru.hooks.goConfigHook
          ];

          # This is also important: point the hook at the vendor env
          # generated from gomod2nix.toml.
          goVendorDir = ksubstituteApp.passthru.vendorEnv;

          checkPhase = ''
            runHook preCheck

            export GOLANGCI_LINT_CACHE="$TMPDIR/golangci-lint-cache"
            golangci-lint run ./...

            runHook postCheck
          '';

          installPhase = ''
            mkdir -p "$out"
          '';
        };

        containerImage = pkgs.dockerTools.buildLayeredImage {
          name = "ksubstitute";
          tag = "latest";

          contents = [
            # Uncomment if you want a usable shell inside the container
            # pkgs.busybox
            pkgs.cacert
            pkgs.openssl
            ksubstituteApp
            cmpConfig
          ];

          config = {
            Cmd = [ "/bin/ksubstitute" ];
          };
        };
      in
      {
        checks = {
          # This builds the app and runs its Go tests via gomod2nix.
          ksubstitute = ksubstituteApp;

          # This lints with gomod2nix's vendored deps available.
          inherit go-lint;
        };

        packages = {
          default = ksubstituteApp;
          inherit containerImage;
        };

        devShells.default = callPackage ./shell.nix {
          inherit (gomod2nixPkgs) mkGoEnv gomod2nix;
        };

        apps.build-and-load = {
          type = "app";
          program = "${pkgs.writeShellScriptBin "build-and-load" ''
            nix build .#containerImage
            docker load < result
            echo "Container image loaded"
          ''}/bin/build-and-load";
          meta.description = "Build and load the container image into Docker";
        };
      }
    );
}
