package main

import (
	"os"
	"strconv"
	"strings"

	jwt "github.com/golang-jwt/jwt"
	"github.com/joho/godotenv"

	"github.com/roboportal/bot_box/pkg/arena"
	"github.com/roboportal/bot_box/pkg/communicator"
	"github.com/roboportal/bot_box/pkg/consoleoutput"
	"github.com/roboportal/bot_box/pkg/ipc"
	"github.com/roboportal/bot_box/pkg/serial"
)

const (
	Serial  = "serial"
	Console = "console"
	IPC     = "ipc"
)

func main() {

	err := godotenv.Load()

	if err != nil {
		panic(err)
	}

	srvURL := os.Getenv("srv_url")
	publicKey := os.Getenv("public_key")
	secretKey := os.Getenv("secret_key")

	outputMode := os.Getenv("output_mode")
	portName := os.Getenv("port_name")

	if outputMode != Serial && outputMode != IPC && outputMode != Console {
		panic("output_mode param has wrong value")
	}

	stunUrls := strings.SplitAfter(os.Getenv("stun_urls"), ",")
	baudRate, err := strconv.ParseInt(os.Getenv("baud_rate"), 10, 32)

	if err != nil {
		panic(err)
	}

	videoCodecBitRate, err := strconv.ParseInt(os.Getenv("video_codec_bit_rate"), 10, 32)

	if err != nil {
		panic(err)
	}

	frameFormat := os.Getenv("frame_format")

	videoWidth, err := strconv.ParseInt(os.Getenv("video_width"), 10, 32)
	if err != nil {
		panic(err)
	}

	videoFrameRate, err := strconv.ParseInt(os.Getenv("video_frame_rate"), 10, 32)

	if err != nil {
		panic(err)
	}

	isAudioInputEnabled, err := strconv.ParseBool(os.Getenv("audio_input_enabled"))

	if err != nil {
		panic(err)
	}

	isAudioOutputEnabled, err := strconv.ParseBool(os.Getenv("audio_output_enabled"))

	if err != nil {
		panic(err)
	}

	botBoxIPCPort, err := strconv.ParseInt(os.Getenv("bot_box_ipc_port"), 10, 32)

	if err != nil {
		panic(err)
	}

	robotIPCPort, err := strconv.ParseInt(os.Getenv("robot_ipc_port"), 10, 32)

	if err != nil {
		panic(err)
	}

	robotIPCHost := os.Getenv("robot_ipc_host")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"publicKey": publicKey,
	})

	tokenString, err := token.SignedString([]byte(secretKey))

	if err != nil {
		panic(err)
	}

	arenaParams := arena.InitParams{
		StunUrls:    stunUrls,
		TokenString: tokenString,
		PublicKey:   publicKey,

		VideoCodecBitRate: int(videoCodecBitRate),
		FrameFormat:       frameFormat,
		VideoWidth:        int(videoWidth),
		VideoFrameRate:    int(videoFrameRate),

		IsAudioInputEnabled:  isAudioInputEnabled,
		IsAudioOutputEnabled: isAudioOutputEnabled,
	}

	_arena := arena.Factory(arenaParams)

	if outputMode == Serial {
		serialParams := serial.InitParams{
			PortName:    portName,
			BaudRate:    int(baudRate),
			SendChan:    _arena.BotCommandsWriteChan,
			ReceiveChan: _arena.BotCommandsReadChan,
		}

		go serial.Init(serialParams)
	}

	if outputMode == IPC {
		ipcParams := ipc.InitParams{
			BotBoxIPCPort: int(botBoxIPCPort),
			RobotIPCPort:  int(robotIPCPort),
			RobotIPCHost:  robotIPCHost,

			SendChan:    _arena.BotCommandsWriteChan,
			ReceiveChan: _arena.BotCommandsReadChan,
		}

		go ipc.Init(ipcParams)
	}

	if outputMode == Console {
		consoleParams := consoleoutput.InitParams{
			SendChan: _arena.BotCommandsWriteChan,
		}

		go consoleoutput.Init(consoleParams)
	}

	communicatorParams := communicator.InitParams{
		PlatformUri:         srvURL,
		ReceiveChan:         _arena.WSReadChan,
		SendChan:            _arena.WSWriteChan,
		ReconnectTimeoutSec: 3,
		PingIntervalSec:     1,
		SendTimeoutSec:      1,
		TokenString:         tokenString,
		PublicKey:           publicKey,
		ConStatChan:         _arena.WSConStatChan,
	}

	go communicator.Init(communicatorParams)

	go _arena.Run()

	select {}
}
