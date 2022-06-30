package serial

import (
	"bufio"
	"log"

	"github.com/tarm/serial"
)

type InitParams struct {
	PortName    string
	BaudRate    int
	SendChan    chan string
	ReceiveChan chan string
}

type ASerial struct {
	serial      *serial.Port
	sendChan    chan string
	receiveChan chan string
}

// Init setups
func Init(p InitParams) {
	c := &serial.Config{Name: p.PortName, Baud: p.BaudRate}
	s, err := serial.OpenPort(c)

	if err != nil {
		log.Println("Failed to open serial port: ", err)
		panic(err)
	}

	go func() {
		for {
			select {
			case msg := <-p.SendChan:
				_, err := s.Write([]byte(msg + "\n"))
				if err != nil {
					log.Println("Serial write error:", err)
				}
			}
		}
	}()

	scanner := bufio.NewScanner(s)

	for scanner.Scan() {
		data := scanner.Text()
		p.ReceiveChan <- data
	}
}
