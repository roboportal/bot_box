package botcom

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/pion/mediadevices"
	"github.com/roboportal/bot_box/pkg/utils"

	"github.com/pion/webrtc/v3"

	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
)

type InitParams struct {
	Id                                int
	StunUrls                          []string
	Api                               *webrtc.API
	MediaStream                       mediadevices.MediaStream
	DescriptionChan                   chan webrtc.SessionDescription
	CandidateChan                     chan webrtc.ICECandidateInit
	ArenaDescriptionChan              chan webrtc.SessionDescription
	ArenaCandidateChan                chan webrtc.ICECandidateInit
	WebRTCConnectionStateChan         chan string
	SendDataChan                      chan string
	QuitWebRTCChan                    chan struct{}
	SerialWriteChan                   chan string
	ControlsReadyChan                 chan bool
	GetAreControlsAllowedBySupervisor func() bool
	GetAreBotsReady                   func() bool
}

func haltControls(serialWriteChan chan string, id int) {
	command := fmt.Sprintf("{\"address\":%d,\"controls\":{\"stop\":true}}", id)
	serialWriteChan <- command
}

func enableControls(serialWriteChan chan string, id int) {
	command := fmt.Sprintf("{\"address\":%d,\"controls\":{\"start\":true}}", id)
	serialWriteChan <- command
}

func Init(p InitParams) {

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: p.StunUrls,
			},
		},
	}

	var candidatesMux sync.Mutex
	pendingCandidates := make([]*webrtc.ICECandidate, 0)

	var peerConnection *webrtc.PeerConnection

	closeDataChannelChan := make(chan struct{})

	for {
		select {

		case description := <-p.DescriptionChan:
			var err error

			peerConnection, err = p.Api.NewPeerConnection(config)

			if err != nil {
				log.Println("Create peerConnection error", err)
				defer peerConnection.Close()
				defer haltControls(p.SerialWriteChan, p.Id)
				return
			}
			defer peerConnection.Close()

			p.ControlsReadyChan <- false

			peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
				log.Println("OnICECandidate bot:", p.Id, c)
				if c == nil {
					return
				}

				candidatesMux.Lock()
				defer candidatesMux.Unlock()

				desc := peerConnection.RemoteDescription()
				if desc == nil {
					pendingCandidates = append(pendingCandidates, c)
				} else {
					candidate := c.ToJSON()
					p.ArenaCandidateChan <- candidate
				}
			})

			peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
				connectionStateString := connectionState.String()
				log.Println("ICE Connection State has changed:", p.Id, connectionStateString)
				p.WebRTCConnectionStateChan <- connectionStateString
			})

			peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
				log.Println("New DataChannel:", d.Label(), d.ID())

				// Register channel opening handling
				d.OnOpen(func() {
					log.Println("Data channel open:", d.Label(), d.ID())

					enableControls(p.SerialWriteChan, p.Id)

					for {
						select {
						case msg := <-p.SendDataChan:
							if peerConnection.ICEConnectionState() == webrtc.ICEConnectionStateConnected {
								err := d.SendText(msg)
								if err != nil {
									log.Println("Send data to Client App over data channel error", err)
								}
							} else {
								log.Println("Sending data to Client App over data channel when not connected", d.Label(), d.ID())
							}

						case <-closeDataChannelChan:
							log.Println("Closing data channel for bot:", p.Id)
							defer d.Close()
							defer haltControls(p.SerialWriteChan, p.Id)
							return
						}
					}
				})

				// Register text message handling
				d.OnMessage(func(msg webrtc.DataChannelMessage) {
					message := string(msg.Data)

					type aMessage struct {
						Type string
					}

					var data aMessage
					err := json.Unmarshal([]byte(message), &data)

					if err != nil {
						log.Println("Parse data channel message from Client App error", err)
						defer peerConnection.Close()
						defer haltControls(p.SerialWriteChan, p.Id)
						return
					}

					switch data.Type {
					case "CONTROLS":

						if !p.GetAreControlsAllowedBySupervisor() {
							log.Println("Controls blocked by supervisor")
							break
						}

						if !p.GetAreBotsReady() {
							log.Println("Controls are not allowed yet:", p.Id)
							break
						}

						type aControlsMessage struct {
							Payload string
						}

						var data aControlsMessage
						err := json.Unmarshal([]byte(message), &data)

						if err != nil {
							log.Println("Parse 'CONTROLS' message over data channel from Client App error", err)
							defer peerConnection.Close()
							defer haltControls(p.SerialWriteChan, p.Id)
							return
						}

						command := fmt.Sprintf("{\"address\":%d,\"controls\":%s}", p.Id, data.Payload)

						p.SerialWriteChan <- command

					case "READY":
						enableControls(p.SerialWriteChan, p.Id)
						p.ControlsReadyChan <- true

					case "NOT_READY":
						haltControls(p.SerialWriteChan, p.Id)
						p.ControlsReadyChan <- false
					}

				})
			})

			for _, track := range p.MediaStream.GetTracks() {
				track.OnEnded(func(err error) {
					log.Println("Track ended with error:", track.ID(), err)
					defer track.Close()
				})

				t, err := peerConnection.AddTransceiverFromTrack(track,
					webrtc.RtpTransceiverInit{
						Direction: webrtc.RTPTransceiverDirectionSendonly,
					},
				)
				if err != nil {
					log.Println("AddTransceiverFromTrack to peerConnection error", err)
					defer peerConnection.Close()
					defer haltControls(p.SerialWriteChan, p.Id)
					return
				}

				defer t.Stop()

			}

			err = peerConnection.SetRemoteDescription(description)

			if err != nil {
				log.Println("SetRemoteDescription to peerConnection error", err)
				defer peerConnection.Close()
				defer haltControls(p.SerialWriteChan, p.Id)
				return
			}

			answer, err := peerConnection.CreateAnswer(nil)

			if err != nil {
				log.Println("CreateAnswer for Offer error", err)
				defer peerConnection.Close()
				defer haltControls(p.SerialWriteChan, p.Id)
				return
			}

			err = peerConnection.SetLocalDescription(answer)

			if err != nil {
				log.Println("SetLocalDescription error", err)
				defer peerConnection.Close()
				defer haltControls(p.SerialWriteChan, p.Id)
				return
			}

			p.ArenaDescriptionChan <- *peerConnection.LocalDescription()

			candidatesMux.Lock()

			for _, c := range pendingCandidates {
				candidate := c.ToJSON()
				p.ArenaCandidateChan <- candidate
			}

			candidatesMux.Unlock()

		case candidate := <-p.CandidateChan:
			err := peerConnection.AddICECandidate(candidate)
			if err != nil {
				log.Println("AddICECandidate error", err)
			}

		case <-p.QuitWebRTCChan:
			log.Println("Quitting WebRTC for bot:", p.Id)
			if peerConnection != nil {
				peerConnection.Close()
			}
			go utils.TriggerChannel(closeDataChannelChan)
			go haltControls(p.SerialWriteChan, p.Id)
			go Init(p)
			return
		}

	}

}
