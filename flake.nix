{
  inputs.nixpkgs.url = "nixpkgs/nixos-22.11";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    let
      packageDef = { buildGoModule }: buildGoModule {
        pname = "ve-ctrl-tool";
        version = "0.0.1";
        src = ./.;
        vendorSha256 = "sha256-aPxfZDTjXAW1WLT26rYTkQDaLeWH9uekah1rCJqycio=";
      };
    in
    flake-utils.lib.eachDefaultSystem
      (system:
        rec {
          packages = { ve-ctrl-tool = nixpkgs.legacyPackages.${system}.callPackage packageDef { }; };
          defaultPackage = packages.ve-ctrl-tool;
          devShell =
            with import nixpkgs { inherit system; }; mkShell {
              packages = [ go nixpkgs-fmt golangci-lint ];
            };
        }) // {
      nixosModule = { pkgs, config, lib, ... }:
        let
          ve-ctrl-tool = pkgs.callPackage packageDef { };
        in
        {
          options.services.ve-ess-shelly = {
            enable = lib.mkEnableOption "the multiplus + shelly controller";
            shellyUrl = lib.mkOption {
              type = lib.types.str;
              default = "http://10.1.0.210";
              description = "Base URL of the shelly device";
            };
            metricsAddress = lib.mkOption {
              type = lib.types.str;
              default = "127.0.0.1:18001";
              description = "address to listen on for serving /metrics requests";
            };
            serialDevice = lib.mkOption {
              type = lib.types.str;
              default = "/dev/ttyUSB0";
              description = "MK3 device";
            };
            maxCharge = lib.mkOption {
              type = lib.types.nullOr lib.types.int;
              default = null;
            };
            maxInverter = lib.mkOption {
              type = lib.types.nullOr lib.types.int;
              default = null;
            };
            maxInverterPeak = lib.mkOption {
              type = lib.types.nullOr lib.types.int;
              default = null;
            };
          };
          config = let cfg = config.services.ve-ess-shelly; in
            lib.mkIf cfg.enable {
              systemd.services.ve-ess-shelly = {
                description = "the multiplus + shelly controller";
                path = [ ve-ctrl-tool ];
                wantedBy = [ "default.target" ];
                script = ''
                  ve-ess-shelly \
                    -metricsHTTP "${cfg.metricsAddress}" \
                    ${lib.optionalString (cfg.maxCharge != null) "-maxCharge ${toString cfg.maxCharge}"} \
                    ${lib.optionalString (cfg.maxInverter != null) "-maxInverter ${toString cfg.maxInverter}"} \
                    ${lib.optionalString (cfg.maxInverterPeak != null) "-maxInverterPeak ${toString cfg.maxInverterPeak}"} \
                    "${cfg.shellyUrl}"
                '';
                serviceConfig = {
                  LockPersonality = true;
                  CapabilityBoundingSet = "";
                  DeviceAllow = "${cfg.serialDevice}";
                  DynamicUser = true;
                  Group = "dialout";
                  MemoryDenyWriteExecute = true;
                  NoNewPrivileges = true;
                  PrivateUsers = true;
                  ProtectClock = true;
                  ProtectControlGroups = true;
                  ProtectHome = true;
                  ProtectHostname = true;
                  ProtectKernelLogs = true;
                  ProtectKernelModules = true;
                  ProtectKernelTunables = true;
                  ProtectProc = true;
                  ProtectSystem = "strict";
                  RemoveIPC = true;
                  Restart = "always";
                  RestrictAddressFamilies = "AF_INET AF_INET6";
                  RestrictNamespaces = true;
                  RestrictRealtime = true;
                  RestrictSUIDSGID = true;
                  SystemCallArchitectures = "native";
                  SystemCallErrorNumber = "EPERM";
                  SystemCallFilter = [ "@system-service" ];
                  UMask = "0007";
                };
              };
            };
        };
    };
}
