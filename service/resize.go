package services

import (
	"bytes"
	"image"
	"image/color/palette"
	"image/gif"
	"log"

	"github.com/babilu-online/common/context"
	"github.com/nfnt/resize"
	"golang.org/x/image/draw"

	"image/jpeg"
	"image/png"
	"io"

	_ "golang.org/x/image/vp8"
	_ "golang.org/x/image/webp"
)

type ResizeService struct {
	context.DefaultService
}

const RESIZE_SVC = "resize_svc"

func (svc ResizeService) Id() string {
	return RESIZE_SVC
}

func (svc *ResizeService) Start() error {
	return nil
}

func (svc *ResizeService) Resize(data []byte, out io.Writer, size int) error {
	src, imgType, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	if imgType == "gif" {
		resizedGif, err := svc.resizeGif(data, 0, size/2)
		if err != nil {
			return err
		}

		return gif.EncodeAll(out, resizedGif)
	}

	// Resize:
	dst := resize.Resize(0, uint(size), src, resize.MitchellNetravali)

	switch imgType {
	case "png":
		return png.Encode(out, dst)
	case "jpeg", "jpg":
		return jpeg.Encode(out, dst, &jpeg.Options{Quality: 100})
	default:
		log.Printf("Unsupported media type (%s), encoding as jpeg", imgType)
		return jpeg.Encode(out, dst, &jpeg.Options{Quality: 100})
	}
}

func (svc *ResizeService) resizeGif(data []byte, width, height int) (*gif.GIF, error) {
	gifImage, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	if width == 0 {
		width = int(gifImage.Config.Width * height / gifImage.Config.Width)
	} else if height == 0 {
		height = int(width * gifImage.Config.Height / gifImage.Config.Width)
	}

	// reset the gif width and height
	gifImage.Config.Width = width
	gifImage.Config.Height = height

	firstFrame := gifImage.Image[0].Bounds()
	img := image.NewRGBA(image.Rect(0, 0, firstFrame.Dx(), firstFrame.Dy()))

	// resize frame by frame
	for index, frame := range gifImage.Image {
		frameBounds := frame.Bounds()
		draw.Draw(img, frameBounds, frame, frameBounds.Min, draw.Over)
		gifImage.Image[index] = svc.imageToPaletted(resize.Resize(uint(width), uint(height), img, resize.MitchellNetravali))
	}

	return gifImage, nil
}

func (svc *ResizeService) imageToPaletted(img image.Image) *image.Paletted {
	frameBounds := img.Bounds()
	palettedImg := image.NewPaletted(frameBounds, palette.Plan9)
	draw.FloydSteinberg.Draw(palettedImg, frameBounds, img, image.ZP)
	return palettedImg
}
