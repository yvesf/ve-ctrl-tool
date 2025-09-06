{
  inputs.nixpkgs.url = "nixpkgs/nixos-25.05";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    let
      packageDef = { buildGo124Module }: buildGo124Module {
        pname = "ve-ctrl-tool";
        version = "0.0.1";
        src = ./.;
        vendorHash = "sha256-4t026XI7iu0kuZ/+CfaWbtU/1CvLqfy0EO2yoW6I+Rg=";
      };
    in
    flake-utils.lib.eachDefaultSystem
      (system:
        rec {
          packages = { ve-ctrl-tool = nixpkgs.legacyPackages.${system}.callPackage packageDef { }; };
          defaultPackage = packages.ve-ctrl-tool;
          devShell =
            with import nixpkgs { inherit system; }; mkShell {
              packages = [ go_1_24 nixpkgs-fmt golangci-lint golangci-lint-langserver gofumpt ];
            };
        }) // {
      nixosModule = { pkgs, config, lib, ... }:
        let ve-ctrl-tool = pkgs.callPackage packageDef { }; in
        {
          options.services.forecast = {
            enable = lib.mkEnableOption "the ve-ess-shelly sunshine forecast service";
            lat = lib.mkOption {
              type = lib.types.float;
              default = 48.517737;
              description = "latitude WGS-84 geo coordinate";
            };
            lon = lib.mkOption {
              type = lib.types.float;
              default = 7.917704;
              description = "longitude WGS-84 geo coordinate";
            };
          };
          options.services.ve-ess-shelly = {
            enable = lib.mkEnableOption "the multiplus + shelly controller";
            shellyEM3 = lib.mkOption {
              type = lib.types.str;
              default = "10.1.0.210";
              description = "Address of the shelly EM 3 Energy Meter";
            };
            http = lib.mkOption {
              type = lib.types.str;
              default = "127.0.0.1:18001";
              description = "address to listen on for /metrics and /flags";
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
          };
          config =
            let
              cfgForecast = config.services.forecast;
              cfgShelly = config.services.ve-ess-shelly;
            in
            {
              # add to system because it's handy for debugging
              environment.systemPackages = lib.mkIf (cfgForecast.enable or cfgShelly.enable) [ ve-ctrl-tool ];

              systemd.services.forecast = lib.mkIf cfgForecast.enable {
                description = "the ve-ess-shelly forecast";
                wantedBy = [ "default.target" ];
                unitConfig = {
                  StartLimitInterval = 0;
                };
                serviceConfig = {
                  ExecStart = ''
                    ${ve-ctrl-tool}/bin/forecast \
                      -http "http://${cfgShelly.http}" \
                      -lat ${toString cfgForecast.lat} \
                      -lon ${toString cfgForecast.lon}
                  '';
                  LockPersonality = true;
                  CapabilityBoundingSet = "";
                  DynamicUser = true;
                  Group = "dialout";
                  MemoryDenyWriteExecute = true;
                  NoNewPrivileges = true;
                  PrivateDevices = true;
                  PrivateUsers = true;
                  ProcSubset = "pid";
                  ProtectClock = true;
                  ProtectControlGroups = true;
                  ProtectHome = true;
                  ProtectHostname = true;
                  ProtectKernelLogs = true;
                  ProtectKernelModules = true;
                  ProtectKernelTunables = true;
                  ProtectProc = "invisible";
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
                  UMask = "0022";
                };
              };
              systemd.services.ve-ess-shelly = lib.mkIf cfgShelly.enable {
                description = "the multiplus + shelly controller";
                wantedBy = [ "default.target" ];
                unitConfig = {
                  StartLimitInterval = 0;
                };
                serviceConfig = {
                  ExecStart = ''
                    ${ve-ctrl-tool}/bin/ve-ess-shelly \
                      -http "${cfgShelly.http}" \
                      ${lib.optionalString (cfgShelly.maxCharge != null) "-maxCharge ${toString cfgShelly.maxCharge}"} \
                      ${lib.optionalString (cfgShelly.maxInverter != null) "-maxInverter ${toString cfgShelly.maxInverter}"} \
                      "${cfgShelly.shellyEM3}"
                  '';
                  LockPersonality = true;
                  CapabilityBoundingSet = "";
                  DynamicUser = true;
                  Group = "dialout";
                  MemoryDenyWriteExecute = true;
                  NoNewPrivileges = true;
                  PrivateUsers = true;
                  ProcSubset = "pid";
                  ProtectClock = true;
                  ProtectControlGroups = true;
                  ProtectHome = true;
                  ProtectHostname = true;
                  ProtectKernelLogs = true;
                  ProtectKernelModules = true;
                  ProtectKernelTunables = true;
                  ProtectProc = "invisible";
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
                  SystemCallFilter = [ "@system-service" "@~privileged" ];
                  UMask = "0022";
                };
              };
            };
        };
    };
}
