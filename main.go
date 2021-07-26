package main

import (
	"os"
	"strconv"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"publicKey": publicKey,
	})

	tokenString, err := token.SignedString([]byte(secretKey))

	if err != nil {
		panic(err)
	}

	_arena := arena.Factory(stunUrls, tokenString, publicKey, int(nBots))

	go _arena.Run()

	go serial.Init(portName, int(baudRate), _arena.SerialWrite, _arena.SerialRead, 1)
	go communicator.Init(srvURL, _arena.WSRead, _arena.WSWrite, _arena.Disconnect, 1, 1, 1, tokenString, publicKey)

	select {}
}
