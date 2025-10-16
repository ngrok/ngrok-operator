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
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            go-tools
            golangci-lint
            gotools
            jq
            kubebuilder
            kubernetes-helm
            kyverno-chainsaw
            nixfmt-rfc-style
            yq
          ];

          CGO_ENABLED = "0";
          # GitHub Codespaces sets GOROOT in /etc/environment. However, we are managing
          # go via nix, so we need to unset it to avoid conflicts. See also: https://dave.cheney.net/2013/06/14/you-dont-need-to-set-goroot-really
          GOROOT = "";
        };
      }
    ));
}
