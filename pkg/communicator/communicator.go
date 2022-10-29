package communicator

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	disconnected = "disconnected"
	connected    = "connected"
	connecting   = "connecting"
)

type ACommunicator struct {
	status              string
	platformUri         string
	receiveChan         chan string
	sendChan            chan string
	reconnectTimeoutSec int
	pingIntervalSec     int
	sendTimeoutSec      int
	tokenString         string
	publicKey           string
	conn                *websocket.Conn
	doReconnect         chan struct{}
	stopSending         chan struct{}
	stopReceiving       chan struct{}
	conStatChan         chan string
	awaitingPong        bool
}

type InitParams struct {
	PlatformUri         string
	ReceiveChan         chan string
	SendChan            chan string
	ReconnectTimeoutSec int
	PingIntervalSec     int
	SendTimeoutSec      int
	TokenString         string
	PublicKey           string
	ConStatChan         chan string
}

func factory(p InitParams) ACommunicator {
	return ACommunicator{
		status:              disconnected,
		platformUri:         p.PlatformUri,
		receiveChan:         p.ReceiveChan,
		sendChan:            p.SendChan,
		reconnectTimeoutSec: p.ReconnectTimeoutSec,
		pingIntervalSec:     p.PingIntervalSec,
		sendTimeoutSec:      p.SendTimeoutSec,
		tokenString:         p.TokenString,
		publicKey:           p.PublicKey,
		doReconnect:         make(chan struct{}),
		stopSending:         make(chan struct{}),
		stopReceiving:       make(chan struct{}),
		conStatChan:         p.ConStatChan,
		awaitingPong:        false,
	}
}

func (comm *ACommunicator) setConnecting() {
	log.Println("WS connecting")
	comm.status = connecting
	comm.conStatChan <- connecting
}

func (comm *ACommunicator) setConnected() {
	log.Println("WS connected")
	comm.status = connected
	comm.conStatChan <- connected
}

func (comm *ACommunicator) setDisconnected() {
	log.Println("WS disconnected")
	comm.status = disconnected
	comm.conStatChan <- disconnected
}

func (comm *ACommunicator) isConnected() bool {
	return comm.status == connected
}

func (comm *ACommunicator) isConnecting() bool {
	return comm.status == connecting
}

// Init setups ws connection
func Init(p InitParams) {
	comm := factory(p)

	headers := http.Header{"x-public-key": {comm.publicKey}}
	tickerP := time.NewTicker(time.Duration(comm.pingIntervalSec) * time.Second)

	defer (func() {
		comm.setDisconnected()
		tickerP.Stop()
		comm.awaitingPong = false
		if comm.conn != nil {
			comm.conn.Close()
		}
		time.Sleep(time.Duration(comm.reconnectTimeoutSec) * time.Second)
		Init(p)
	})()

	comm.setConnecting()

	log.Println("Connecting to the platform.")

	c, _, err := websocket.DefaultDialer.Dial(comm.platformUri, headers)

	if err != nil {
		log.Println("Connection failed error:", err)
		return
	}

	comm.conn = c

	c.SetPongHandler(func(d string) error {
		comm.awaitingPong = false
		return nil
	})

	comm.setConnected()

	log.Println("Connected to the platform.")

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			if !comm.isConnected() {
				continue
			}

			_, message, err := c.ReadMessage()

			if err != nil {
				log.Println("WS receiving error", err)
				return
			}

			if message != nil {
				comm.receiveChan <- string(message)
			}
		}
	}()

	tickerP.Reset(time.Duration(comm.pingIntervalSec) * time.Second)

	for {
		select {
		case <-done:
			return

		case <-tickerP.C:
			if !comm.isConnected() {
				continue
			}

			if comm.awaitingPong {
				log.Println("Pong was not received")
				return
			}

			c := comm.conn

			err := c.WriteControl(websocket.PingMessage, []byte(comm.publicKey), time.Now().Add(time.Duration(comm.sendTimeoutSec)*time.Second))

			comm.awaitingPong = true

			if err != nil {
				log.Println("WS Ping error:", err)
				return
			}

		case msg := <-comm.sendChan:
			if !comm.isConnected() {
				panic("Trying to send data thru closed socket")
			}

			c := comm.conn
			c.SetWriteDeadline(time.Now().Add(time.Duration(comm.sendTimeoutSec) * time.Second))

			err := c.WriteMessage(websocket.TextMessage, []byte(msg))

			if err != nil {
				log.Println("Send WS data error:", err)
				return
			}
		}
	}

}
