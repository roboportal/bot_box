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
	WSReadChan                     chan string
	WSWriteChan                    chan string
	WSConStatChan                  chan string
	SerialWriteChan                chan string
	SerialReadChan                 chan string
	DisconnectChan                 chan struct{}
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

type InitParams struct {
	StunUrls    []string
	TokenString string
	PublicKey   string
	NBots       int

	VideoCodecBitRate int
	FrameFormat       string
	VideoWidth        int
	VideoFrameRate    int
}

func Factory(p InitParams) AnArena {
	return AnArena{
		WSReadChan:      make(chan string, 1000),
		WSWriteChan:     make(chan string, 1000),
		WSConStatChan:   make(chan string, 1),
		SerialWriteChan: make(chan string, 1000),
		SerialReadChan:  make(chan string, 1000),
		DisconnectChan:  make(chan struct{}, 1),

		TokenString: p.TokenString,
		PublicKey:   p.PublicKey,
		stunURLs:    p.StunUrls,

		videoCodecBitRate: p.VideoCodecBitRate,
		frameFormat:       p.FrameFormat,
		videoWidth:        p.VideoWidth,
		videoFrameRate:    p.VideoFrameRate,

		botsCount:                      p.NBots,
		Bots:                           make([]*bot.ABot, p.NBots),
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

func (a *AnArena) SetBotNotReady(id int) {
	a.Bots[id].IsReady = false
	a.areBotsReady = false
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
		b.SendDataChan <- "{\"type\": \"DISCONNECTED_BY_ADMIN\"}"
		utils.TriggerChannel(b.QuitWebRTCChan)
	}
}

func (a *AnArena) setAreControlsAllowedBySupervisor(state bool) {
	a.areControlsAllowedBySupervisor = state
}

func (a *AnArena) getAreControlsAllowedBySupervisor() bool {
	return a.areControlsAllowedBySupervisor
}

func (a *AnArena) getAreBotsReady() bool {
	return a.areBotsReady
}

func (a *AnArena) Run() {
	log.Println("Arena: waiting for WS connection")

	for stat := range a.WSConStatChan {
		if stat == "connected" {
			break
		}
	}

	log.Println("Arena: WS connected")

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
		log.Println("GetUserMedia error", err)
		panic(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine), webrtc.WithSettingEngine(settingEngine))

	for index := 0; index < a.botsCount; index++ {
		b := bot.Factory(index)
		a.Bots[index] = &b

		botParams := bot.RunParams{
			StunUrls:                          a.stunURLs,
			TokenString:                       a.TokenString,
			PublicKey:                         a.PublicKey,
			Api:                               api,
			MediaStream:                       mediaStream,
			WsWriteChan:                       a.WSWriteChan,
			WSConStatChan:                     a.WSConStatChan,
			SerialWriteChan:                   a.SerialWriteChan,
			SerialReadChan:                    a.SerialReadChan,
			GetAreControlsAllowedBySupervisor: a.getAreControlsAllowedBySupervisor,
			GetAreBotsReady:                   a.getAreBotsReady,
			SetBotReady:                       a.SetBotReady,
			SetBotNotReady:                    a.SetBotNotReady,
		}
		go b.Run(botParams)
	}

	for {
		select {
		case msg := <-a.WSReadChan:
			type aData struct {
				Action string
				Data   string
				ID     int
			}

			var data aData

			err := json.Unmarshal([]byte(msg), &data)

			if err != nil {
				log.Println("Parse message from RoboPortal error", err)
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
					log.Println("Parse 'TOGGLE_CONTROLS' message from RoboPortal error", err)
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
					log.Println("Parse 'SET_DESCRIPTION' message from RoboPortal error", err)
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
					log.Println("Parse 'SET_CANDIDATE' message from RoboPortal error", err)

					continue
				}

				b.CandidateChan <- d

				continue
			}

			if data.Action == "IS_BOT_READY_FOR_CONNECTION" {
				type BotIsReadyForConnecionPayload struct {
					Token     string `json:"token"`
					PublicKey string `json:"publicKey"`
					ID        int    `json:"id"`
					IsReady   bool   `json:"isReady"`
				}

				type BotIsReadyForConnecionAction struct {
					Name    string                        `json:"name"`
					Payload BotIsReadyForConnecionPayload `json:"payload"`
				}

				message := BotIsReadyForConnecionAction{
					Name: "BOT_IS_READY_FOR_CONNECTION",
					Payload: BotIsReadyForConnecionPayload{
						Token:     a.TokenString,
						PublicKey: a.PublicKey,
						ID:        data.ID,
						IsReady:   b.Status == bot.Idle,
					},
				}

				br, err := json.Marshal(message)

				if err != nil {
					log.Println("Serialize 'BOT_IS_READY_FOR_CONNECTION' message to RoboPortal error", err)
					return
				}

				a.WSWriteChan <- string(br)

				continue
			}

			if data.Action == "DISCONNECT_BOT" {
				log.Println("Disconnect bot: ", b.ID)
				utils.TriggerChannel(b.QuitWebRTCChan)

				continue
			}

		case serialMsg := <-a.SerialReadChan:
			r := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "\x00", "")
			sanitizedMsg := r.Replace(serialMsg)

			type TelemetryMessage struct {
				ID int `json:"id"`
			}

			var t TelemetryMessage

			err := json.Unmarshal([]byte(sanitizedMsg), &t)

			if err != nil {
				log.Println("Parse telemetry message from robot error", err)
				continue
			}

			if a.Bots[t.ID].Status == bot.Connected {
				telemetry := fmt.Sprintf("{\"type\": \"TELEMETRY\", \"payload\": %s}", sanitizedMsg)
				a.Bots[t.ID].SendDataChan <- telemetry
			}
		}
	}
}
