package serial

import (
	"bufio"
	"log"

	"github.com/tarm/serial"
)

// Init setups
func Init(
	portName string,
	baudrate int,
	write chan string,
	read chan string,
	reconnectTimeoutSec int,
) {
	done := make(chan struct{})

	c := &serial.Config{Name: portName, Baud: baudrate}
	s, err := serial.OpenPort(c)

	go func() {
		defer s.Close()
		for {
			select {
			case <-done:
				return
			case msg := <-write:
				_, err = s.Write([]byte(msg + "\n"))
				if err != nil {
					log.Println(err)
					return
				}
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(s)

		for scanner.Scan() {
			data := scanner.Text()

			go func(data string) {
				read <- data
			}(data)
		}
	}()
}
