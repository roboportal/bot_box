package serial

import (
	"os"
	"strconv"
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
	debug, err := strconv.ParseBool(os.Getenv("debug"))

	if err != nil {
		panic(err)
	}
	
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
				if debug {
					log.Println("Writing message over serial:", msg)
				}

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

		if debug {
			log.Println("Received message over serial:", data)
		}
	}
}
