package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/goburrow/serial"
	"github.com/google/shlex"
	"github.com/peterh/liner"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	flagAddress      = flag.String("serialAddress", "/dev/ttyUSB0", "Device")
	flagInitialReset = flag.Bool("reset", false, "Do a reset before start operation")
	flagVEAddress    = flag.Int("veAddress", -1, "If set then select this address on startup")
	flagDebug        = flag.Bool("debug", false, "Set log level to debug")
	flagTrace        = flag.Bool("trace", false, "Set log level to trace (overrides -debug)")
)

func mustOpenMk2() Mk2 {
	config := serial.Config{}
	config.Address = *flagAddress
	config.BaudRate = 2400
	config.DataBits = 8
	config.Parity = "N"
	config.StopBits = 1
	config.Timeout = 5 * time.Second

	port, err := serial.Open(&config)
	if err != nil {
		panic(err)
	}

	return Mk2{NewReader(port)}
}

func main() {
	flag.Parse()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	if *flagTrace {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	if f := flag.Args(); len(f) == 1 && f[0] == "help" {
		help()
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	mk2 := mustOpenMk2()

	if *flagInitialReset {
		f := transportFrame{data: []byte{0x02, 0xff, 'R'}}
		mk2.Write(f)
		time.Sleep(time.Second * 1)
	}

	mk2Ctx, mk2CtxCancel := context.WithCancel(context.Background())
	err := mk2.StartReader(mk2Ctx)
	if err != nil {
		panic(err)
	}
	defer mk2.Wait()
	defer mk2CtxCancel()

	if *flagVEAddress > 0 {
		err = mk2.SetAddress(ctx, byte(*flagVEAddress))
		if err != nil {
			panic(err)
		}
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

func execute(ctx context.Context, mk2 Mk2, tokens []string) error {
	for _, comm := range commands {
		if comm.command != tokens[0] {
			continue
		}
		if comm.args != len(tokens)-1 {
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
