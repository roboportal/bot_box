package cameraselector

import (
	"os"
	"log"
	"strconv"
	"github.com/stianeikeland/go-rpio/v4"
	"github.com/d2r2/go-i2c"
)



func SelectCameraA() {
	cameraMultiplexorI2cBus, err := strconv.ParseInt(os.Getenv("camera_multiplexor_i2c_bus"), 10, 32)

	if err != nil {
		panic(err)
	}

	err = rpio.Open()
	
	if err != nil {
		log.Println("SelectCameraA rpio open error:", err)
		return 
	}

	defer rpio.Close()

	i2cConnection, err := i2c.NewI2C(0x70, int(cameraMultiplexorI2cBus))

	if err != nil {
		log.Println("SelectCameraA new i2c error:", err)
		return 
	}

	defer i2cConnection.Close()

	selPin := rpio.Pin(4)
	oePin := rpio.Pin(17)

	selPin.Low() 
	oePin.Low() 

	_, err = i2cConnection.WriteBytes([]byte{0x01})

	if err != nil {
		log.Println("SelectCameraA i2c write error:", err)
		return 
	}
}

func SelectCameraB() {
	cameraMultiplexorI2cBus, err := strconv.ParseInt(os.Getenv("camera_multiplexor_i2c_bus"), 10, 32)

	if err != nil {
		panic(err)
	}

	err = rpio.Open()
	
	if err != nil {
		log.Println("SelectCameraB rpio open error:", err)
		return 
	}

	defer rpio.Close()

	i2cConnection, err := i2c.NewI2C(0x70, int(cameraMultiplexorI2cBus))

	if err != nil {
		log.Println("SelectCameraB new i2c error:", err)
		return 
	}

	defer i2cConnection.Close()

	selPin := rpio.Pin(4)
	oePin := rpio.Pin(17)

	selPin.Low() 
	oePin.Low() 

	_, err = i2cConnection.WriteBytes([]byte{0x02})

	selPin.High() 
	oePin.Low() 
	
	if err != nil {
		log.Println("SelectCameraB i2c write error:", err)
		return 
	}
}