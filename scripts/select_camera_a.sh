#!/bin/sh

raspi-gpio set 4 op
raspi-gpio set 17 op

raspi-gpio set 17 dl #set the gpio17 low
raspi-gpio set  4 dl #set the gpio4 low

i2cset -y 1 0x70 0x00 0x01

