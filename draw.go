package tad


import (
	"github.com/cosmouser/tnt"
	"github.com/fogleman/gg"
	log "github.com/sirupsen/logrus"
	"math"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"time"
	"io"
)

func drawGif(w io.Writer, frames []playbackFrame, mapPic image.Image, rect image.Rectangle) error {
	outGif := gif.GIF{}
	maxDim := math.Max(float64(mapPic.Bounds().Size().X), float64(mapPic.Bounds().Size().Y))
	var scale float64
	if rect.Size().X > rect.Size().Y {
		scale = maxDim / float64(rect.Size().X)
	} else {
		scale = maxDim / float64(rect.Size().Y)
	}
	ts1 := time.Now()
	playerColors := []color.RGBA{
		tnt.TAPalette[252].(color.RGBA),
		tnt.TAPalette[249].(color.RGBA),
		tnt.TAPalette[17].(color.RGBA),
		tnt.TAPalette[250].(color.RGBA),
		tnt.TAPalette[36].(color.RGBA),
		tnt.TAPalette[218].(color.RGBA),
		tnt.TAPalette[208].(color.RGBA),
		tnt.TAPalette[93].(color.RGBA),
		tnt.TAPalette[100].(color.RGBA),
		tnt.TAPalette[210].(color.RGBA),
	}
	for i := range frames {
		dc := gg.NewContext(mapPic.Bounds().Size().X, mapPic.Bounds().Size().Y)
		dc.DrawImage(mapPic, 0, 0)
		// log.Printf("Frame %d: %+v", i, frames[i].Units) 
		for _, tau := range frames[i].Units {
			if tau != nil && tau.Finished {
				dc.SetColor(tnt.TAPalette[0x55])
				dc.DrawPoint(scale*float64(tau.XPos), scale*float64(tau.YPos), 3.8)
				dc.Fill()
				dc.SetColor(playerColors[tau.Owner-1])
				dc.DrawPoint(scale*float64(tau.XPos), scale*float64(tau.YPos), 3)
				dc.Fill()
			}
		}
		imgItem := dc.Image()
		palettedImage := image.NewPaletted(imgItem.Bounds(), tnt.TAPalette)
		draw.Draw(palettedImage, palettedImage.Rect, imgItem, imgItem.Bounds().Min, draw.Over)
		outGif.Image = append(outGif.Image, palettedImage)
		outGif.Delay = append(outGif.Delay, 0)
	}
	log.Printf("drawing frames took %v", time.Since(ts1))
	ts2 := time.Now()
	gif.EncodeAll(w, &outGif)
	log.Printf("encoding took %v", time.Since(ts2))
	log.WithFields(log.Fields{
		"numFrames": len(frames),
	}).Info()

	return nil
}
