package cmd

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/bsm/openmetrics/omhttp"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

var (
	flagSerialDevice = flag.String("serialDevice", "/dev/ttyUSB0", "Device")
	flagLow          = flag.Bool("low", false, "Do not attempt to upgrade to 115200 baud")
	flagVEAddress    = flag.Int("veAddress", 0, "Set other address than 0")
	flagDebug        = flag.Bool("debug", false, "Set log level to debug")
	flagMetricsHTTP  = flag.String("metricsHTTP", "", "Address of a http server serving metrics under /metrics")
)

func CommonInit(ctx context.Context) *mk2.Adapter {
	flag.Parse()

	logLevel := slog.LevelInfo
	if *flagDebug {
		logLevel = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(h))

	// Metrics HTTP endpoint
	if *flagMetricsHTTP != `` {
		mux := http.NewServeMux()
		mux.Handle("/metrics", omhttp.NewHandler(openmetrics.DefaultRegistry()))

		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", *flagMetricsHTTP)
		if err != nil {
			slog.Error("Listen on http failed", slog.String("addr", *flagMetricsHTTP))
			os.Exit(1)
		}

		srv := &http.Server{Handler: mux}
		go func() {
			err := srv.Serve(ln)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http server failed", slog.Any("err", err))
				os.Exit(1)
			}
		}()
	}

	mk2, err := mk2.NewAdapter(*flagSerialDevice)
	if err != nil {
		panic(err)
	}

	// reset both in high and low speed
	err = mk2.SetBaudHigh()
	if err != nil {
		panic(err)
	}
	mk2.Write(vebus.CommandR.Frame().Marshal())
	time.Sleep(time.Second * 1)
	err = mk2.SetBaudLow()
	if err != nil {
		panic(err)
	}

	// The following is supposed to switch the MK3 adapter to High-Speed mode.
	// This is undocumented and may break, therefore the switch to skip it.
	if !*flagLow {
		err := mk2.UpgradeHighSpeed()
		if err != nil {
			panic(err)
		}
	}

	err = mk2.StartReader()
	if err != nil {
		panic(err)
	}

	err = mk2.SetAddress(ctx, byte(*flagVEAddress))
	if err != nil {
		panic(err)
	}

	return mk2
}
