package communicator

import (
	"log"
	"net/http"
	"context"
	"time"
	"github.com/gorilla/websocket"
)

const (
	disconnected = "disconnected"
	connected    = "connected"
	connecting   = "connecting"
)

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

func nicelyClose(ch chan bool) {
	select {
	case <-ch:
		return
	default:
	}
	close(ch)
}

// Init setups ws connection
func Init(p InitParams) {
	ctx := context.Background()
	status := disconnected
	p.ConStatChan <- status

	headers := http.Header{"x-public-key": {p.PublicKey}}

	tickerPing := time.NewTicker(time.Duration(p.PingIntervalSec) * time.Second)
	tickerPong := time.NewTicker(time.Duration(p.PingIntervalSec * 3) * time.Second)

	status = connecting
	p.ConStatChan <- status

	log.Println("Connecting to the platform.")

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, p.PlatformUri, headers)

	done := make(chan bool)

	defer (func() {
		status := disconnected
		p.ConStatChan <- status

		tickerPing.Stop()
		tickerPong.Stop()

		if conn != nil {
			conn.Close()
		}

		time.Sleep(time.Duration(p.ReconnectTimeoutSec) * time.Second)

		go Init(p)
	})()

	if err != nil {
		log.Println("Connection failed error:", err)
		return
	}

	conn.SetPongHandler(func(d string) error {
		tickerPong.Reset(time.Duration(p.PingIntervalSec * 3) * time.Second)

		return nil
	})


	status = connected
	p.ConStatChan <- status

	log.Println("Connected to the platform.")
	
	go (func() {
		for {
			select {
			case <-done:
				return

			default:
				if status != connected {
					continue
				}

				_, message, err := conn.ReadMessage()

				if err != nil {
					log.Println("WS receiving error", err)
					nicelyClose(done)
					return
				}

				if message != nil {
					log.Println(string(message))
					p.ReceiveChan <- string(message)
				}
			}
		}
	})()

	tickerPing.Reset(time.Duration(p.PingIntervalSec) * time.Second)
	tickerPong.Reset(time.Duration(p.PingIntervalSec * 3) * time.Second)

	for {
		select {
		case <-done:
			return
		case <-tickerPong.C:
			log.Println("tickerPong.C")
			nicelyClose(done)
			return

		case <-tickerPing.C:
			if status != connected {
				continue
			}

			err := conn.WriteControl(websocket.PingMessage, []byte(p.PublicKey), time.Now().Add(time.Duration(p.SendTimeoutSec)*time.Second))

			if err != nil {
				nicelyClose(done)
				return
			}

		case msg := <-p.SendChan:
			if status != connected {
				panic("Trying to send data thru closed socket")
			}

			conn.SetWriteDeadline(time.Now().Add(time.Duration(p.SendTimeoutSec) * time.Second))

			err := conn.WriteMessage(websocket.TextMessage, []byte(msg))

			if err != nil {
				log.Println("Send WS data error:", err)
				nicelyClose(done)
				return
			}
		}
	}
}
