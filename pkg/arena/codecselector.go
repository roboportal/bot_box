//go:build !arm
// +build !arm

package arena

import (
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/x264"

	_ "github.com/pion/mediadevices/pkg/driver/camera"
)

func getCodecSelector(bitRate int) *mediadevices.CodecSelector {
	x264Params, err := x264.NewParams()

	if err != nil {
		panic(err)
	}

	x264Params.Preset = x264.PresetMedium
	x264Params.BitRate = bitRate

	opusParams, err := opus.NewParams()

	if err != nil {
		panic(err)
	}

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	return codecSelector
}
