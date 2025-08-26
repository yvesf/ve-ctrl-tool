package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/peterh/liner"

	"github.com/yvesf/ve-ctrl-tool/cmd"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
)

func main() {
	flag.CommandLine.Usage = help

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	adapter := cmd.CommonInit(ctx)
	defer adapter.Wait()
	defer adapter.Shutdown()

	line := liner.NewLiner()
	defer line.Close()

	// if arguments passed then execute as command
	if args := flag.Args(); len(args) > 0 {
		if err := execute(ctx, adapter, args); err != nil {
			slog.Error("failed", slog.Any("err", err))
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
			inputTokens, err := shellwords.Parse(response)
			if err != nil {
				slog.Error("failed to parse input", slog.Any("err", err))
				continue
			}
			if len(inputTokens) == 0 {
				continue
			}
			if len(inputTokens) == 1 && inputTokens[0] == `quit` {
				cancel()

				break
			}
			err = execute(ctx, adapter, inputTokens)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			if err == nil {
				line.AppendHistory(response)
			}
		} else if errors.Is(err, liner.ErrPromptAborted) {
			fmt.Printf("Send EOF (CTRL-D) or execute 'quit' to exit\n")
			continue
		} else if errors.Is(err, io.EOF) {
			fmt.Printf("\n")
			cancel()
			break
		} else {
			slog.Error("error reading line", slog.Any("err", err))
		}
	}
	slog.Info("start shutdown")
}

func execute(ctx context.Context, mk2 *mk2.Adapter, tokens []string) error {
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
			return fmt.Errorf("command failed %v: %w", tokens, err)
		}
		return nil
	}
	return fmt.Errorf("command not found: %v", tokens[0])
}
