package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	mc "github.com/moge800/gomcprotocol"
)

type PLCFrame string

const (
	frame3E PLCFrame = "3e"
	frame4E PLCFrame = "4e"
)

type PLCTransport string

const (
	transportTCP PLCTransport = "tcp"
	transportUDP PLCTransport = "udp"
)

type ServerConfig struct {
	Host       string
	Port       int
	Mode       mc.Mode
	ModeString string
	Listen     string
	ReadOnly   bool
	Frame      PLCFrame
	Transport  PLCTransport
	QueueSize  int
	Timeout    time.Duration
}

func parseConfig(args []string, lookupEnv func(string) string, output io.Writer) (ServerConfig, error) {
	fs := flag.NewFlagSet("gomc-rest", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	fs.SetOutput(output)

	host := fs.String("host", getenvWith(lookupEnv, "PLC_HOST", "192.168.0.1"), "PLC host")
	portStr := fs.String("port", getenvWith(lookupEnv, "PLC_PORT", "5007"), "PLC port")
	modeStr := fs.String("mode", getenvWith(lookupEnv, "PLC_MODE", "binary"), "PLC mode (binary|ascii)")
	listen := fs.String("listen", getenvWith(lookupEnv, "LISTEN_ADDR", ":8080"), "HTTP listen address")
	readonly := fs.Bool("readonly", getenvBoolWith(lookupEnv, "READONLY", false), "disable write and remote-control endpoints")
	frameStr := fs.String("frame", getenvWith(lookupEnv, "PLC_FRAME", string(frame3E)), "MC Protocol frame (3e|4e)")
	transportStr := fs.String("transport", getenvWith(lookupEnv, "PLC_TRANSPORT", string(transportTCP)), "PLC transport (tcp|udp)")
	queueSizeStr := fs.String("queue-size", getenvWith(lookupEnv, "QUEUE_SIZE", "32"), "PLC communication queue size")
	timeoutStr := fs.String("timeout", getenvWith(lookupEnv, "PLC_TIMEOUT", "5s"), "PLC communication timeout")

	if err := fs.Parse(args); err != nil {
		return ServerConfig{}, err
	}

	port, err := strconv.Atoi(*portStr)
	if err != nil || port < 1 || port > 65535 {
		return ServerConfig{}, fmt.Errorf("invalid port %q: must be 1 to 65535", *portStr)
	}

	var mode mc.Mode
	switch *modeStr {
	case "binary":
		mode = mc.ModeBinary
	case "ascii":
		mode = mc.ModeASCII
	default:
		return ServerConfig{}, fmt.Errorf("invalid mode %q: must be binary or ascii", *modeStr)
	}

	frame := PLCFrame(*frameStr)
	switch frame {
	case frame3E, frame4E:
	default:
		return ServerConfig{}, fmt.Errorf("invalid frame %q: must be 3e or 4e", *frameStr)
	}

	transport := PLCTransport(*transportStr)
	switch transport {
	case transportTCP, transportUDP:
	default:
		return ServerConfig{}, fmt.Errorf("invalid transport %q: must be tcp or udp", *transportStr)
	}
	if frame == frame4E && transport == transportUDP {
		return ServerConfig{}, errors.New("invalid transport \"udp\": frame 4e supports tcp only")
	}

	queueSize, err := strconv.Atoi(*queueSizeStr)
	if err != nil || queueSize <= 0 {
		return ServerConfig{}, fmt.Errorf("invalid queue-size %q: must be greater than 0", *queueSizeStr)
	}

	timeout, err := time.ParseDuration(*timeoutStr)
	if err != nil || timeout <= 0 {
		return ServerConfig{}, fmt.Errorf("invalid timeout %q: must be a positive duration", *timeoutStr)
	}

	return ServerConfig{
		Host:       *host,
		Port:       port,
		Mode:       mode,
		ModeString: *modeStr,
		Listen:     *listen,
		ReadOnly:   *readonly,
		Frame:      frame,
		Transport:  transport,
		QueueSize:  queueSize,
		Timeout:    timeout,
	}, nil
}

func getenvWith(lookupEnv func(string) string, key, fallback string) string {
	if v := lookupEnv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBoolWith(lookupEnv func(string) string, key string, fallback bool) bool {
	v := lookupEnv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("invalid %s %q: must be a boolean (true/false or 1/0)", key, v)
	}
	return b
}
