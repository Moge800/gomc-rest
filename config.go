package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
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
	Host         string
	Port         int
	Mode         mc.Mode
	ModeString   string
	Listen       string
	ReadOnly     bool
	EnableRemote bool
	Frame        PLCFrame
	Transport    PLCTransport
	QueueSize    int
	Timeout      time.Duration
	LogFile      string
	LogLevel     slog.Level
	LogFileLevel slog.Level
	Token        string
}

func parseConfig(args []string, lookupEnv func(string) string, output io.Writer) (ServerConfig, error) {
	fs := flag.NewFlagSet("gomc-rest", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	fs.SetOutput(output)

	host := fs.String("host", getenvWith(lookupEnv, "GOMCR_HOST", "192.168.0.1"), "PLC host")
	portStr := fs.String("port", getenvWith(lookupEnv, "GOMCR_PORT", "5007"), "PLC port")
	modeStr := fs.String("mode", getenvWith(lookupEnv, "GOMCR_MODE", "binary"), "PLC mode (binary|ascii)")
	listen := fs.String("listen", getenvWith(lookupEnv, "GOMCR_LISTEN", ":8080"), "HTTP listen address")
	readonly := fs.Bool("readonly", false, "disable write and remote-control endpoints")
	enableRemote := fs.Bool("enable-remote", false, "enable remote-control endpoints (run/stop/pause/latch-clear/reset)")
	frameStr := fs.String("frame", getenvWith(lookupEnv, "GOMCR_FRAME", string(frame3E)), "MC Protocol frame (3e|4e)")
	transportStr := fs.String("transport", getenvWith(lookupEnv, "GOMCR_TRANSPORT", string(transportTCP)), "PLC transport (tcp|udp)")
	queueSizeStr := fs.String("queue-size", getenvWith(lookupEnv, "GOMCR_QUEUE_SIZE", "32"), "PLC communication queue size")
	timeoutStr := fs.String("timeout", getenvWith(lookupEnv, "GOMCR_TIMEOUT", "5s"), "PLC communication timeout")
	logFile := fs.String("log-file", getenvWith(lookupEnv, "GOMCR_LOG_FILE", ""), "path to log file (empty = console only)")
	logLevelStr := fs.String("log-level", getenvWith(lookupEnv, "GOMCR_LOG_LEVEL", "info"), "terminal log level (debug|info|warn|error)")
	logFileLevelStr := fs.String("log-file-level", getenvWith(lookupEnv, "GOMCR_LOG_FILE_LEVEL", "warn"), "file log level (debug|info|warn|error); only used with -log-file")
	token := fs.String("token", getenvWith(lookupEnv, "GOMCR_TOKEN", ""), "static bearer token for request auth (empty = no auth); overrides GOMCR_TOKEN. Visible in the process list")

	if err := fs.Parse(args); err != nil {
		return ServerConfig{}, err
	}

	var readonlySet, enableRemoteSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "readonly":
			readonlySet = true
		case "enable-remote":
			enableRemoteSet = true
		}
	})
	if !readonlySet {
		readonlyDefault, err := getenvBoolWith(lookupEnv, "GOMCR_READONLY", false)
		if err != nil {
			return ServerConfig{}, err
		}
		*readonly = readonlyDefault
	}
	if !enableRemoteSet {
		enableRemoteDefault, err := getenvBoolWith(lookupEnv, "GOMCR_ENABLE_REMOTE", false)
		if err != nil {
			return ServerConfig{}, err
		}
		*enableRemote = enableRemoteDefault
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

	logLevel, err := parseLogLevel(*logLevelStr)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("invalid log-level %q: must be debug, info, warn, or error", *logLevelStr)
	}

	logFileLevel, err := parseLogLevel(*logFileLevelStr)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("invalid log-file-level %q: must be debug, info, warn, or error", *logFileLevelStr)
	}

	return ServerConfig{
		Host:         *host,
		Port:         port,
		Mode:         mode,
		ModeString:   *modeStr,
		Listen:       normalizeListen(*listen),
		ReadOnly:     *readonly,
		EnableRemote: *enableRemote,
		Frame:        frame,
		Transport:    transport,
		QueueSize:    queueSize,
		Timeout:      timeout,
		LogFile:      *logFile,
		LogLevel:     logLevel,
		LogFileLevel: logFileLevel,
		Token:        *token,
	}, nil
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown level %q", s)
	}
}

func getenvWith(lookupEnv func(string) string, key, fallback string) string {
	if v := lookupEnv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBoolWith(lookupEnv func(string) string, key string, fallback bool) (bool, error) {
	v := lookupEnv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback, fmt.Errorf("invalid %s %q: must be a boolean (true/false or 1/0)", key, v)
	}
	return b, nil
}

// normalizeListen prepends ":" to a bare port number so both "8080" and ":8080" work.
func normalizeListen(s string) string {
	if !strings.Contains(s, ":") {
		return ":" + s
	}
	return s
}
