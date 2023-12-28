{ pkgs ? (
    let
      inherit (builtins) fetchTree fromJSON readFile;
      inherit ((fromJSON (readFile ./flake.lock)).nodes) nixpkgs gomod2nix;
    in
    import (fetchTree nixpkgs.locked) {
      overlays = [
        (import "${fetchTree gomod2nix.locked}/overlay.nix")
      ];
    }
  )
, mkGoEnv ? pkgs.mkGoEnv
, gomod2nix ? pkgs.gomod2nix
}:

let
  goEnv = mkGoEnv { pwd = ./.; };
  emacs = (pkgs.emacs.pkgs.withPackages (epkgs: (with epkgs.melpaStablePackages; [
      go-mode
    ])));
in
pkgs.mkShell {
  buildInputs = with pkgs; [
    emacs
    go
    gopls
    gotools
    go-tools
    goEnv
    gomod2nix
  ];
}
