{
  description = "vanda の開発に使う道具";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.tlaplus18 pkgs.git pkgs.gnumake pkgs.nodejs pkgs.pnpm pkgs.expect ];
          # Playwright が自分でブラウザを落とす代わりに、nix のブラウザを使う。
          # web/package.json の @playwright/test は、この playwright-driver と同じバージョンにする（バージョンが違うとブラウザが合わない）
          PLAYWRIGHT_BROWSERS_PATH = "${pkgs.playwright-driver.browsers}";
          PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD = "1";
          PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS = "true";
        };
      });
    };
}
