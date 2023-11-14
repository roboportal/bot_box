package communicator

import (
	"context"
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

// Init setups ws connection
func Init(p InitParams) {
	for {
		var wg sync.WaitGroup
		ctx := context.Background()
		status := disconnected
		p.ConStatChan <- status

		headers := http.Header{"x-public-key": {p.PublicKey}}

		tickerPing := time.NewTicker(time.Duration(p.PingIntervalSec) * time.Second)
		tickerPong := time.NewTicker(time.Duration(p.PingIntervalSec*3) * time.Second)

		status = connecting
		p.ConStatChan <- status

		log.Println("Connecting to the platform.")

		dialer := websocket.Dialer{}
		conn, _, err := dialer.DialContext(ctx, p.PlatformUri, headers)

		done := make(chan struct{})

		if err != nil {
			log.Println("Connection failed error:", err)

			log.Println("Sleeping before reconnecting.")

			time.Sleep(time.Duration(p.ReconnectTimeoutSec) * time.Second)

			continue
		}

		conn.SetPongHandler(func(d string) error {
			tickerPong.Reset(time.Duration(p.PingIntervalSec*3) * time.Second)

			return nil
		})

		status = connected
		p.ConStatChan <- status

		log.Println("Connected to the platform.")

		go (func() {
			wg.Add(1)

			defer wg.Done()

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
						utils.NicelyClose(done)
						return
					}

					if message != nil {
						p.ReceiveChan <- string(message)
					}
				}
			}
		})()

		tickerPing.Reset(time.Duration(p.PingIntervalSec) * time.Second)
		tickerPong.Reset(time.Duration(p.PingIntervalSec*3) * time.Second)

		for loop := true; loop; {
			select {
			case <-done:
				log.Println("Exiting communicator loop")
				loop = false
				break

			case <-tickerPong.C:
				log.Println("Pong timeout")
				utils.NicelyClose(done)
				loop = false
				break

			case <-tickerPing.C:
				if status != connected {
					continue
				}

				err := conn.WriteControl(websocket.PingMessage, []byte(p.PublicKey), time.Now().Add(time.Duration(p.SendTimeoutSec)*time.Second))

				if err != nil {
					log.Println("WriteControl error:", err)
					utils.NicelyClose(done)
					loop = false
					break
				}

			case msg := <-p.SendChan:
				if status != connected {
					log.Println("Trying to send data thru closed socket")
					loop = false
					break
				}

				conn.SetWriteDeadline(time.Now().Add(time.Duration(p.SendTimeoutSec) * time.Second))

				err := conn.WriteMessage(websocket.TextMessage, []byte(msg))

				if err != nil {
					log.Println("Send WS data error:", err)
					utils.NicelyClose(done)
					loop = false
					break
				}
			}
		}

		status = disconnected
		p.ConStatChan <- status

		tickerPing.Stop()
		tickerPong.Stop()

		if conn != nil {
			conn.Close()
		}

		log.Println("Awaiting for read gorutine to finish.")

		wg.Wait()

		log.Println("Sleeping before reconnecting.")

		time.Sleep(time.Duration(p.ReconnectTimeoutSec) * time.Second)
	}
}
