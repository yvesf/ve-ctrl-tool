package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/yvesf/ve-ctrl-tool/pkg/logging"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
)

func TestRunController__exitOnCancelledContext(t *testing.T) {
	logging.Init(true)

	simulatorPort, clientPort := mk2.PipedPort()

	simulator := mk2.Simulator{
		Port: simulatorPort,
	}
	go func() {
		ctx := logging.WithAttrs(context.Background(), slog.String("logger", "simulator"))
		simulator.Run(ctx)
	}()

	ctx := logging.WithAttrs(context.Background(), slog.String("logger", "client"))

	mk2Adapter := &mk2.Adapter{IO: mk2.NewReader(clientPort)}

	err := mk2Adapter.StartReader(ctx, true)
	if err != nil {
		panic(err)
	}

	mk2Ess, err := mk2.ESSInit(ctx, mk2Adapter)
	if err != nil {
		panic(err)
	}

	err = mk2Ess.SetpointSet(ctx, 100)
	if err != nil {
		t.Fatalf("setpoint set: %s", err)
	}

	if simulator.RamVars[129] == nil {
		t.Fatal("Ram var was not written")
	}
	if val := *simulator.RamVars[129]; val != 100 {
		t.Fatalf("Wrong value %d in ram var 129", val)
	}
}
