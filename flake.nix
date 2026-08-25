{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    (flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfreePredicate = pkg: pkg.pname == "ngrok";
          };
        };

        # Tools that parse our source with their own embedded go/types must be
        # built with the same Go toolchain we target. A tool built with go1.26
        # rejects go1.27 source outright ("method must have no type parameters"
        # for generic methods; golangci-lint refuses to start at all). Drop
        # these overrides once nixpkgs builds them with go 1.27 by default.
        goVersion = pkgs.go_1_27;

        # Each package names its Go builder differently, so the override arg
        # differs too; nixpkgs pins golangci-lint to a specific Go version on
        # purpose (see the comment in its package.nix).
        #
        # Building against go 1.27 is necessary but not sufficient: golangci-lint
        # 2.12.2 vendors honnef.co/go/tools v0.7.0 (staticcheck 2026.1), whose IR
        # builder panics on go 1.27 syntax ("unexpected expr: *ast.KeyValueExpr"
        # -- embedded-field struct initializers) while analyzing the go 1.27
        # stdlib, which takes down the whole run. 2.13.1 vendors v0.8.0
        # (staticcheck 2026.2), the first release to support go 1.27.
        #
        # nixpkgs master already has 2.13.1; nixpkgs-unstable does not yet. Drop
        # this whole binding -- override and overrideAttrs both -- once it lands,
        # since master also renames the builder arg to buildGo127Module.
        golangci-lint =
          (pkgs.golangci-lint.override { buildGo126Module = pkgs.buildGo127Module; }).overrideAttrs
            (_: {
              version = "2.13.1";
              src = pkgs.fetchFromGitHub {
                owner = "golangci";
                repo = "golangci-lint";
                tag = "v2.13.1";
                hash = "sha256-8nWHSMAwIILfKMPfxWKMimxWt9N+kUsZEAaoAOPbRBE=";
              };
              vendorHash = "sha256-yZRqfht5rY2yyoZNtYttE57sB7EYjk71yrKw8dLYzNk=";
            });
        kubernetes-controller-tools = pkgs.kubernetes-controller-tools.override {
          buildGoModule = pkgs.buildGo127Module;
        };
        # goimports parses our source too, and `go` here is the toolchain it
        # wraps itself with. Note: go-tools (staticcheck) is deliberately NOT
        # rebuilt against go 1.27 -- its own test suite fails under 1.27 for the
        # same reason golangci-lint's vendored copy does (see .golangci.yml).
        gotools = pkgs.gotools.override {
          buildGoModule = pkgs.buildGo127Module;
          go = pkgs.go_1_27;
        };

        readmeGeneratorForHelm = pkgs.buildNpmPackage {
          pname = "readme-generator-for-helm";
          version = "2.6.1";

          src = pkgs.fetchFromGitHub {
            owner = "bitnami";
            repo = "readme-generator-for-helm";
            rev = "2.6.1";
            hash = "sha256-hgVSiYOM33MMxVlt36aEc0uBWIG/OS0l7X7ZYNESO6A=";
          };

          npmDepsHash = "sha256-baRBchp4dBruLg0DoGq7GsgqXkI/mBBDowtAljC2Ckk=";
          dontNpmBuild = true;
        };

        mkScript =
          name: text:
          let
            script = pkgs.writeShellScriptBin name text;
          in
          script;

        scripts = [
          (mkScript "devhelp" ''
            cat <<'EOF'

            Welcome to the ngrok-operator development environment!

            Please make sure you have the following environment variables set:

              NGROK_API_KEY      - Your ngrok API key
              NGROK_AUTHTOKEN    - Your ngrok authtoken

            If you are using GitHub Codespaces, a kind cluster should
            already be running. You can verify this by running:

              kind get clusters

            Common commands:
              make build          - Build the operator
              make test           - Run tests
              make lint           - Run linters
              make deploy         - Deploy to the kind cluster

            For more information, see the development documentation in

              ./docs/developer-guide/README.md

            You can also run "devhelp" at any time to see this message again.
            EOF
          '')
        ];
      in
      {
        packages.readme-generator-for-helm = readmeGeneratorForHelm;

        devShells.default = pkgs.mkShell {
          buildInputs =
            with pkgs;
            [
              bashInteractive
              goVersion
              go-tools
              golangci-lint
              gotools
              jq
              kind
              kubebuilder
              kubectl
              kubernetes-controller-tools
              (pkgs.wrapHelm pkgs.kubernetes-helm {
                plugins = [
                  pkgs.kubernetes-helmPlugins.helm-unittest
                ];
              })
              kyverno-chainsaw
              ngrok
              nixfmt
              setup-envtest
              tilt
              yq
              readmeGeneratorForHelm
            ]
            ++ scripts;

          CGO_ENABLED = "0";
          # GitHub Codespaces sets GOROOT in /etc/environment. However, we are managing
          # go via nix, so we need to unset it to avoid conflicts. See also: https://dave.cheney.net/2013/06/14/you-dont-need-to-set-goroot-really
          GOROOT = "";

          ENVTEST_K8S_VERSION = "1.34.1";

          shellHook = ''
            export KUBEBUILDER_ASSETS="$(setup-envtest use $ENVTEST_K8S_VERSION -p path)"
            devhelp
          '';
        };
      }
    ));
}
