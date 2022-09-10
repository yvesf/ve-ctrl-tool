package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/bsm/openmetrics/omhttp"
	"github.com/google/shlex"
	"github.com/peterh/liner"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"ve-ctrl-tool/victron"
	"ve-ctrl-tool/victron/veBus"
)

var (
	flagSerialDevice = flag.String("serialDevice", "/dev/ttyUSB0", "Device")
	flagLow          = flag.Bool("low", false, "Do not attempt to upgrade to 115200 baud")
	flagVEAddress    = flag.Int("veAddress", 0, "Set other address than 0")
	flagDebug        = flag.Bool("debug", false, "Set log level to debug")
	flagTrace        = flag.Bool("trace", false, "Set log level to trace (overrides -debug)")
	flagMetricsHTTP  = flag.String("metricsHTTP", "", "Address of a http server serving metrics under /metrics")
)

func main() {
	flag.CommandLine.Usage = help
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	if *flagTrace {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Metrics HTTP endpoint
	if *flagMetricsHTTP != `` {
		mux := http.NewServeMux()
		mux.Handle("/metrics", omhttp.NewHandler(openmetrics.DefaultRegistry()))

		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", *flagMetricsHTTP)
		if err != nil {
			log.Fatal().Err(err).Str("addr", *flagMetricsHTTP).Msg("Listen on http failed")
		}

		srv := &http.Server{Handler: mux}
		go func() {
			err := srv.Serve(ln)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("http server failed")
			}
		}()
	}

	mk2, err := victron.NewMk2(*flagSerialDevice)
	if err != nil {
		panic(err)
	}

	// reset both in high and low speed
	err = mk2.SetBaudHigh()
	if err != nil {
		panic(err)
	}
	mk2.Write(veBus.CommandR.Frame().Marshal())
	time.Sleep(time.Second * 1)
	err = mk2.SetBaudLow()
	if err != nil {
		panic(err)
	}

	mk2Ctx, mk2CtxCancel := context.WithCancel(context.Background())
	err = mk2.StartReader(mk2Ctx)
	if err != nil {
		panic(err)
	}
	defer mk2.Wait()
	defer mk2CtxCancel()

	// The following is supposed to switch the MK3 adapter to High-Speed mode.
	// This is undocumented and may break, therefore the switch to skip it.
	if !*flagLow {
		err := mk2.UpgradeHighSpeed()
		if err != nil {
			panic(err)
		}
	}

	err = mk2.SetAddress(ctx, byte(*flagVEAddress))
	if err != nil {
		panic(err)
	}

	line := liner.NewLiner()
	defer line.Close()

	// if arguments passed then execute as command
	if args := flag.Args(); len(args) > 0 {
		if err := execute(ctx, mk2, args); err != nil {
			log.Error().Err(err).Msg("failed")
		}
		return
	}

	// otherwise: start repl
	line.SetCtrlCAborts(true)
	line.SetCompleter(func(line string) (c []string) {
		for _, comm := range commands {
			if strings.HasPrefix(comm.command, line) {
				c = append(c, comm.command)
			}
		}
		return c
	})

	for ctx.Err() == nil {
		if response, err := line.Prompt("Mk2> "); err == nil {
			inputTokens, err := shlex.Split(response)
			if err != nil {
				log.Error().Err(err).Msg("failed to parse input")
				continue
			}
			if len(inputTokens) == 0 {
				continue
			}
			if len(inputTokens) == 1 && inputTokens[0] == `quit` {
				cancel()

				break
			}
			err = execute(ctx, mk2, inputTokens)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			if err == nil {
				line.AppendHistory(response)
			}
		} else if err == liner.ErrPromptAborted {
			fmt.Printf("Send EOF (CTRL-D) or execute 'quit' to exit\n")
			continue
		} else if err == io.EOF {
			fmt.Printf("\n")
			cancel()
			break
		} else {
			log.Error().Err(err).Msg("error reading line")
		}
	}
	log.Info().Msg("start shutdown")
}

func execute(ctx context.Context, mk2 *victron.Mk2, tokens []string) error {
	for _, comm := range commands {
		if comm.command != tokens[0] {
			continue
		}
		if comm.args != 0 && comm.args != len(tokens)-1 {
			return fmt.Errorf("invalid number of arguments for command %v, expected %v got %v",
				comm.command, comm.args, len(tokens)-1)
		}
		err := comm.fun(ctx, mk2, tokens[1:]...)
		if err != nil {
			return fmt.Errorf("command failed %v: %v", tokens, err)
		}
		return nil
	}
	return fmt.Errorf("command not found: %v", tokens[0])
}
