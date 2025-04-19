{
  inputs.nixpkgs.url = "nixpkgs/nixos-24.11";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    let
      packageDef = { buildGo123Module }: buildGo123Module {
        pname = "ve-ctrl-tool";
        version = "0.0.1";
        src = ./.;
        vendorHash = "sha256-zwJ13y2B8NluCv8IQhQp6k08wedxb+Y2kKFO0k2SOVc=";
      };
    in
    flake-utils.lib.eachDefaultSystem
      (system:
        rec {
          packages = { ve-ctrl-tool = nixpkgs.legacyPackages.${system}.callPackage packageDef { }; };
          defaultPackage = packages.ve-ctrl-tool;
          devShell =
            with import nixpkgs { inherit system; }; mkShell {
              packages = [ go_1_23 nixpkgs-fmt golangci-lint gofumpt ];
            };
        }) // {
      nixosModule = { pkgs, config, lib, ... }:
        let ve-ctrl-tool = pkgs.callPackage packageDef { }; in
        {
          options.services.ve-ess-shelly = {
            enable = lib.mkEnableOption "the multiplus + shelly controller";
            shellyEM3 = lib.mkOption {
              type = lib.types.str;
              default = "10.1.0.210";
              description = "Address of the shelly EM 3 Energy Meter";
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
          config =
            let
              cfg = config.services.ve-ess-shelly;
            in
            lib.mkIf cfg.enable {
              environment.systemPackages = [ ve-ctrl-tool ]; # add to system because it's handy for debugging
              systemd.services.ve-ess-shelly = {
                description = "the multiplus + shelly controller";
                wantedBy = [ "default.target" ];
                unitConfig = {
                  StartLimitInterval = 0;
                };
                serviceConfig = {
                  ExecStart = ''
                    ${ve-ctrl-tool}/bin/ve-ess-shelly \
                      -metricsHTTP "${cfg.metricsAddress}" \
                      ${lib.optionalString (cfg.maxCharge != null) "-maxCharge ${toString cfg.maxCharge}"} \
                      ${lib.optionalString (cfg.maxInverter != null) "-maxInverter ${toString cfg.maxInverter}"} \
                      ${lib.optionalString (cfg.maxInverterPeak != null) "-maxInverterPeak ${toString cfg.maxInverterPeak}"} \
                      "${cfg.shellyEM3}"
                  '';
                  LockPersonality = true;
                  CapabilityBoundingSet = "";
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
                  ProtectProc = "noaccess";
                  ProtectSystem = "strict";
                  RemoveIPC = true;
                  Restart = "always";
                  RestartSec = "10s";
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
