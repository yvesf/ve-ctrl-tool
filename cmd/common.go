package cmd

import (
	"context"
	"flag"
	"time"

	"github.com/yvesf/ve-ctrl-tool/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"

	"github.com/rs/zerolog"
)

var (
	flagSerialDevice = flag.String("serialDevice", "/dev/ttyUSB0", "Device")
	flagLow          = flag.Bool("low", false, "Do not attempt to upgrade to 115200 baud")
	flagVEAddress    = flag.Int("veAddress", 0, "Set other address than 0")
	flagDebug        = flag.Bool("debug", false, "Set log level to debug")
	flagTrace        = flag.Bool("trace", false, "Set log level to trace (overrides -debug)")
)

func CommonInit(ctx context.Context) *mk2.Adapter {
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	if *flagTrace {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
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
