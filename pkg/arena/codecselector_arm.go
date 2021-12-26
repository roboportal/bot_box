// +build arm

package arena

import (
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/mmal"
	"github.com/pion/mediadevices/pkg/codec/opus"
)

func getCodecSelector(bitRate int) *mediadevices.CodecSelector {
	mmalParams, err := mmal.NewParams()

	if err != nil {
		panic(err)
	}

	mmalParams.BitRate = bitRate

	opusParams, err := opus.NewParams()

	if err != nil {
		panic(err)
	}

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&mmalParams),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	return codecSelector
}
