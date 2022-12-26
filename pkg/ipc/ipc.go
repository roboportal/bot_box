package ipc

import (
	"strconv"

	"github.com/zeromq/goczmq"
)

type InitParams struct {
	BotBoxIPCPort int
	RobotIPCPort  int
	RobotIPCHost  string
	SendChan      chan string
	ReceiveChan   chan string
}

func Init(p InitParams) {

	router := goczmq.NewRouterChanneler("tcp://*:" + strconv.Itoa(p.BotBoxIPCPort))

	defer router.Destroy()

	dealer := goczmq.NewDealerChanneler("tcp://" + p.RobotIPCHost + ":" + strconv.Itoa(p.RobotIPCPort))

	defer dealer.Destroy()

	for {
		select {
		case request := <-router.RecvChan:
			p.ReceiveChan <- string(request[1])

		case msg := <-p.SendChan:
			dealer.SendChan <- [][]byte{[]byte(msg)}
		}
	}
}
