{
  description = "glossary: a vocabulary store for words and phrases met while reading";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      forAllSystems = nixpkgs.lib.genAttrs [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "glossary";
            version = "0.0.0";
            src = self;
            subPackages = [ "cmd/glossary" ];
            vendorHash = null; # standard library only
          };
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go # the toolchain: go run / build / test / fmt / vet / mod
              pkgs.gopls # language server, only needed by your editor
            ];
          };
        }
      );
    };
}
