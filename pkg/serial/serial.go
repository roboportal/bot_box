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

func serialFactory(p InitParams) ASerial {
	c := &serial.Config{Name: p.PortName, Baud: p.BaudRate}
	s, err := serial.OpenPort(c)

	if err != nil {
		log.Println("Failed to open serial port: ", err)
		panic(err)
	}

	return ASerial{
		serial:      s,
		sendChan:    p.SendChan,
		receiveChan: p.ReceiveChan,
	}
}

func (s *ASerial) handleSend() {
	for {
		select {
		case msg := <-s.sendChan:
			_, err := s.serial.Write([]byte(msg + "\n"))
			if err != nil {
				log.Println("Serial write error:", err)
			}
		}
	}
}

func (s *ASerial) handleReceive() {
	scanner := bufio.NewScanner(s.serial)

	for scanner.Scan() {
		data := scanner.Text()
		s.receiveChan <- data
	}
}

// Init setups
func Init(p InitParams) {
	s := serialFactory(p)

	go s.handleReceive()
	go s.handleSend()
}
