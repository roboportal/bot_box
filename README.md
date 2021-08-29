# Bot Box

Bot Box is a client application for https://roboportal.io.
It enables peer-to-peer controls and video streaming between robots and web-based control panels. In other words, if you are building the robot and planning to add remote-control functionality and real-time video streaming, it is worth considering using this service.

# Features

- real time video streaming
- keyboard and gamepad controls streaming with data channel

# How it works

![System diagram](./doc/system_diagram.png)

# Installation guide

1. Prepare SD card with Raspberry Pi OS image. [How To guide for Raspberry Pi Imager.](https://www.youtube.com/watch?v=ntaXWS8Lk34) We don't need Desktop version for this application.
2. Enable camera, SSH (optional, but handy) and set up the WiFi with `raspi-config`. It also make sense to change the default password and set up the auto login. [ Documentation for raspi-config.](https://www.raspberrypi.org/documentation/configuration/raspi-config.md)
3. Install git and wget (staring from this step you'll need an internet connection)
  ```
  sudo apt update
  sudo apt install git wget
  ```
4. Install golang
  ```
  wget https://dl.google.com/go/go1.14.4.linux-armv6l.tar.gz
  sudo tar -C /usr/local -xzf go1.14.4.linux-armv6l.tar.gz
  rm go1.14.4.linux-armv6l.tar.gz
  ```
  
5. And configure it:
    - Open .profile `nano ~/.profile`
    - And insert this:
    ```
    PATH=$PATH:/usr/local/go/bin
    GOPATH=$HOME/go
    ```
    - save your changes and exit nano: `Ctrl + O` and `Ctrl + X`
    - apply your changes: `source ~/.profile`
    - check the installation: `go version`
   
6. Clone this repository to your Raspberry:
   `git clone git@github.com:roboportal/bot_box.git`

7. Navigate to the repo: `cd ./bot_box`
8. And compile the BotBox: `go build`
9. Create `.env` file for the configuration [following the instructions](#botbox-configuration).
10. Run the bot by executing: `./bot_box`

# BotBox configuration

All the configuration of Box Bot is done by setting up `.env` file. You can use `.env_example` as a staring point for your config.
The list of params:
- `srv_url` - the WSS endpoint of roboportal.io
- `public_key` and `secret_key` - the key pair obtained after the bot creation
- `stun_urls` - comma-separated list of STUN servers URLs 
- `mmal_bit_rate` - bit rate for MMAL codec
- `frame_format` - camera image format
- `video_width` - camera image width
- `video_frame_rate` - camera frame rate

- `port_name` - name of the serial port to communicate with robot hardware
- `baud_rate` - serial port baud rate

- `n_bots` - number of bots controlled by one Bot Box.

# Supervisor setup

Let's setup supervisor. It will start Bot Box process after Raspberry Pi boot and handle restarts after possible application crashes.

1. Install the supervisor: `sudo apt-get install supervisor`
2. The next step is to setup `supervisorctl` for `pi` user: `sudo nano /etc/supervisor/supervisord.conf` 
3. Under the section `[unix_http_server]` modify and create this:
  ```
  chmod=0770
  chown=root:pi
  ```
4. Create the config file for Bot Box: `sudo nano /etc/supervisor/conf.d/bot-box.conf`
5. Add there the following:
  ```
  [program:bot-box]
  command=/home/pi/bot_box/bot_box
  directory=/home/pi/bot_box
  autostart=true
  autorestart=true
  user=pi
  ```
6. To start the bot box run `supervisorctl start bot-box`. There are some handy commands: `supervisorctl stop bot-box`, `supervisorctl restart bot-box`, `supervisorctl tail bot-box stdout`.

# Contacts:

[Join our Discord channel](https://discord.gg/WeAahmwMMv) or reach out over email: info@roboportal.io
