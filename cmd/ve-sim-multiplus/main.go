package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/containerd/console"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
)

var flagDebug = flag.Bool("debug", false, "Set log level to debug")

func main() {
	logLevel := slog.LevelInfo
	if *flagDebug {
		logLevel = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(h))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Open a new PTY as a virtual serial port.
	c, slave, err := console.NewPty()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	slog.Info("pty", slog.String("slave", slave))

	err = c.SetRaw()
	if err != nil {
		panic(err)
	}

	go func() {
		<-ctx.Done()
		c.Close()
	}()

	port := &mk2.FilePort{
		ReadWriter: c,
	}
	simulator := mk2.Simulator{
		Port: port,
	}
	simulator.Run(ctx)
}
