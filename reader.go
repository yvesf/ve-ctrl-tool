package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/goburrow/serial"
	"github.com/rs/zerolog/log"
)

type transportFrame struct {
	data []byte
}

type mk2IO struct {
	listenerProduce chan chan *transportFrame
	listenerClose   chan chan *transportFrame

	input        io.ReadWriter
	commandMutex sync.Mutex
	running      chan struct{}
	wg           sync.WaitGroup
}

func NewReader(port serial.Port) *mk2IO {
	return &mk2IO{
		listenerProduce: make(chan chan *transportFrame),
		listenerClose:   make(chan chan *transportFrame),
		input:           port,
		commandMutex:    sync.Mutex{},
		running:         make(chan struct{}),
	}
}

// WriteAndReadFrame write a command and return the response
// StartReader must have been called once before
func (r *mk2IO) WriteAndReadFrame(ctx context.Context, command byte, data ...byte) (*transportFrame, error) {
	r.commandMutex.Lock()
	defer r.commandMutex.Unlock()
	length := byte(1 + 1 + len(data))

	var done = make(chan struct{})
	defer close(done)

	var response = make(chan *transportFrame)
	go func() {
		var l = r.newListenChannel()
		defer close(response)
		defer r.Close(l)

		var message []byte
		message = append(message, length, 0xff, command)
		message = append(message, data...)
		r.Write(transportFrame{data: message})
		for {
			select {
			case frame := <-l:
				if frame.data[2] == command {
					select {
					case response <- frame:
					case <-done:
					}
					return
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
	var listeners []chan *transportFrame
	var frames = make(chan *transportFrame)
	var wait = make(chan struct{})
	var waitOnce = sync.Once{}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		var l = make(chan *transportFrame)
		for {
			select {
			case <-ctx.Done():
				close(l)
				return
			case r.listenerProduce <- l:
				listeners = append(listeners, l)
				l = make(chan *transportFrame)
			case unregL := <-r.listenerClose:
				for i := range listeners {
					if listeners[i] == unregL {
						listeners = append(listeners[:i], listeners[i+1:]...)
						close(unregL)
						break
					}
				}
			case f := <-frames:
				for _, l := range listeners {
					select {
					case l <- f:
					case <-time.After(time.Millisecond * 100):
						log.Warn().Msg("timeout signalling listener")
					}
				}

				if len(f.data) == 8 && f.data[2] == 'V' {
					log.Trace().Msgf("received broadcase frame 'V': %v", hexArray(f.data[2:]))
				} else {
					log.Debug().Msgf("received frame: %v", hexArray(f.data))
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
				log.Printf("Error reading: %v", err)
				continue
			}
			if n == 0 {
				continue
			}
			log.Trace().Msgf("Read: %v", hexArray(frameBuf[0:n]))
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
				} else if checksum(scannerBuffer.Bytes()[0:length+1]) == scannerBuffer.Bytes()[length+1] {
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
				log.Printf("received 0x%x instead of 0xff marker, trigger re-sync", scannerBuffer.Bytes()[1])
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			if scannerBuffer.Len() < int(length)+3 {
				continue // fill buffer first
			}

			potentialFrame := scannerBuffer.Bytes()[0 : length+2]
			if cksum := checksum(potentialFrame[0 : length+1]); cksum != potentialFrame[length+1] {
				log.Printf("checksum mismatch, got 0x%x, expected 0x%x, trigger re-sync", cksum, potentialFrame[length+1])
				synchronized = false
				scannerBuffer.Reset()
				continue
			}

			fullFrame := scannerBuffer.Next(int(length) + 2) // drop successful read data
			f := &transportFrame{data: fullFrame[:length+1]}

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
func (r *mk2IO) Write(f transportFrame) {
	data := append(f.data[:], checksum(f.data))
	n, err := r.input.Write(data)
	if err != nil {
		panic(err) // todo
	}
	log.Debug().Msgf("sent %v bytes: %v ", n, hexArray(data))
}

func (r *mk2IO) Close(l chan *transportFrame) {
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

func (r *mk2IO) newListenChannel() chan *transportFrame {
	return <-r.listenerProduce
}

// checksum implements the check-summing algorithm for ve.bus
func checksum(data []byte) byte {
	var sum = byte(0)
	for _, d := range data {
		sum += d
	}
	checksum := 255 - (sum % 255) + 1
	return checksum
}

// hexArray returns a byte-slice more readable as hex-formatted values string
func hexArray(data []byte) string {
	var buf = new(strings.Builder)
	_, _ = fmt.Fprintf(buf, "[ ")
	for _, c := range data {
		_, _ = fmt.Fprintf(buf, "0x%02x ", c)
	}
	_, _ = fmt.Fprintf(buf, " ]")
	return buf.String()
}
