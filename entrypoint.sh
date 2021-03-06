#!/bin/bash

health_check_url=https://api.roboportal.io:8000/.well-known/apollo/server-health

function check_connection() {
  curl -LI $health_check_url -o /dev/null -w '%{http_code}\n' -s
}

if [ $(check_connection) == '200' ]
  then 
    echo Connected to the internet. Starting.
  else
    echo Dialing LTE
    
    sudo wvdialconf
    sudo wvdial &
    sleep 10
    if [ $(check_connection) != '200' ]
      then
        echo Failed to establish connection.
        exit 1
    fi
fi

/home/pi/bot_box/bot_box