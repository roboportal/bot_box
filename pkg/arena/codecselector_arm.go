//go:build arm
// +build arm

package arena

import (
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	"github.com/pion/mediadevices/pkg/codec/opus"

	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
)

func getCodecSelector(bitRate int) *mediadevices.CodecSelector {
	vpxParams, err := vpx.NewVP8Params()

	if err != nil {
		panic(err)
	}

	vpxParams.BitRate = bitRate

	opusParams, err := opus.NewParams()

	if err != nil {
		panic(err)
	}

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&vpxParams),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	return codecSelector
}
