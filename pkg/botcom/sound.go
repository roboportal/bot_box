package botcom

import (
	"io"
	"log"

	"github.com/faiface/beep"
	"github.com/pion/webrtc/v3"
	"gopkg.in/hraban/opus.v2"
)

func Sound(track *webrtc.TrackRemote) beep.Streamer {
	decoder, err := opus.NewDecoder(48000, 1)

	if err != nil {
		log.Println("opus.NewDecoder error", err)
	}

	tmp := make([]float32, 8192)

	tmpCount := 0

	return beep.StreamerFunc(func(samples [][2]float64) (n int, ok bool) {

		if tmpCount < len(samples) {
			data, _, err := track.ReadRTP()

			if data == nil {
				return 0, false
			}

			if err == io.EOF {
				return 0, false
			}

			if err != nil {
				log.Println("Stream fn error", err)
				return 0, false
			}

			pcm := make([]float32, 8192)

			n, err = decoder.DecodeFloat32(data.Payload, pcm)

			if err != nil {
				log.Println("Decode error", err)
			}

			tmp = append(tmp, pcm...)
			tmpCount += n
		}

		for i := range samples {
			samples[i][0] = float64(tmp[i])
			samples[i][1] = float64(tmp[i])
		}

		tmp = tmp[len(samples):tmpCount]
		tmpCount -= len(samples)

		if tmpCount < 0 {
			tmpCount = 0
		}

		return len(samples), true
	})
}