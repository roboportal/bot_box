package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt"
	"github.com/joho/godotenv"
	"github.com/roboportal/bot_box/pkg/arena"
	"github.com/roboportal/bot_box/pkg/communicator"
	"github.com/roboportal/bot_box/pkg/serial"
)

func main() {

	err := godotenv.Load()

	if err != nil {
		panic(err)
	}

	srvURL := os.Getenv("srv_url")
	publicKey := os.Getenv("public_key")
	secretKey := os.Getenv("secret_key")
	portName := os.Getenv("port_name")
	stunUrls := strings.SplitAfter(os.Getenv("stun_urls"), ",")
	baudRate, err := strconv.ParseInt(os.Getenv("baud_rate"), 10, 32)
	if err != nil {
		panic(err)
	}
	nBots, err := strconv.ParseInt(os.Getenv("n_bots"), 10, 32)
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"publicKey": publicKey,
	})

	tokenString, err := token.SignedString([]byte(secretKey))

	if err != nil {
		panic(err)
	}

	_arena := arena.Factory(stunUrls, tokenString, publicKey, int(nBots), int(videoCodecBitRate), frameFormat, int(videoWidth), int(videoFrameRate))

	shutdownChan := make(chan struct{})

	serialParams := serial.InitParams{
		PortName:     portName,
		BaudRate:     int(baudRate),
		SendChan:     _arena.SerialWrite,
		ReceiveChan:  _arena.SerialRead,
		ShutdownChan: shutdownChan,
	}

	go serial.Init(serialParams)

	communicatorParams := communicator.InitParams{
		PlatformUri:         srvURL,
		ReceiveChan:         _arena.WSRead,
		SendChan:            _arena.WSWrite,
		ReconnectTimeoutSec: 1,
		PingIntervalSec:     1,
		SendTimeoutSec:      5,
		TokenString:         tokenString,
		PublicKey:           publicKey,
		ShutdownChan:        shutdownChan,
	}
	go communicator.Init(communicatorParams)

	time.Sleep(time.Duration(2) * time.Second)

	go _arena.Run()

	select {}
}
