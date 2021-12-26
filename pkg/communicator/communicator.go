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
	mu                  sync.Mutex
	shutdownChan        chan struct{}
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
	ShutdownChan        chan struct{}
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
		shutdownChan:        p.ShutdownChan,
	}
}

func (comm *ACommunicator) setConnecting() {
	comm.status = connecting
}

func (comm *ACommunicator) setConnected() {
	comm.status = connected
}

func (comm *ACommunicator) setDisconected() {
	comm.status = disconnected
}

func (comm *ACommunicator) isConnected() bool {
	return comm.status == connected
}

func (comm *ACommunicator) isConnecting() bool {
	return comm.status == connecting
}

func (comm *ACommunicator) handleConnection() {

	headers := http.Header{"x-public-key": {comm.publicKey}}

	for {
		select {
		case <-comm.doReconnect:
			if comm.isConnecting() {
				continue
			}

			comm.mu.Lock()
			comm.setConnecting()

			log.Println("Connecting to the platform.")

			c, _, err := websocket.DefaultDialer.Dial(comm.platformUri, headers)

			if err != nil {
				log.Println("Connection failed error:", err)

				time.Sleep(time.Duration(comm.reconnectTimeoutSec) * time.Second)

				comm.doReconnect <- struct{}{}
				comm.setDisconected()
				comm.mu.Unlock()
				continue
			}

			c.SetWriteDeadline(time.Now().Add(time.Duration(comm.sendTimeoutSec) * time.Second))
			comm.conn = c

			comm.setConnected()

			comm.mu.Unlock()

			log.Println("Connected to the platform.")

		case <-comm.shutdownChan:
			c := comm.conn

			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

			if err != nil {
				log.Println("WS Close error:", err)
			}

			c.Close()
			comm.setDisconected()
			return

		default:
			continue
		}
	}
}

func (comm *ACommunicator) handleSend() {
	tickerP := time.NewTicker(time.Duration(comm.pingIntervalSec) * time.Second)

	for {
		select {
		case msg := <-comm.sendChan:
			if !comm.isConnected() {
				continue
			}

			c := comm.conn

			err := c.WriteMessage(websocket.TextMessage, []byte(msg))

			if err != nil {
				log.Println("Send WS data error:", err)

				if !comm.isConnecting() {
					comm.doReconnect <- struct{}{}
				}

				continue
			}

		case <-tickerP.C:
			if !comm.isConnected() {
				continue
			}

			c := comm.conn

			err := c.WriteMessage(websocket.PingMessage, []byte(comm.publicKey))

			if err != nil {
				log.Println("WS Ping error:", err)

				if !comm.isConnecting() {
					comm.doReconnect <- struct{}{}
				}

				continue
			}

		case <-comm.shutdownChan:
			return

		default:
			continue
		}

	}
}

func (comm *ACommunicator) handleReceive() {
	for {
		select {
		case <-comm.shutdownChan:
			return

		default:
			if !comm.isConnected() {
				continue
			}
			_, message, err := comm.conn.ReadMessage()
			if err != nil {
				log.Println("WS receiving error", err)
				comm.doReconnect <- struct{}{}
				continue
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
	go comm.handleSend()
	go comm.handleReceive()
}
