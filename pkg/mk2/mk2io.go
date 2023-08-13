package mk2

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/goburrow/serial"
	"golang.org/x/exp/slog"

	"github.com/yvesf/ve-ctrl-tool/pkg/timemock"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

// IO provides raw read/write to MK2-Adapter.
type IO struct {
	listenerProduce chan chan []byte
	listenerClose   chan chan []byte

	input          serial.Port
	commandMutex   sync.Mutex
	signalShutdown chan struct{}
	running        bool
	wg             sync.WaitGroup
	config         serial.Config
}

func NewReader(address string) (*IO, error) {
	config := serial.Config{}
	config.Address = address
	config.BaudRate = 2400
	config.DataBits = 8
	config.Parity = "N"
	config.StopBits = 1
	config.Timeout = 5 * time.Second

	port, err := serial.Open(&config)
	if err != nil {
		return nil, err
	}

	return &IO{
		config:          config,
		listenerProduce: make(chan chan []byte),
		listenerClose:   make(chan chan []byte),
		input:           port,
		commandMutex:    sync.Mutex{},
	}, nil
}

func (r *IO) SetBaudHigh() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	r.config.BaudRate = 115200
	return r.input.Open(&r.config)
}

func (r *IO) SetBaudLow() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	r.config.BaudRate = 2400
	return r.input.Open(&r.config)
}

// ReadAndWrite write a command and return the response
// StartReader must have been called once before.
func (r *IO) ReadAndWrite(ctx context.Context, data []byte, receiver func([]byte) bool) ([]byte, error) {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()

	done := make(chan struct{})
	defer close(done)

	response := make(chan []byte)
	go func() {
		l := r.newListenChannel()
		defer close(response)
		defer r.Close(l)

		r.Write(data)
		for {
			select {
			case frame := <-l:
				if receiver(frame) {
					select {
					case response <- frame:
					case <-done:
					}
					return
				}
				slog.Debug("dropping while waiting for response", slog.Any("frame.data", frame))
			case <-done: // timeout
				return
			}
		}
	}()

	select {
	case frame := <-response:
		return frame, nil
	case <-timemock.After(time.Second * 2):
		return nil, errors.New("WriteAndReadFrame timed out waiting for response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StartReader runs the go-routines that read from the port in the background.
func (r *IO) StartReader() error {
	var listeners []chan []byte
	frames := make(chan []byte)
	wait := make(chan struct{})
	waitOnce := sync.Once{}

	r.commandMutex.Lock()
	if r.running {
		r.commandMutex.Unlock()
		return fmt.Errorf("already running")
	}
	r.signalShutdown = make(chan struct{})
	r.running = true
	r.commandMutex.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		l := make(chan []byte)
		for {
			select {
			case <-r.signalShutdown:
				close(l)
				return
			case r.listenerProduce <- l:
				listeners = append(listeners, l)
				l = make(chan []byte)
			case unregL := <-r.listenerClose:
				for i := range listeners {
					if listeners[i] == unregL {
						listeners = append(listeners[:i], listeners[i+1:]...)
						close(unregL)
						break
					}
				}
			case f := <-frames:
				if len(f) == 8 && f[2] == 'V' {
					slog.Debug("received broadcast frame 'V'", slog.Any("data", f[2:]))
				} else {
					slog.Debug("received bytes", slog.Any("data", f), slog.Int("len", len(f)))
					for _, l := range listeners {
						select {
						case l <- f:
						case <-timemock.After(time.Millisecond * 100):
							slog.Warn("timeout signalling listener")
						}
					}
				}
			}
		}
	}()

	r.wg.Add(1)
	go func() {
		defer r.Shutdown()
		defer r.wg.Done()
		defer close(frames)

		var scannerBuffer bytes.Buffer
		synchronized := false
		frameBuf := make([]byte, 1024)
		for r.running {
			n, err := r.input.Read(frameBuf)
			if err != nil {
				slog.Warn(fmt.Sprintf("Error reading: %v", err))
				continue
			}
			if n == 0 {
				continue
			}
			slog.Debug(fmt.Sprintf("Read %v bytes", n), slog.Any("data", frameBuf[0:n]))
			_, _ = scannerBuffer.Write(frameBuf[0:n])
			slog.Debug("buffer", slog.Any("scannerBufHex", hex.EncodeToString(scannerBuffer.Bytes())))

			if scannerBuffer.Len() == 0 {
				continue
			}
			for scannerBuffer.Len() > 0 && scannerBuffer.Bytes()[0] == 0x00 {
				// drop 0x00
				_ = scannerBuffer.Next(1)
			}

			// wait for at least 9 bytes in buffer before trying to sync
			for !synchronized && scannerBuffer.Len() >= 9 {
				slog.Debug("re-syncing", slog.Bool("synchronized", synchronized))
				if scannerBuffer.Bytes()[1] != 0xff {
					_, _ = scannerBuffer.ReadByte()
				} else if length := scannerBuffer.Bytes()[0]; scannerBuffer.Len() < int(length) {
					_, _ = scannerBuffer.ReadByte()
				} else if vebus.Checksum(scannerBuffer.Bytes()[0:length+1]) == scannerBuffer.Bytes()[length+1] {
					slog.Debug("synchronized", slog.Any("checksum", scannerBuffer.Bytes()))
					synchronized = true
					// to wait for sync  before returning from StartReader
					waitOnce.Do(func() { close(wait) })

					break
				} else {
					_, _ = scannerBuffer.ReadByte()
				}
			}
			if !synchronized {
				continue
			}
			if scannerBuffer.Len() < 3 {
				continue
			}

			length := scannerBuffer.Bytes()[0]
			if scannerBuffer.Bytes()[1] != 0xff {
				slog.Warn(fmt.Sprintf("received 0x%x instead of 0xff marker, trigger re-sync", scannerBuffer.Bytes()[1]))
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			if scannerBuffer.Len() < int(length)+3 {
				continue // fill buffer first
			}

			potentialFrame := scannerBuffer.Bytes()[0 : length+2]
			if cksum := vebus.Checksum(potentialFrame[0 : length+1]); cksum != potentialFrame[length+1] {
				slog.Warn(fmt.Sprintf("checksum mismatch, got 0x%x, expected 0x%x, trigger re-sync",
					cksum, potentialFrame[length+1]))
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			fullFrame := scannerBuffer.Next(int(length) + 2) // drop successful read data
			f := fullFrame[:length+1]

			select {
			case <-r.signalShutdown:
			case frames <- f:
			}
		}
		slog.Debug("reader exits")
	}()

	select {
	case <-wait: // for first broadcast
		return nil
	case <-r.signalShutdown: // shutdown during init
		return nil
	case <-timemock.After(time.Second * 50): // timeout
		r.Shutdown()
		return errors.New("could not do initial sync")
	}
}

// Write calculates the checksum and writes the frame to the port.
func (r *IO) Write(data []byte) {
	n, err := r.input.Write(data)
	if err != nil {
		panic(err) // todo
	}
	slog.Debug("sent bytes", slog.Int("len", n), slog.Any("data", data))
}

func (r *IO) Close(l chan []byte) {
	for {
		select {
		case r.listenerClose <- l:
			return
		case <-l: // drop remaining frames
		}
	}
}

// Shutdown initiates stop reading.
// Call Wait() to make sure shutdown is completed.
func (r *IO) Shutdown() {
	slog.Debug("try shutdown")
	r.commandMutex.Lock()
	if r.running {
		slog.Debug("trigger shutdown")
		close(r.signalShutdown)
		r.running = false
	}
	r.commandMutex.Unlock()
}

// Wait blocks until all reader go-routines finished.
// Shutdown is initiated by calling Shutdown().
func (r *IO) Wait() {
	r.wg.Wait()
}

func (r *IO) newListenChannel() chan []byte {
	return <-r.listenerProduce
}

func (r *IO) UpgradeHighSpeed() error {
	timemock.Sleep(time.Millisecond * 100)

	n, err := r.input.Write([]byte{0x02, 0xff, 0x4e, 0xb1})
	if err != nil {
		return fmt.Errorf("failed magic high-speed sequence: %w", err)
	}
	if n != 4 {
		return fmt.Errorf("failed magic high-speed sequence: incomplete write")
	}

	timemock.Sleep(time.Millisecond * 50)

	err = r.SetBaudHigh()
	if err != nil {
		return fmt.Errorf("failed to set high baud rate: %w", err)
	}

	n, err = r.input.Write([]byte("UUUUU"))
	if err != nil {
		return fmt.Errorf("failed write UUUUU: %w", err)
	}
	if n != 5 {
		return fmt.Errorf("failed write UUUUU: incomplete write")
	}

	timemock.Sleep(time.Millisecond * 100)

	return nil
}
