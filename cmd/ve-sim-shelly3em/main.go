// package main implements a simulator for a shelly em 3
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
	"golang.org/x/exp/slog"
)

var (
	flagListenAddr = flag.String("l", "0.0.0.0:8082", "Address (host:port) to listen on")
	currentValue   = int64(0)
)

func main() {
	flag.Parse()

	server := http.Server{
		Addr: *flagListenAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			var doc shelly.Gen2MeterData
			doc.TotalPowerFloat = float64(atomic.LoadInt64(&currentValue))

			err := json.NewEncoder(w).Encode(doc)
			if err != nil {
				slog.Error("failed to encode json response", slog.Any("err", err))
			}
		}),
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGINT)
	defer cancel()

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	reader := bufio.NewReader(os.Stdin)
	for ctx.Err() == nil {
		fmt.Printf("TotalPower=> ")
		l, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			slog.Info("shutdown")
			cancel()
			break
		}
		if err != nil {
			slog.Error("failed to read line", slog.Any("err", err))
			continue
		}
		l = strings.TrimSpace(l)
		value, err := strconv.ParseInt(l, 10, 64)
		if err != nil {
			slog.Error("failed to parse line", slog.Any("err", err))
			continue
		}
		atomic.StoreInt64(&currentValue, value)
	}

	wg.Wait()
}
