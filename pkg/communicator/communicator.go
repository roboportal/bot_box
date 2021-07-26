package communicator

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Init setups
func Init(
	addr string,
	read chan string,
	write chan string,
	disconnect chan struct{},
	reconnectTimeoutSec int,
	pingPeriodSec int,
	writeDeadlineSec int,
	tokenString string,
	publicKey string,
) {
	for {

		done := make(chan struct{})

		headers := http.Header{"x-public-key": {publicKey}}

		c, _, err := websocket.DefaultDialer.Dial(addr, headers)

		if err != nil {
			log.Println(err)
			time.Sleep(time.Duration(reconnectTimeoutSec) * time.Second)
			continue
		}

		//write
		go func() {
			defer c.Close()

			tickerP := time.NewTicker(time.Duration(pingPeriodSec) * time.Second)
			defer tickerP.Stop()

			for {
				select {
				case <-done:
					return
				case msg := <-write:
					c.SetWriteDeadline(time.Now().Add(time.Duration(writeDeadlineSec) * time.Second))
					err := c.WriteMessage(websocket.TextMessage, []byte(msg))
					if err != nil {
						log.Println(err)
						return
					}
				case <-tickerP.C:
					c.SetWriteDeadline(time.Now().Add(time.Duration(writeDeadlineSec) * time.Second))
					err := c.WriteMessage(websocket.PingMessage, []byte(publicKey))
					if err != nil {
						return
					}
				case <-disconnect:
					err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					if err != nil {
						log.Println(err)
						return
					}
					select {
					case <-done:
					case <-time.After(time.Second):
					}
					return
				}
			}
		}()

		//read
		func() {
			defer close(done)
			defer c.Close()

			for {
				_, message, err := c.ReadMessage()
				if err != nil {
					log.Println(err)
					return
				}
				if message != nil {
					read <- string(message)
				}
			}
		}()
	}
}
