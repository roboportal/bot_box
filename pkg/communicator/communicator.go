package communicator

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/roboportal/bot_box/pkg/utils"
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
	conStatChan         chan string
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
		conStatChan:         p.ConStatChan,
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

	headers := http.Header{"x-public-key": {comm.publicKey}}
	tickerP := time.NewTicker(time.Duration(comm.pingIntervalSec) * time.Second)

	for {
		select {

		case <-comm.doReconnect:
			if comm.isConnecting() {
				continue
			}

			comm.mu.Lock()
			comm.setConnecting()
			tickerP.Stop()

			log.Println("Connecting to the platform.")

			c, _, err := websocket.DefaultDialer.Dial(comm.platformUri, headers)

			if err != nil {
				log.Println("Connection failed error:", err)

				time.Sleep(time.Duration(comm.reconnectTimeoutSec) * time.Second)

				comm.setDisconnected()

				comm.mu.Unlock()

				go utils.TriggerChannel(comm.doReconnect)

				continue
			}

			comm.conn = c

			comm.setConnected()
			tickerP.Reset(time.Duration(comm.pingIntervalSec) * time.Second)
			comm.mu.Unlock()

			log.Println("Connected to the platform.")

		case <-tickerP.C:
			if !comm.isConnected() {
				continue
			}

			c := comm.conn

			err := c.WriteControl(websocket.PingMessage, []byte(comm.publicKey), time.Now().Add(time.Duration(comm.sendTimeoutSec)*time.Second))

			if err != nil {
				log.Println("WS Ping error:", err)

				if !comm.isConnecting() {
					utils.TriggerChannel(comm.doReconnect)
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
			err := c.WriteMessage(websocket.TextMessage, []byte(msg))

			if err != nil {
				log.Println("Send WS data error:", err)
				continue
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
				go utils.TriggerChannel(comm.doReconnect)
				time.Sleep(time.Duration(comm.reconnectTimeoutSec) * time.Second)
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
	utils.TriggerChannel(comm.doReconnect)

	go comm.handleSend()
	go comm.handleReceive()
}
