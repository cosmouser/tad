package tad

import (
	"github.com/cosmouser/tnt"
	"github.com/fogleman/gg"
	log "github.com/sirupsen/logrus"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"math"
	"runtime"
	"sync"
	"time"
)

type numberedFrame struct {
	Number   int
	Paletted *image.Paletted
}

type unitClass int
const (
	buildingClass unitClass = iota
	commanderClass
	mobileClass
	factoryClass
	airClass
)
func drawUnit(dc *gg.Context, t *taUnit, scale float64, colors []color.RGBA) {
	dc.SetColor(tnt.TAPalette[0x55])
	if t == nil || t.Finished == false {
		return
	}
	switch t.Class {
	case buildingClass:
		dc.DrawRectangle(scale*float64(t.Pos.X)-4, scale*float64(t.Pos.Y)-4, 8, 8)
		dc.Fill()
		dc.SetColor(colors[t.Owner-1])
		dc.DrawRectangle(scale*float64(t.Pos.X)-3, scale*float64(t.Pos.Y)-3, 6, 6)
		dc.Fill()
	case mobileClass:
		dc.DrawPoint(scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 3.8)
		dc.Fill()
		dc.SetColor(colors[t.Owner-1])
		dc.DrawPoint(scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 3)
		dc.Fill()
	case factoryClass:
		dc.DrawRoundedRectangle(scale*float64(t.Pos.X)-6, scale*float64(t.Pos.Y)-6, 12, 12, 1)
		dc.Fill()
		dc.SetColor(colors[t.Owner-1])
		dc.DrawRoundedRectangle(scale*float64(t.Pos.X)-5, scale*float64(t.Pos.Y)-5, 10, 10, 1)
		dc.Fill()
	case commanderClass:
		dc.DrawRegularPolygon(5, scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 5, 0)
		dc.Fill()
		dc.SetColor(colors[t.Owner-1])
		dc.DrawRegularPolygon(5, scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 4, 0)
		dc.Fill()
	case airClass:
		dc.DrawRegularPolygon(3, scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 5, 0)
		dc.Fill()
		dc.SetColor(colors[t.Owner-1])
		dc.DrawRegularPolygon(3, scale*float64(t.Pos.X), scale*float64(t.Pos.Y), 4, 0)
		dc.Fill()
	}
}

func (t *taUnit) updatePos(timeVal int) {
	vectorX := float64(t.NextPos.X - t.Pos.X)
	vectorY := float64(t.NextPos.Y - t.Pos.Y)
	magnitude := math.Sqrt((vectorX*vectorX)+(vectorY*vectorY))
	if magnitude == 0 {
		// unit stays in the same place
		// no update required
		return
	}
	unitVectorX := vectorX/magnitude
	unitVectorY := vectorY/magnitude
	timeDiff1 := float64(t.NextPos.Time - t.Pos.Time)
	timeDiff2 := float64(timeVal - t.Pos.Time)
	distanceModifier := timeDiff2/timeDiff1
	newX := t.Pos.X + int(unitVectorX * magnitude * distanceModifier)
	newY := t.Pos.Y + int(unitVectorY * magnitude * distanceModifier)
	t.Pos.X = newX
	t.Pos.Y = newY
	t.Pos.Time = timeVal
}

func drawGif(w io.Writer, frames []playbackFrame, mapPic image.Image, rect image.Rectangle) error {
	outGif := gif.GIF{}
	outGif.Image = make([]*image.Paletted, len(frames))
	outGif.Delay = make([]int, len(frames))
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
	frameGen := func() <-chan playbackFrame {
		frameStream := make(chan playbackFrame)
		go func() {
			defer close(frameStream)
			for i := range frames {
				frameStream <- frames[i]
			}
		}()
		return frameStream
	}
	done := make(chan interface{})
	frameStream := frameGen()
	frameDrawer := func(done <-chan interface{}, frameStream <-chan playbackFrame) <-chan numberedFrame {
		palettedStream := make(chan numberedFrame)
		go func() {
			defer close(palettedStream)
			for {
				select {
				case <-done:
					return
				case incomingFrame := <-frameStream:
					// draw the map image into each frame - slow 21 fps
					// dc := gg.NewContextForImage(mapPic)
					// draw just the points - fast 70 fps
					dc := gg.NewContext(mapPic.Bounds().Size().X, mapPic.Bounds().Size().Y)
					for _, tau := range incomingFrame.Units {
						drawUnit(dc, tau, scale, playerColors)
					}
					imgItem := dc.Image()
					palettedImage := image.NewPaletted(imgItem.Bounds(), tnt.TAPalette)
					draw.Draw(palettedImage, palettedImage.Rect, imgItem, imgItem.Bounds().Min, draw.Over)
					log.Printf("frame %d of %d drawn", incomingFrame.Number, len(frames))
					palettedStream <- numberedFrame{
						Number:   incomingFrame.Number,
						Paletted: palettedImage,
					}
				}
			}
		}()
		return palettedStream
	}
	fanIn := func(done <-chan interface{}, channels ...<-chan numberedFrame) <-chan numberedFrame {
		var wg sync.WaitGroup
		multiplexedStream := make(chan numberedFrame)
		multiplex := func(c <-chan numberedFrame) {
			defer wg.Done()
			for i := range c {
				select {
				case <-done:
					return
				case multiplexedStream <- i:
				}
			}
		}
		wg.Add(len(channels))
		for _, c := range channels {
			go multiplex(c)
		}
		go func() {
			wg.Wait()
			close(multiplexedStream)
		}()
		return multiplexedStream
	}
	numDrawers := runtime.NumCPU()
	drawers := make([]<-chan numberedFrame, numDrawers)
	for i := 0; i < numDrawers; i++ {
		drawers[i] = frameDrawer(done, frameStream)
	}
	frameStatus := make([]bool, len(frames))
	multiplexedStream := fanIn(done, drawers...)
	for f := range multiplexedStream {
		outGif.Image[f.Number] = f.Paletted
		frameStatus[f.Number] = true
		finished := false
		for _, v := range frameStatus {
			if v {
				finished = true
			} else {
				finished = false
				break
			}
		}
		if finished {
			break
		}
	}

	log.Printf("drawing frames took %v", time.Since(ts1))
	gif.EncodeAll(w, &outGif)
	log.WithFields(log.Fields{
		"numFrames": len(frames),
	}).Info()

	return nil
}
