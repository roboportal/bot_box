package arena

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/pion/mediadevices"

	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	"github.com/roboportal/bot_box/pkg/bot"
	"github.com/roboportal/bot_box/pkg/utils"
)

type AnArena struct {
	WSRead                         chan string
	WSWrite                        chan string
	SerialWrite                    chan string
	SerialRead                     chan string
	Disconnect                     chan struct{}
	TokenString                    string
	PublicKey                      string
	stunURLs                       []string
	botsCount                      int
	Bots                           []*bot.ABot
	areControlsAllowedBySupervisor bool
	videoCodecBitRate              int
	frameFormat                    string
	videoWidth                     int
	videoFrameRate                 int
	areBotsReady                   bool
}

func Factory(
	stunUrls []string,
	tokenString string,
	publicKey string,
	nBots int,

	videoCodecBitRate int,
	frameFormat string,
	videoWidth int,
	videoFrameRate int,

) AnArena {
	return AnArena{
		WSRead:      make(chan string),
		WSWrite:     make(chan string),
		SerialWrite: make(chan string),
		SerialRead:  make(chan string),
		Disconnect:  make(chan struct{}),

		TokenString: tokenString,
		PublicKey:   publicKey,
		stunURLs:    stunUrls,

		videoCodecBitRate: videoCodecBitRate,
		frameFormat:       frameFormat,
		videoWidth:        videoWidth,
		videoFrameRate:    videoFrameRate,

		botsCount:                      nBots,
		Bots:                           make([]*bot.ABot, nBots),
		areControlsAllowedBySupervisor: true,
		areBotsReady:                   false,
	}
}

func (a *AnArena) SetBot(id int, b *bot.ABot) {
	a.Bots[id] = b
}

func (a *AnArena) SetBotReady(id int) {
	a.Bots[id].IsReady = true
	if a.AreBotsReady() {
		a.areBotsReady = true
	}
}

func (a *AnArena) AreBotsReady() bool {
	for _, b := range a.Bots {
		if !b.IsReady {
			return false
		}
	}
	return true
}

func (a *AnArena) disconnectAllBots() {
	for _, b := range a.Bots {
		go func(b *bot.ABot) {
			b.SendDataChan <- "{\"type\": \"DISCONNECTED_BY_ADMIN\"}"
			b.QuitWebRTCChan <- struct{}{}
		}(b)
	}
}

func (a *AnArena) setAreControlsAllowedBySupervisor(state bool) {
	a.areControlsAllowedBySupervisor = state
}

func (a *AnArena) Run() {

	codecSelector := getCodecSelector(a.videoCodecBitRate)

	mediaEngine := webrtc.MediaEngine{}
	codecSelector.Populate(&mediaEngine)

	settingEngine := webrtc.SettingEngine{}

	mediaStream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.FrameFormat = prop.FrameFormat(a.frameFormat)
			c.Width = prop.Int(a.videoWidth)
			c.FrameRate = prop.Float(a.videoFrameRate)

		},
		Codec: codecSelector,
	})

	if err != nil {
		panic(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine), webrtc.WithSettingEngine(settingEngine))

	for index := 0; index < a.botsCount; index++ {
		b := bot.Factory(index)
		a.Bots[index] = &b

		go b.Run(
			a.stunURLs,
			a.TokenString,
			a.PublicKey,
			api,
			mediaStream,
			a.WSWrite,
			a.SerialWrite,
			a.SerialRead,
			&a.areControlsAllowedBySupervisor,
			&a.areBotsReady,
			a.SetBotReady,
		)
	}

	for {
		select {
		case msg := <-a.WSRead:
			type aData struct {
				Action string
				Data   string
				ID     int
			}
			var data aData
			err := json.Unmarshal([]byte(msg), &data)
			if err != nil {
				log.Println(err)
				continue
			}
			if data.Action == "DISCONNECT_ALL" {
				a.disconnectAllBots()
			}

			if data.Action == "TOGGLE_CONTROLS" {
				type aPayload struct {
					AreControlsAllowed bool
				}
				var payload aPayload
				err := json.Unmarshal([]byte(data.Data), &payload)
				if err != nil {
					log.Println(err)
					continue
				}
				a.setAreControlsAllowedBySupervisor(payload.AreControlsAllowed)

				for _, b := range a.Bots {
					b.NotifyAreControlsAllowedBySupervisorChange(payload.AreControlsAllowed)
				}
			}

			if data.ID >= a.botsCount {
				continue
			}

			b := a.Bots[data.ID]

			if data.Action == "SET_DESCRIPTION" {
				if b.Status != bot.Idle {
					log.Println("Bot is not connected when set description:", b.ID)
					continue
				}
				b.SetConnecting()
				log.Println("Set description for bot: ", b.ID)
				var d webrtc.SessionDescription
				err := json.Unmarshal([]byte(data.Data), &d)
				if err != nil {
					log.Println(err)
					continue
				}
				b.DescriptionChan <- d
				continue
			}

			if data.Action == "SET_CANDIDATE" {
				if b.Status != bot.Connecting {
					log.Println("Bot is not connecting when set candidate:", b.ID)
					continue
				}

				log.Println("Set candidate for bot:", b.ID, b.Status)
				var d webrtc.ICECandidateInit
				err := json.Unmarshal([]byte(data.Data), &d)
				if err != nil {
					log.Println(err)
					continue
				}
				b.CandidateChan <- d
				continue
			}

			if data.Action == "DISCONNECT_BOT" {
				log.Println("Disconnect bot: ", b.ID)
				go utils.TriggerChannel(b.QuitWebRTCChan)
				continue
			}

		case serialMsg := <-a.SerialRead:
			r := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "\x00", "")
			sanitizedMsg := r.Replace(serialMsg)

			type TelemetryMessage struct {
				ID int `json:"id"`
			}

			var t TelemetryMessage

			err := json.Unmarshal([]byte(sanitizedMsg), &t)

			if err != nil {
				log.Println(err)
				continue
			}

			if a.Bots[t.ID].Status == bot.Connected {
				telemetry := fmt.Sprintf("{\"type\": \"TELEMETRY\", \"payload\": %s}", sanitizedMsg)
				go func() {
					a.Bots[t.ID].SendDataChan <- telemetry
				}()
			}
		}
	}
}
