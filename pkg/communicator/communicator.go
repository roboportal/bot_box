package communicator

import (
	"log"
	"net/http"
	"sync"
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
	mu                  sync.Mutex
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

func commFactory(p InitParams) ACommunicator {
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

func (comm *ACommunicator) handleConnection() {
	defer (func() {
		comm.mu.Lock()
		comm.setDisconnected()
		comm.mu.Unlock()

		time.Sleep(time.Duration(comm.reconnectTimeoutSec) * time.Second)

		go comm.handleConnection()
	})()

	headers := http.Header{"x-public-key": {comm.publicKey}}
	tickerP := time.NewTicker(time.Duration(comm.pingIntervalSec) * time.Second)

	defer tickerP.Stop()

	comm.mu.Lock()

	comm.awaitingPong = false
	comm.setConnecting()

	tickerP.Stop()

	log.Println("Connecting to the platform.")

	c, _, err := websocket.DefaultDialer.Dial(comm.platformUri, headers)

	if err != nil {
		log.Println("Connection failed error:", err)

		comm.mu.Unlock()

		return
	}

	comm.conn = c

	defer c.Close()

	c.SetPongHandler(func(d string) error {
		comm.mu.Lock()
		comm.awaitingPong = false
		comm.mu.Unlock()
		return nil
	})

	comm.setConnected()

	go comm.handleSend()
	go comm.handleReceive()

	tickerP.Reset(time.Duration(comm.pingIntervalSec) * time.Second)
	comm.mu.Unlock()

	log.Println("Connected to the platform.")

	for {
		select {

		case <-tickerP.C:
			if !comm.isConnected() {
				continue
			}

			if comm.awaitingPong {
				return
			}

			c := comm.conn

			comm.mu.Lock()
			err := c.WriteControl(websocket.PingMessage, []byte(comm.publicKey), time.Now().Add(time.Duration(comm.sendTimeoutSec)*time.Second))

			comm.awaitingPong = true
			comm.mu.Unlock()

			if err != nil {
				log.Println("WS Ping error:", err)

				if !comm.isConnecting() {
					return
				}
			}
		}
	}
}

func (comm *ACommunicator) handleSend() {
	for {
		select {
		case msg := <-comm.sendChan:
			if !comm.isConnected() {
				continue
			}

			c := comm.conn
			c.SetWriteDeadline(time.Now().Add(time.Duration(comm.sendTimeoutSec) * time.Second))

			comm.mu.Lock()
			err := c.WriteMessage(websocket.TextMessage, []byte(msg))
			comm.mu.Unlock()

			if err != nil {
				log.Println("Send WS data error:", err)
				return
			}
		}
	}
}

func (comm *ACommunicator) handleReceive() {
	for {
		select {
		default:
			if !comm.isConnected() {
				continue
			}

			_, message, err := comm.conn.ReadMessage()

			if err != nil {
				log.Println("WS receiving error", err)
				return
			}

			if message != nil {
				comm.receiveChan <- string(message)
			}
		}
	}
}

// Init setups ws connection
func Init(p InitParams) {
	comm := commFactory(p)

	go comm.handleConnection()

}
