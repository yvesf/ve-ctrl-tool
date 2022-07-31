# ve-ctrl-tool (Victron Energy VE.Bus MK2 protocol tool)

this is a commandline tool to interact with Victron (https://www.victronenergy.com/) devices
over the Mk3 adapter.

## Usage

Interactive mode:

```shell
$ go run .
Mk2> read-ram 1
value=14 value=0b1110 value=0xe
Mk2> (exit with EOF / CTRL-D)
```

Commandline invocation:

```shell
$ go run . read-ram 1
value=14 value=0b1110 value=0xe
```

Run the `help` command to get a list of commands.

## Failing to talk to ESS Assistant

Connect

```
Mk2> state
{"level":"debug","time":"2022-07-31T18:30:04+02:00","message":"CommandGetSetDeviceState setState=0x0"}
{"level":"debug","time":"2022-07-31T18:30:04+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x0e 0x00 0x00 0x97  ] "}
{"level":"debug","time":"2022-07-31T18:30:04+02:00","message":"received Frame data [ 0x07 0xff 0x56 0x24 0xdb 0x11 0x00 0x00  ]"}
{"level":"debug","time":"2022-07-31T18:30:05+02:00","message":"received Frame data [ 0x05 0xff 0x57 0x94 0x08 0x01  ]"}
{"level":"debug","time":"2022-07-31T18:30:05+02:00","message":"CommandGetSetDeviceState state=bypass subState=bulk"}
device state bypass bulk
```

Identify the assistant:

```
Mk2> read-ram 128
{"level":"debug","time":"2022-07-31T18:51:13+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x30 0x80 0x00 0xf5  ] "}
{"level":"debug","time":"2022-07-31T18:51:14+02:00","message":"received frame: [ 0x07 0xff 0x57 0x85 0x54 0x00 0x03 0x5a  ]"}
value=84 value=0b1010100 value=0x54
```

We can interpret these values according to https://www.victronenergy.com/live/ess:ess_mode_2_and_3:
`0x0054` means Assistent ID 5 (ESS ID) and 4 following ram IDs (ID_Size).


Writing the set point with +200 Watt fails:
```
{"level":"debug","time":"2022-07-31T18:54:04+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x32 0x81 0x00 0xf2  ] "}
{"level":"debug","time":"2022-07-31T18:54:04+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x34 0xc8 0x00 0xa9  ] "}
{"level":"debug","time":"2022-07-31T18:54:05+02:00","message":"received frame: [ 0x04 0xff 0x57 0x80 0x00  ]"}
{"level":"error","error":"write-ram failed: write failed","time":"2022-07-31T18:54:05+02:00","message":"Command failed [write-ram 129 200 0]"}
```
However, these are the exact bytes from the _ESS Mode 2 and 3_ document!

We can try the CommandWriteViaID command instead:
```
Mk2> write-ram-id 129 200 0
{"level":"debug","time":"2022-07-31T18:59:19+02:00","message":"sent 9 bytes: [ 0x07 0xff 0x57 0x37 0x00 0x81 0xc8 0x00 0x23  ] "}
{"level":"debug","time":"2022-07-31T18:59:20+02:00","message":"received frame: [ 0x03 0xff 0x57 0x87  ]"}
```
`0x87` indicates OK.

Something I can hear the device making some coil-humming noise after the command for ca. 0.5s.

The read on the same setting returns a different value:
```
Mk2> write-ram-id 129 200 0
{"level":"debug","time":"2022-07-31T19:03:40+02:00","message":"sent 9 bytes: [ 0x07 0xff 0x57 0x37 0x00 0x81 0xc8 0x00 0x23  ] "}
{"level":"debug","time":"2022-07-31T19:03:42+02:00","message":"received frame: [ 0x03 0xff 0x57 0x87  ]"}
Mk2> read-ram 129
{"level":"debug","time":"2022-07-31T19:03:44+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x30 0x81 0x00 0xf4  ] "}
{"level":"debug","time":"2022-07-31T19:03:45+02:00","message":"received frame: [ 0x07 0xff 0x57 0x85 0x14 0x00 0xa2 0x59  ]"}
value=20 value(signed)=20 value=0b10100 value=0x14
Mk2> write-ram-id 129 230 0
{"level":"debug","time":"2022-07-31T19:03:55+02:00","message":"sent 9 bytes: [ 0x07 0xff 0x57 0x37 0x00 0x81 0xe6 0x00 0x05  ] "}
{"level":"debug","time":"2022-07-31T19:03:56+02:00","message":"received frame: [ 0x03 0xff 0x57 0x87  ]"}
Mk2> read-ram 129
{"level":"debug","time":"2022-07-31T19:03:58+02:00","message":"sent 7 bytes: [ 0x05 0xff 0x57 0x30 0x81 0x00 0xf4  ] "}
{"level":"debug","time":"2022-07-31T19:04:00+02:00","message":"received frame: [ 0x07 0xff 0x57 0x85 0xf4 0xff 0x40 0x59  ]"}
value=65524 value(signed)=-12 value=0b1111111111110100 value=0xfff4
```

no clue what is going there 🤷