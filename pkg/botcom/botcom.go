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

// Init running the WebRTC runtime
func Init(
	id int,
	stunUrls []string,
	api *webrtc.API,
	mediaStream mediadevices.MediaStream,
	descriptionChan chan webrtc.SessionDescription,
	candidateChan chan webrtc.ICECandidateInit,
	arenaDescriptionChan chan webrtc.SessionDescription,
	arenaCandidateChan chan webrtc.ICECandidateInit,
	webRTCConnectionStateChan chan string,
	sendDataChan chan string,
	quitWebRTCChan chan struct{},
	serialWrite chan string,
	controlsReady chan bool,
	areControlsAllowedBySupervisor *bool,
	isReady *bool,
) {

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: stunUrls,
			},
		},
	}

	var candidatesMux sync.Mutex
	pendingCandidates := make([]*webrtc.ICECandidate, 0)

	var peerConnection *webrtc.PeerConnection

	for {
		select {

		case description := <-descriptionChan:
			var err error

			peerConnection, err = api.NewPeerConnection(config)

			if err != nil {
				log.Println(err)
				peerConnection.Close()
				return
			}
			defer peerConnection.Close()

			controlsReady <- false

			peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
				log.Println("OnICECandidate bot:", id, c)
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
					arenaCandidateChan <- candidate
				}
			})

			peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
				connectionStateString := connectionState.String()
				log.Println("ICE Connection State has changed:", id, connectionStateString)
				webRTCConnectionStateChan <- connectionStateString
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
						case msg := <-sendDataChan:
							err := d.SendText(msg)
							if err != nil {
								log.Println(err)
							}

						case <-quitWebRTCChan:
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

						if !*areControlsAllowedBySupervisor {
							log.Println("Controls blocked by supervisor")
							break
						}
						if !*isReady {
							log.Println("Controls are not allowed yet:", id)
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

						command := fmt.Sprintf("{\"address\":%d,\"controls\":%s}", id, data.Payload)

						serialWrite <- command

					case "READY":
						controlsReady <- true
					}

				})
			})

			for _, track := range mediaStream.GetTracks() {
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

			arenaDescriptionChan <- *peerConnection.LocalDescription()

			candidatesMux.Lock()

			for _, c := range pendingCandidates {
				candidate := c.ToJSON()
				arenaCandidateChan <- candidate
			}

			candidatesMux.Unlock()

		case candidate := <-candidateChan:
			err := peerConnection.AddICECandidate(candidate)
			if err != nil {
				log.Println(err)
			}

		case <-quitWebRTCChan:
			if peerConnection != nil {
				peerConnection.Close()
			}
			go Init(
				id,
				stunUrls,
				api,
				mediaStream,
				descriptionChan,
				candidateChan,
				arenaDescriptionChan,
				arenaCandidateChan,
				webRTCConnectionStateChan,
				sendDataChan,
				quitWebRTCChan,
				serialWrite,
				controlsReady,
				areControlsAllowedBySupervisor,
				isReady,
			)
			return
		}

	}

}
