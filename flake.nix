{
  description = "cliamp - Terminal music player inspired by Winamp";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems f;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = self.packages.${system}.cliamp;

          cliamp = pkgs.buildGoModule {
            pname = "cliamp";
            version = "1.27.3-unstable-2026-03-25";
            src = /home/mads/git/cliamp;

            vendorHash = null;

            nativeBuildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.alsa-lib pkgs.libvorbis pkgs.libogg pkgs.flac ];

            env.CGO_ENABLED = if pkgs.stdenv.isLinux then "1" else "0";

            meta = with pkgs.lib; {
              description = "Retro terminal music player inspired by Winamp";
              homepage = "https://github.com/bjarneo/cliamp";
              license = licenses.mit;
              mainProgram = "cliamp";
              platforms = platforms.linux ++ platforms.darwin;
            };
          };
        }
      );

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            buildInputs = [
              pkgs.go
              pkgs.ffmpeg
              pkgs.yt-dlp
            ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.pkg-config
              pkgs.alsa-lib
              pkgs.libvorbis
              pkgs.libogg
              pkgs.flac
            ];

            shellHook = ''
              cd /home/mads/git/cliamp
            '';
          };
        }
      );
    };
}
