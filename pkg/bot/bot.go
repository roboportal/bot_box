package bot

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/pion/mediadevices"
	"github.com/pion/webrtc/v3"
	"github.com/roboportal/bot_box/pkg/botcom"
	"github.com/roboportal/bot_box/pkg/utils"
)

const (
	Idle       = "Idle"
	Connecting = "Connecting"
	Connected  = "Connected"
)

type ABot struct {
	QuitWebRTCChan            chan struct{}
	WebRTCConnectionStateChan chan string
	DescriptionChan           chan webrtc.SessionDescription
	CandidateChan             chan webrtc.ICECandidateInit
	ArenaDescriptionChan      chan webrtc.SessionDescription
	ArenaCandidateChan        chan webrtc.ICECandidateInit
	SendDataChan              chan string
	ControlsReadyChan         chan bool
	ID                        int
	Status                    string
	IsReady                   bool
}

func (b *ABot) SetIdle() {
	b.Status = Idle
	b.IsReady = false
}

func (b *ABot) SetConnecting() {
	b.Status = Connecting
	b.IsReady = false
}

func (b *ABot) SetConnected() {
	b.Status = Connected
}

func (b *ABot) NotifyAreControlsAllowedBySupervisorChange(state bool) {
	if b.Status != Connected {
		return
	}

	status := "DECLINED"
	if state {
		status = "ALLOWED"
	}
	command := fmt.Sprintf("{\"type\": \"CONTROLS_SUPERVISOR_STATUS_CHANGE\", \"payload\": {\"status\": \"%s\"}}", status)
	go func() {
		b.SendDataChan <- command
	}()
}

type RunParams struct {
	StunUrls                          []string
	TokenString                       string
	PublicKey                         string
	Api                               *webrtc.API
	MediaStream                       mediadevices.MediaStream
	WsWriteChan                       chan string
	SerialWriteChan                   chan string
	SerialReadChan                    chan string
	GetAreControlsAllowedBySupervisor func() bool
	GetAreBotsReady                   func() bool
	SetBotReady                       func(int)
}

func (b *ABot) Run(p RunParams) {

	type CreateConnectionPayload struct {
		Token     string `json:"token"`
		PublicKey string `json:"publicKey"`
		ID        int    `json:"id"`
	}

	type CreateConnectionAction struct {
		Name    string                  `json:"name"`
		Payload CreateConnectionPayload `json:"payload"`
	}

	log.Println("Creating connection for bot: ", b.ID)
	message := CreateConnectionAction{
		Name: "CREATE_CONNECTION",
		Payload: CreateConnectionPayload{
			Token:     p.TokenString,
			PublicKey: p.PublicKey,
			ID:        b.ID,
		},
	}

	br, err := json.Marshal(message)

	if err != nil {
		log.Println(err)
		return
	}

	p.WsWriteChan <- string(br)

	botcomParams := botcom.InitParams{
		Id:                                b.ID,
		StunUrls:                          p.StunUrls,
		Api:                               p.Api,
		MediaStream:                       p.MediaStream,
		DescriptionChan:                   b.DescriptionChan,
		CandidateChan:                     b.CandidateChan,
		ArenaDescriptionChan:              b.ArenaDescriptionChan,
		ArenaCandidateChan:                b.ArenaCandidateChan,
		WebRTCConnectionStateChan:         b.WebRTCConnectionStateChan,
		SendDataChan:                      b.SendDataChan,
		QuitWebRTCChan:                    b.QuitWebRTCChan,
		SerialWriteChan:                   p.SerialWriteChan,
		ControlsReadyChan:                 b.ControlsReadyChan,
		GetAreControlsAllowedBySupervisor: p.GetAreControlsAllowedBySupervisor,
		GetAreBotsReady:                   p.GetAreBotsReady,
	}

	go botcom.Init(botcomParams)

	for {
		select {

		case description := <-b.ArenaDescriptionChan:
			type SetDescriptionPayload struct {
				Token       string                    `json:"token"`
				PublicKey   string                    `json:"publicKey"`
				Description webrtc.SessionDescription `json:"description"`
				ID          int                       `json:"id"`
			}

			type SetOfferAction struct {
				Name    string                `json:"name"`
				Payload SetDescriptionPayload `json:"payload"`
			}

			log.Println("Sending answer for bot: ", b.ID)
			message := SetOfferAction{
				Name: "SET_DESCRIPTION",
				Payload: SetDescriptionPayload{
					Token:       p.TokenString,
					PublicKey:   p.PublicKey,
					Description: description,
					ID:          b.ID,
				},
			}

			b, err := json.Marshal(message)

			if err != nil {
				log.Println(err)
				return
			}
			p.WsWriteChan <- string(b)

		case candidate := <-b.ArenaCandidateChan:
			type SetCandidatePayload struct {
				Token     string                  `json:"token"`
				PublicKey string                  `json:"publicKey"`
				Candidate webrtc.ICECandidateInit `json:"candidate"`
				ID        int                     `json:"id"`
			}

			type SetCandidateAction struct {
				Name    string              `json:"name"`
				Payload SetCandidatePayload `json:"payload"`
			}

			log.Println("Sending candidate for bot: ", b.ID)
			message := SetCandidateAction{
				Name: "SET_CANDIDATE",
				Payload: SetCandidatePayload{
					Token:     p.TokenString,
					PublicKey: p.PublicKey,
					Candidate: candidate,
					ID:        b.ID,
				},
			}

			b, err := json.Marshal(message)

			if err != nil {
				log.Println(err)
				return
			}
			p.WsWriteChan <- string(b)

		case state := <-b.WebRTCConnectionStateChan:
			if state == webrtc.ICEConnectionStateConnected.String() {
				b.SetConnected()

				type BotConnectedPayload struct {
					Token     string `json:"token"`
					PublicKey string `json:"publicKey"`
					ID        int    `json:"id"`
				}

				type BotConnectedAction struct {
					Name    string              `json:"name"`
					Payload BotConnectedPayload `json:"payload"`
				}

				log.Println("Bot connected via WebRTC: ", b.ID)
				message := BotConnectedAction{
					Name: "BOT_CONNECTED",
					Payload: BotConnectedPayload{
						Token:     p.TokenString,
						PublicKey: p.PublicKey,
						ID:        b.ID,
					},
				}

				command, err := json.Marshal(message)

				if err != nil {
					log.Println(err)
					return
				}

				p.WsWriteChan <- string(command)

			}
			if state == webrtc.ICEConnectionStateFailed.String() ||
				state == webrtc.ICEConnectionStateDisconnected.String() ||
				state == webrtc.ICEConnectionStateClosed.String() {
				b.SetIdle()

				type UnblockDisconnectedPayload struct {
					Token     string `json:"token"`
					PublicKey string `json:"publicKey"`
					ID        int    `json:"id"`
				}

				type UnblockDisconnectedAction struct {
					Name    string                     `json:"name"`
					Payload UnblockDisconnectedPayload `json:"payload"`
				}

				log.Println("Unblocking disconnected bot: ", b.ID)
				message := UnblockDisconnectedAction{
					Name: "UNBLOCK_DISCONNECTED",
					Payload: UnblockDisconnectedPayload{
						Token:     p.TokenString,
						PublicKey: p.PublicKey,
						ID:        b.ID,
					},
				}

				command, err := json.Marshal(message)

				if err != nil {
					log.Println(err)
					return
				}

				p.WsWriteChan <- string(command)

				go utils.TriggerChannel(b.QuitWebRTCChan)

			}

		case state := <-b.ControlsReadyChan:
			log.Println("Bot ready:", b.ID, state)

			status := "DECLINED"
			if state {
				status = "ALLOWED"
			}
			command := fmt.Sprintf("{\"type\": \"CONTROLS_STATUS_CHANGE\", \"payload\": {\"status\": \"%s\"}}", status)

			go func() {
				b.SendDataChan <- command
			}()

			if state {
				p.SetBotReady(b.ID)
			}
		}
	}
}

func Factory(id int) ABot {
	return ABot{
		QuitWebRTCChan:            make(chan struct{}),
		WebRTCConnectionStateChan: make(chan string),
		DescriptionChan:           make(chan webrtc.SessionDescription),
		CandidateChan:             make(chan webrtc.ICECandidateInit),
		ArenaDescriptionChan:      make(chan webrtc.SessionDescription),
		ArenaCandidateChan:        make(chan webrtc.ICECandidateInit),
		SendDataChan:              make(chan string),
		ControlsReadyChan:         make(chan bool),
		ID:                        id,
		Status:                    Idle,
		IsReady:                   false,
	}
}
