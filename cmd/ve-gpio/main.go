package main

import (
	"os"
	"strconv"
	"time"

	"github.com/warthog618/gpiod"
)

func main() {
	gpio, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		panic(err)
	}
	println(gpio)

	state, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		panic(err)
	}
	print(state)

	g, err := gpiod.RequestLine("gpiochip0", int(gpio), gpiod.AsOutput(0))
	if err != nil {
		panic(err)
	}
	defer g.Close()

	err = g.SetValue(int(state))
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 3)
}
