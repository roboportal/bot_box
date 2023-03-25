package consoleoutput

import (
	"log"
)

type InitParams struct {
	SendChan chan string
}

// Init setups
func Init(p InitParams) {
	go func() {
		for {
			msg := <-p.SendChan
			log.Println("Console output:", msg)
		}
	}()
}
