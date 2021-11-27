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
}

func (b *ABot) SetConnecting() {
	b.Status = Connecting
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

func (b *ABot) Run(
	stunUrls []string,
	tokenString string,
	publicKey string,
	api *webrtc.API,
	mediaStream mediadevices.MediaStream,
	wsWrite chan string,
	serialWrite chan string,
	serialRead chan string,
	areControlsAllowedBySupervisor *bool,
	areBotsReady *bool,
	setBotReady func(int),
) {

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
			Token:     tokenString,
			PublicKey: publicKey,
			ID:        b.ID,
		},
	}

	br, err := json.Marshal(message)

	if err != nil {
		log.Println(err)
		return
	}

	wsWrite <- string(br)

	go botcom.Init(
		b.ID,
		stunUrls,
		api,
		mediaStream,
		b.DescriptionChan,
		b.CandidateChan,
		b.ArenaDescriptionChan,
		b.ArenaCandidateChan,
		b.WebRTCConnectionStateChan,
		b.SendDataChan,
		b.QuitWebRTCChan,
		serialWrite,
		b.ControlsReadyChan,
		areControlsAllowedBySupervisor,
		areBotsReady,
	)

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
					Token:       tokenString,
					PublicKey:   publicKey,
					Description: description,
					ID:          b.ID,
				},
			}

			b, err := json.Marshal(message)

			if err != nil {
				log.Println(err)
				return
			}
			wsWrite <- string(b)

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
					Token:     tokenString,
					PublicKey: publicKey,
					Candidate: candidate,
					ID:        b.ID,
				},
			}

			b, err := json.Marshal(message)

			if err != nil {
				log.Println(err)
				return
			}
			wsWrite <- string(b)

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

				log.Println("Creating connection for bot: ", b.ID)
				message := BotConnectedAction{
					Name: "BOT_CONNECTED",
					Payload: BotConnectedPayload{
						Token:     tokenString,
						PublicKey: publicKey,
						ID:        b.ID,
					},
				}

				command, err := json.Marshal(message)

				if err != nil {
					log.Println(err)
					return
				}

				wsWrite <- string(command)

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

				log.Println("Creating connection for bot: ", b.ID)
				message := UnblockDisconnectedAction{
					Name: "UNBLOCK_DISCONNECTED",
					Payload: UnblockDisconnectedPayload{
						Token:     tokenString,
						PublicKey: publicKey,
						ID:        b.ID,
					},
				}

				command, err := json.Marshal(message)

				if err != nil {
					log.Println(err)
					return
				}

				wsWrite <- string(command)

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
				setBotReady(b.ID)
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
