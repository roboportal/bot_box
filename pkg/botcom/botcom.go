package botcom

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/pion/mediadevices"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/roboportal/bot_box/pkg/utils"
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
	ClosePeerConnectionChan           chan struct{}
	QuitWebRTCChan                    chan struct{}
	BotCommandsWriteChan              chan string
	ControlsReadyChan                 chan bool
	GetAreControlsAllowedBySupervisor func() bool
	GetAreBotsReady                   func() bool
	IsAudioOutputEnabled              bool
}

func haltControls(botCommandsWriteChan chan string, id int) {
	command := fmt.Sprintf("{\"address\":%d,\"controls\":{\"stop\":true}}", id)
	botCommandsWriteChan <- command
}

func enableControls(botCommandsWriteChan chan string, id int) {
	command := fmt.Sprintf("{\"address\":%d,\"controls\":{\"start\":true}}", id)
	botCommandsWriteChan <- command
}

func Init(p InitParams) {
	for {
		var wg sync.WaitGroup

		var candidatesMux sync.Mutex
		var peerConnection *webrtc.PeerConnection
		var err error

		config := webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: p.StunUrls,
				},
			},
		}

		pendingCandidates := make([]*webrtc.ICECandidate, 0)

		closeDataChannelChan := make(chan struct{})

		doneAudioTrack := make(chan bool)

		log.Println("Startig webrtc loop")

		for loop := true; loop; {
			select {

			case description := <-p.DescriptionChan:

				peerConnection, err = p.Api.NewPeerConnection(config)

				if err != nil {
					log.Println("Create peerConnection error", err)
					loop = false
					break
				}

				p.ControlsReadyChan <- false

				log.Println("Disable controls on webrtc start")

				peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
					if !p.IsAudioOutputEnabled {
						return
					}

					go func() {
						wg.Add(1)
						defer wg.Done()

						ticker := time.NewTicker(time.Second * 3)
						defer ticker.Stop()
						for range ticker.C {
							select {

							case <-doneAudioTrack:
								return

							default:
								err := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
								if err != nil {
									log.Println("peerConnection.WriteRTCP error", err)
								}
							}
						}
					}()

					sr := beep.SampleRate(48000)

					speaker.Init(sr, sr.N(time.Second/5))
					speaker.Play(Sound(track))

					defer speaker.Clear()
					defer speaker.Close()

					<-doneAudioTrack
				})

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

						state := p.GetAreControlsAllowedBySupervisor()

						enableControls(p.BotCommandsWriteChan, p.Id)

						status := "DECLINED"

						if state {
							status = "ALLOWED"
						}

						command := fmt.Sprintf("{\"type\": \"CONTROLS_SUPERVISOR_STATUS_CHANGE\", \"payload\": {\"status\": \"%s\"}}", status)

						d.SendText(command)

						for loop := true; loop; {
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
								haltControls(p.BotCommandsWriteChan, p.Id)
								defer d.Close()
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
								return
							}

							command := fmt.Sprintf("{\"address\":%d,\"controls\":%s}", p.Id, data.Payload)

							p.BotCommandsWriteChan <- command

						case "READY":
							enableControls(p.BotCommandsWriteChan, p.Id)
							p.ControlsReadyChan <- true

						case "NOT_READY":
							haltControls(p.BotCommandsWriteChan, p.Id)
							p.ControlsReadyChan <- false
						}

					})
				})

				for _, track := range p.MediaStream.GetTracks() {
					track.OnEnded(func(err error) {
						log.Println("Track ended with error:", track.ID(), err)
					})

					_, err = peerConnection.AddTransceiverFromTrack(track,
						webrtc.RtpTransceiverInit{
							Direction: webrtc.RTPTransceiverDirectionSendrecv.Revers(),
						},
					)
					if err != nil {
						log.Println("AddTransceiverFromTrack to peerConnection error", err)
						loop = false
						break
					}
				}

				if loop == false {
					break
				}

				err = peerConnection.SetRemoteDescription(description)

				if err != nil {
					log.Println("SetRemoteDescription to peerConnection error", err)
					loop = false
					break
				}

				answer, err := peerConnection.CreateAnswer(nil)

				if err != nil {
					log.Println("CreateAnswer for Offer error", err)
					loop = false
					break
				}

				err = peerConnection.SetLocalDescription(answer)

				if err != nil {
					log.Println("SetLocalDescription error", err)
					loop = false
					break
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

			case <-p.ClosePeerConnectionChan:
				log.Println("Closing peer connection")

				if peerConnection != nil && peerConnection.ICEConnectionState() == webrtc.ICEConnectionStateConnected {
					peerConnection.Close()
				}

			case <-p.QuitWebRTCChan:
				log.Println("Quitting WebRTC for bot:", p.Id)
				go utils.NicelyClose(closeDataChannelChan)

				loop = false
				break
			}
		}

		if peerConnection != nil {
			log.Println(peerConnection.ICEConnectionState().String())
		}

		close(doneAudioTrack)

		log.Println("Awaiting for audio gorutine to finish.")

		wg.Wait()
	}
}
