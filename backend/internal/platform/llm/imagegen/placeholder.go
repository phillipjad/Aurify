package imagegen

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"io"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// placeholderSize is deliberately small. The placeholder exists so the pipeline
// completes without credentials, not so it looks like cover art.
const placeholderSize = 512

// placeholderImage renders a gradient derived from the prompt, for when no
// image provider is configured.
//
// It draws locally rather than pointing at a placeholder image service, because
// the bytes have to reach ports.ImageStore either way, and a URL to somewhere
// else would mean development covers stop rendering without a network and would
// send every generated prompt's existence to a third party.
//
// Two colors seeded by the prompt make different playlists visibly different,
// which is what makes a stale cover obvious while iterating.
func placeholderImage(prompt string) domain.GeneratedImage {
	h := fnv.New64a()
	_, _ = io.WriteString(h, prompt)
	seed := h.Sum64()

	top := color.RGBA{R: byte(seed), G: byte(seed >> 8), B: byte(seed >> 16), A: 0xFF}
	bottom := color.RGBA{R: byte(seed >> 24), G: byte(seed >> 32), B: byte(seed >> 40), A: 0xFF}

	img := image.NewRGBA(image.Rect(0, 0, placeholderSize, placeholderSize))
	for y := range placeholderSize {
		row := color.RGBA{
			R: mix(top.R, bottom.R, y),
			G: mix(top.G, bottom.G, y),
			B: mix(top.B, bottom.B, y),
			A: 0xFF,
		}
		for x := range placeholderSize {
			img.SetRGBA(x, y, row)
		}
	}

	// bytes.Buffer never fails a write, so png.Encode cannot fail here.
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return domain.GeneratedImage{Bytes: buf.Bytes(), ContentType: "image/png"}
}

// mix interpolates between two channel values across the image's height.
func mix(from, to byte, y int) byte {
	span := placeholderSize - 1
	return byte((int(from)*(span-y) + int(to)*y) / span)
}
