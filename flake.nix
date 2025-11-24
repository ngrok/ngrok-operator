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

        mkScript = name: text: let
          script = pkgs.writeShellScriptBin name text;
        in script;

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
          buildInputs = with pkgs; [
            go_1_24
            go-tools
            golangci-lint
            gotools
            jq
            kind
            kubebuilder
            kubectl
            kubernetes-helm
            kyverno-chainsaw
            ngrok
            nixfmt-rfc-style
            tilt
            yq
            readmeGeneratorForHelm
          ] ++ scripts;

          CGO_ENABLED = "0";
          # GitHub Codespaces sets GOROOT in /etc/environment. However, we are managing
          # go via nix, so we need to unset it to avoid conflicts. See also: https://dave.cheney.net/2013/06/14/you-dont-need-to-set-goroot-really
          GOROOT = "";

          shellHook = ''
            devhelp
          '';
        };
      }
    ));
}
