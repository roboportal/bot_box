package serial

import (
	"log"
	"strings"
	"time"

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
	for {
		done := make(chan struct{})

		c := &serial.Config{Name: portName, Baud: baudrate}
		s, err := serial.OpenPort(c)

		if err != nil {
			log.Println(err)
			time.Sleep(time.Duration(reconnectTimeoutSec) * time.Second)
			continue
		}

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

		func() {
			defer close(done)
			defer s.Close()
			buf := make([]byte, 200)
			for {
				temp := make([]byte, 200)
				_, err := s.Read(temp)
				if err != nil {
					log.Println(err)
					return
				}
				buf = append(buf, temp...)

				str := string(buf)
				index := strings.Index(str, "\n")
				if index < 0 {
					continue
				}
				read <- str[0 : index-1]
			}
		}()
	}
}
