{
  description = "kswitch — the kubectx for operators: reproducible development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      # Keep this list in sync with the platforms we build in .goreleaser.yaml / the Makefile.
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          name = "kswitch-dev";

          packages = with pkgs; [
            # Go toolchain. go.mod pins `go 1.26.4`; this provides 1.26.x and Go's
            # GOTOOLCHAIN=auto fetches the exact patch release on first build if needed.
            go_1_26

            # Build / lint / release tooling used by the Makefile and CI.
            gnumake # `make build`, `make check`, `make format`, `make test`
            gotools # provides `goimports` (hack/format.sh)
            golangci-lint # matches GOLANGCI_LINT_VERSION (v2.12.2) in hack/tools.mk
            addlicense # license-header check (`make check`)
            ginkgo # BDD test runner used under pkg/.../*_suite_test.go
            goreleaser # release artifacts (.goreleaser.yaml)

            # Editor / debugging convenience.
            gopls
            delve

            git
          ];

          # Ensure module builds work even if a contributor has GOTOOLCHAIN=local set
          # globally, since go.mod requests a slightly newer patch than nixpkgs ships.
          shellHook = ''
            export GOTOOLCHAIN=auto
            echo "kswitch dev shell — $(go version | awk '{print $3, $4}')"
            echo "  run 'make build' to cross-compile binaries into hack/switch/"
          '';
        };
      });

      # `nix fmt` formats the flake itself.
      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
