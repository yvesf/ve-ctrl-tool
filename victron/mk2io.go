package victron

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/goburrow/serial"
	"github.com/rs/zerolog/log"

	"ve-ctrl-tool/victron/veBus"
)

type mk2IO struct {
	listenerProduce chan chan []byte
	listenerClose   chan chan []byte

	input        serial.Port
	commandMutex sync.Mutex
	running      chan struct{}
	wg           sync.WaitGroup
	config       serial.Config
}

func NewReader(address string) (*mk2IO, error) {
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

	return &mk2IO{
		config:          config,
		listenerProduce: make(chan chan []byte),
		listenerClose:   make(chan chan []byte),
		input:           port,
		commandMutex:    sync.Mutex{},
		running:         make(chan struct{}),
	}, nil
}

func (r *mk2IO) SetBaudHigh() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	r.config.BaudRate = 115200
	return r.input.Open(&r.config)
}

func (r *mk2IO) SetBaudLow() error {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	r.config.BaudRate = 2400
	return r.input.Open(&r.config)
}

// ReadAndWrite write a command and return the response
// StartReader must have been called once before
func (r *mk2IO) ReadAndWrite(ctx context.Context, data []byte, receiver func([]byte) bool) ([]byte, error) {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()

	var done = make(chan struct{})
	defer close(done)

	var response = make(chan []byte)
	go func() {
		var l = r.newListenChannel()
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
				} else {
					log.Trace().Hex("frame.data", frame).Msg("dropping while waiting for response")
				}
			case <-done: // timeout
				return
			}
		}
	}()

	select {
	case frame := <-response:
		return frame, nil
	case <-time.After(time.Second * 2):
		return nil, errors.New("WriteAndReadFrame timed out waiting for response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StartReader runs the go-routines that read from the port in the background
func (r *mk2IO) StartReader(ctx context.Context) error {
	var listeners []chan []byte
	var frames = make(chan []byte)
	var wait = make(chan struct{})
	var waitOnce = sync.Once{}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		var l = make(chan []byte)
		for {
			select {
			case <-ctx.Done():
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
					log.Trace().Hex("data", f[2:]).Msgf("received broadcast frame 'V'")
				} else {
					log.Debug().Hex("data", f).Msg("received frame")
					for _, l := range listeners {
						select {
						case l <- f:
						case <-time.After(time.Millisecond * 100):
							log.Warn().Msg("timeout signalling listener")
						}
					}
				}
			}
		}
	}()

	r.wg.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		defer r.wg.Done()
		defer close(frames)

		var synchronized = false
		var frameBuf = make([]byte, 1024)
		var scannerBuffer bytes.Buffer
		for ctx.Err() == nil {
			n, err := r.input.Read(frameBuf)
			if err != nil {
				log.Warn().Msgf("Error reading: %v", err)
				continue
			}
			if n == 0 {
				continue
			}
			log.Trace().Hex("data", frameBuf[0:n]).Msgf("Read %v bytes", n)
			_, _ = scannerBuffer.Write(frameBuf[0:n])

			if scannerBuffer.Len() == 0 {
				continue
			}

			// wait for at least 7 bytes in buffer before trying to sync
			for !synchronized && scannerBuffer.Len() >= 9 {
				if scannerBuffer.Bytes()[1] != 0xff {
					_, _ = scannerBuffer.ReadByte()
					continue
				} else if length := scannerBuffer.Bytes()[0]; scannerBuffer.Len() < int(length) {
					_, _ = scannerBuffer.ReadByte()
					continue
				} else if veBus.Checksum(scannerBuffer.Bytes()[0:length+1]) == scannerBuffer.Bytes()[length+1] {
					log.Debug().Msg("synchronized")
					synchronized = true
					// to wait for sync  before returning from StartReader
					waitOnce.Do(func() { close(wait) })

					break
				} else {
					_, _ = scannerBuffer.ReadByte()
					continue
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
				log.Warn().Msgf("received 0x%x instead of 0xff marker, trigger re-sync", scannerBuffer.Bytes()[1])
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			if scannerBuffer.Len() < int(length)+3 {
				continue // fill buffer first
			}

			potentialFrame := scannerBuffer.Bytes()[0 : length+2]
			if cksum := veBus.Checksum(potentialFrame[0 : length+1]); cksum != potentialFrame[length+1] {
				log.Warn().Msgf("checksum mismatch, got 0x%x, expected 0x%x, trigger re-sync", cksum, potentialFrame[length+1])
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			fullFrame := scannerBuffer.Next(int(length) + 2) // drop successful read data
			f := fullFrame[:length+1]

			select {
			case <-ctx.Done():
			case frames <- f:
			}
		}
		log.Debug().Msg("reader exits")
	}()

	select {
	case <-wait: // for first broadcast
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second * 50):
		cancel()
		return errors.New("could not do initial sync")
	}
}

// Write calculates the checksum and writes the frame to the port
func (r *mk2IO) Write(data []byte) {
	n, err := r.input.Write(data)
	if err != nil {
		panic(err) // todo
	}
	log.Debug().Hex("data", data).Msgf("sent %v bytes ", n)
}

func (r *mk2IO) Close(l chan []byte) {
	for {
		select {
		case r.listenerClose <- l:
			return
		case <-l: // drop remaining frames
		}
	}
}

// Wait blocks until all reader go-routines finished
// Shutdown is initiated by cancelling the context passed to StartReader
func (r *mk2IO) Wait() {
	r.wg.Wait()
}

func (r *mk2IO) newListenChannel() chan []byte {
	return <-r.listenerProduce
}

func (r *mk2IO) UpgradeHighSpeed() error {
	time.Sleep(time.Millisecond * 100)

	n, err := r.input.Write([]byte{0x02, 0xff, 0x4e, 0xb1})
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

	n, err = r.input.Write([]byte("UUUUU"))
	if err != nil {
		return fmt.Errorf("failed write UUUUU: %w", err)
	}
	if n != 5 {
		return fmt.Errorf("failed write UUUUU: incomplete write")
	}

	time.Sleep(time.Millisecond * 100)

	return nil
}
