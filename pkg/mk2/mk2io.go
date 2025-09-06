package mk2

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/goburrow/serial"

	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"
)

type Port interface {
	io.ReadWriter
	SetSpeed(baud int) error
}

type FilePort struct {
	io.ReadWriter
}

func (f *FilePort) SetSpeed(speed int) error {
	slog.Info("skip setting speed on file", slog.Int("speed", speed))
	return nil
}

// PipedPort returns two instances of FilePort which are connected
// to each other using io.Pipe().
func PipedPort() (*FilePort, *FilePort) {
	clientReader, simulatorWriter := io.Pipe()
	simulatorReader, clientWriter := io.Pipe()

	clientReadWriter := struct {
		io.Reader
		io.Writer
	}{clientReader, clientWriter}
	simulatorReadWriter := struct {
		io.Reader
		io.Writer
	}{simulatorReader, simulatorWriter}

	return &FilePort{ReadWriter: clientReadWriter}, &FilePort{ReadWriter: simulatorReadWriter}
}

type SerialPort struct {
	port   serial.Port
	config serial.Config
	lock   sync.RWMutex
}

func NewSerialPort(address string) (*SerialPort, error) {
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

	return &SerialPort{
		port:   port,
		config: config,
	}, nil
}

func (s *SerialPort) Read(buf []byte) (int, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.port.Read(buf)
}

func (s *SerialPort) Write(buf []byte) (int, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.port.Write(buf)
}

func (s *SerialPort) SetSpeed(baud int) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.config.BaudRate = baud
	return s.port.Open(&s.config)
}

// IO provides raw read/write to MK2-Adapter.
type IO struct {
	Port
	listenerProduce chan chan []byte
	listenerClose   chan chan []byte

	commandMutex sync.Mutex
	ctx          context.Context
	ctxCancel    context.CancelFunc
	running      bool
	wg           sync.WaitGroup
}

func NewReader(port Port) *IO {
	return &IO{
		Port:            port,
		listenerProduce: make(chan chan []byte),
		listenerClose:   make(chan chan []byte),
	}
}

func (r *IO) SetBaudHigh() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	return r.SetSpeed(115200)
}

func (r *IO) SetBaudLow() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	return r.SetSpeed(2400)
}

// ReadAndWrite write a command and return the response
// StartReader must be called before calling ReadAndWrite.
func (r *IO) ReadAndWrite(ctx context.Context, data []byte, receiver func([]byte) bool) ([]byte, error) {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()

	if !r.running {
		return nil, fmt.Errorf("IO is not running, call StartReader first")
	}

	done := make(chan struct{})
	defer close(done)

	response := make(chan []byte)
	go func() {
		l := r.NewListenChannel()
		defer close(response)
		defer r.Close(l)

		r.WriteContext(ctx, data)
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
				slog.DebugContext(ctx, "dropping while waiting for response", slog.Any("frame.data", frame))
			case <-done: // timeout
				return
			}
		}
	}()

	select {
	case frame := <-response:
		return frame, nil
	case <-time.After(time.Second * 2):
		return nil, errors.New("ReadAndWrite timed out waiting for response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StartReader runs the go-routines that reads from the port in the background.
func (r *IO) StartReader(ctx context.Context, waitForPackets bool) error {
	var listeners []chan []byte
	frames := make(chan []byte)
	wait := make(chan struct{})
	waitOnce := sync.Once{}

	r.commandMutex.Lock()
	if r.running {
		r.commandMutex.Unlock()
		return fmt.Errorf("already running")
	}
	r.ctx, r.ctxCancel = context.WithCancel(ctx)
	r.running = true
	r.commandMutex.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		l := make(chan []byte)
		for {
			select {
			case <-r.ctx.Done():
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
			case f, ok := <-frames:
				if !ok {
					return
				}
				if len(f) == 8 && f[2] == 'V' {
					slog.DebugContext(r.ctx, "received broadcast frame 'V'",
						slog.Any("frame", hex.EncodeToString(f)))
				} else {
					slog.DebugContext(r.ctx, "received bytes",
						slog.Any("frame", hex.EncodeToString(f)),
						slog.Int("listeners", len(listeners)))
					for _, l := range listeners {
						select {
						case l <- f:
						case <-time.After(time.Millisecond * 100):
							slog.WarnContext(r.ctx, "timeout signalling listener")
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
			n, err := r.Read(frameBuf)
			if err != nil {
				slog.WarnContext(ctx, fmt.Sprintf("Error reading: %v", err))
				return
			}
			if n == 0 {
				continue
			}
			_, _ = scannerBuffer.Write(frameBuf[0:n])
			slog.DebugContext(r.ctx, fmt.Sprintf("Read %v new bytes", n),
				slog.Any("newBytes", hex.EncodeToString(frameBuf[0:n])),
				slog.Any("scannerBufHex", hex.EncodeToString(scannerBuffer.Bytes())))

			if scannerBuffer.Len() == 0 {
				continue
			}

			// wait for at least 7 bytes in buffer before trying to sync
			for !synchronized && scannerBuffer.Len() >= 7 {
				slog.DebugContext(r.ctx, "re-syncing", slog.Bool("synchronized", synchronized))
				if scannerBuffer.Bytes()[1] != 0xff {
					_, _ = scannerBuffer.ReadByte()
				} else if length := scannerBuffer.Bytes()[0]; scannerBuffer.Len() < int(length) {
					break // read more data
				} else if vebus.Checksum(scannerBuffer.Bytes()[0:length+1]) == scannerBuffer.Bytes()[length+1] {
					slog.DebugContext(r.ctx, "synchronized", slog.Any("scannerBufHex", hex.EncodeToString(scannerBuffer.Bytes())))
					synchronized = true
					// to wait for sync  before returning from StartReader
					waitOnce.Do(func() { close(wait) })

					break
				} else {
					// drop byte, try again
					c, _ := scannerBuffer.ReadByte()
					slog.DebugContext(r.ctx, "dropped byte", slog.Any("byte", c))
				}
			}
			if !synchronized {
				continue // read more data
			}

			for scannerBuffer.Len() >= 3 {
				for scannerBuffer.Len() > 0 && scannerBuffer.Bytes()[0] == 0x00 {
					// drop 0x00 bytes
					_ = scannerBuffer.Next(1)
				}
				for scannerBuffer.Len() >= 5 &&
					bytes.Equal(scannerBuffer.Bytes()[0:5], []byte("UUUUU")) {
					// only relevant when acting as mk2 server.
					slog.DebugContext(r.ctx, "drop 'UUUUU' magic")
					_ = scannerBuffer.Next(5)
				}
				if scannerBuffer.Len() < 3 {
					continue
				}

				length := scannerBuffer.Bytes()[0]
				if scannerBuffer.Bytes()[1] != 0xff {
					slog.WarnContext(r.ctx, fmt.Sprintf("received 0x%x instead of 0xff marker, trigger re-sync", scannerBuffer.Bytes()[1]))
					synchronized = false
					scannerBuffer.Reset()
					break
				}

				if scannerBuffer.Len() < int(length+2) {
					slog.Debug("buffer too small, wait for more data")
					break
				}

				potentialFrame := scannerBuffer.Bytes()[0 : length+2]
				if cksum := vebus.Checksum(potentialFrame[0 : length+1]); cksum != potentialFrame[length+1] {
					slog.WarnContext(r.ctx, fmt.Sprintf("checksum mismatch, got 0x%x, expected 0x%x, trigger re-sync",
						cksum, potentialFrame[length+1]))
					synchronized = false
					scannerBuffer.Reset()
					break
				}

				fullFrame := scannerBuffer.Next(int(length) + 2) // drop successful read data
				f := fullFrame[:length+1]

				select {
				case <-r.ctx.Done():
				case frames <- f:
				}
			}
		}
		slog.Debug("reader exits")
	}()

	if !waitForPackets {
		return nil
	}

	select {
	case <-wait: // for first broadcast
		return nil
	case <-r.ctx.Done(): // shutdown during init
		return nil
	case <-time.After(time.Second * 50): // timeout
		r.Shutdown()
		return errors.New("could not do initial sync")
	}
}

// Write calculates the checksum and writes the frame to the port.
func (r *IO) WriteContext(ctx context.Context, data []byte) {
	n, err := r.Write(data)
	if err != nil {
		panic(err) // todo
	}
	slog.DebugContext(ctx, "sent bytes", slog.Int("len", n), slog.Any("data", hex.EncodeToString(data)))
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
	slog.DebugContext(r.ctx, "try shutdown")
	r.commandMutex.Lock()
	if r.running {
		slog.DebugContext(r.ctx, "trigger shutdown")
		r.ctxCancel()
		r.running = false
	}
	r.commandMutex.Unlock()
}

// Wait blocks until all reader go-routines finished.
// Shutdown is initiated by calling Shutdown().
func (r *IO) Wait() {
	r.wg.Wait()
}

func (r *IO) NewListenChannel() chan []byte {
	return <-r.listenerProduce
}

func (r *IO) UpgradeHighSpeed() error {
	time.Sleep(time.Millisecond * 100)

	n, err := r.Write([]byte{0x02, 0xff, 0x4e, 0xb1})
	if err != nil {
		return fmt.Errorf("failed magic high-speed sequence: %w", err)
	}
	if n != 4 {
		return fmt.Errorf("failed magic high-speed sequence: incomplete write")
	}

	time.Sleep(time.Millisecond * 50)

	err = r.SetBaudHigh()
	if err != nil {
		return fmt.Errorf("failed to set high baud rate: %w", err)
	}

	n, err = r.Write([]byte("UUUUU"))
	if err != nil {
		return fmt.Errorf("failed write UUUUU: %w", err)
	}
	if n != 5 {
		return fmt.Errorf("failed write UUUUU: incomplete write")
	}

	time.Sleep(time.Millisecond * 100)

	return nil
}

type Frame[ResultT any] interface {
	Marshal() []byte
	ParseResponse([]byte) *ResultT
}

func WriteAndRead[ResultT any](ctx context.Context, io *IO, frame Frame[ResultT]) (*ResultT, error) {
	var response *ResultT
	_, err := io.ReadAndWrite(ctx, frame.Marshal(), func(d []byte) bool {
		response = frame.ParseResponse(d)
		return response != nil
	})
	return response, err
}
