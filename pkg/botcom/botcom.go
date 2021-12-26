package botcom

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/pion/mediadevices"

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

	for {
		select {

		case description := <-p.DescriptionChan:
			var err error

			peerConnection, err = p.Api.NewPeerConnection(config)

			if err != nil {
				log.Println(err)
				peerConnection.Close()
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

			dataChannel, err := peerConnection.CreateDataChannel("controls", nil)
			if err != nil {
				log.Println(err)
				peerConnection.Close()
				return
			}

			defer dataChannel.Close()
			// Register channel opening handling
			dataChannel.OnOpen(func() {
				log.Println("Data channel open:", dataChannel.Label(), dataChannel.ID())
			})

			// Register text message handling
			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {

			})

			peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
				log.Println("New DataChannel:", d.Label(), d.ID())

				// Register channel opening handling
				d.OnOpen(func() {
					log.Println("Data channel open:", d.Label(), d.ID())

					for {
						select {
						case msg := <-p.SendDataChan:
							err := d.SendText(msg)
							if err != nil {
								log.Println(err)
							}

						case <-p.QuitWebRTCChan:
							d.Close()
							return
						}

					}
				})

				// Register text message handling
				d.OnMessage(func(msg webrtc.DataChannelMessage) {
					// log.Println("Message from DataChannel:", id, d.Label(), string(msg.Data))

					message := string(msg.Data)

					type aMessage struct {
						Type string
					}

					var data aMessage
					err := json.Unmarshal([]byte(message), &data)

					if err != nil {
						log.Println(err)
						peerConnection.Close()
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
							log.Println(err)
							peerConnection.Close()
							return
						}

						command := fmt.Sprintf("{\"address\":%d,\"controls\":%s}", p.Id, data.Payload)

						p.SerialWriteChan <- command

					case "READY":
						p.ControlsReadyChan <- true
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
					log.Println(err)
					peerConnection.Close()
					return
				}

				defer t.Stop()

			}

			err = peerConnection.SetRemoteDescription(description)

			if err != nil {
				log.Println(err)
				peerConnection.Close()
				return
			}

			answer, err := peerConnection.CreateAnswer(nil)

			if err != nil {
				log.Println(err)
				peerConnection.Close()
				return
			}

			err = peerConnection.SetLocalDescription(answer)

			if err != nil {
				log.Println(err)
				peerConnection.Close()
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
				log.Println(err)
			}

		case <-p.QuitWebRTCChan:
			if peerConnection != nil {
				peerConnection.Close()
			}
			go Init(p)
			return
		}

	}

}
