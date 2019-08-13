package tad

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/cosmouser/tnt"
	"github.com/fogleman/gg"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Analyze opens up a demo and gives back the game's data and a channel of its packets
// for analysis.
func Analyze(ctx context.Context, rs io.ReadSeeker) (gp *Game, prs <-chan PacketRec, err error) {
	// Section 1
	// Analyzes and load data into the Game struct to be returned
	sum, err := parseSummary(rs)
	if err != nil {
		return nil, nil, err
	}
	gp = new(Game)
	gp.MapName = string(bytes.Split(sum.MapName[:], []byte{0})[0])
	gp.MaxUnits = int(sum.MaxUnits)
	gp.Players = make([]DemoPlayer, int(sum.NumPlayers))
	err = loadExtraSectors(rs, gp)
	if err != nil {
		return
	}
	for i := 0; i < len(gp.Players); i++ {
		err = parseAndCopyPlayer(rs, &gp.Players[i])
		if err != nil {
			return
		}
	}
	for i := 0; i < len(gp.Players); i++ {
		err = parseAndCopyStatMsg(rs, &gp.Players[i])
		if err != nil {
			return
		}
	}
	err = parseAndCopyUnitSyncData(rs, gp)
	if err != nil {
		return
	}
	gameOffset := getGameOffset(rs)
	err = getGameLengthAndTTD(rs, gp)
	if err != nil {
		return
	}

	// Section 2
	// Creates a stream of PacketRecs for consuming
	nExpected, err := rs.Seek(gameOffset, io.SeekStart)
	if err != nil || nExpected != gameOffset {
		err = errors.New("seek to gameoffset failed")
		return
	}
	prs = prGenerator(ctx, rs, gp.TotalMoves, gp.MaxUnits)
	return
}

// TeamsWorker consumes packets from a stream and returns the numbers of the
// players that the recording player has allied
func TeamsWorker(stream chan PacketRec, gp Game) (allies []int, err error) {
	alliedTimer := make([]int, 10)
	alliedTo := make([]bool, 10)
	alliedBy := make([]bool, 10)
	tdpidMap := make(map[int32]byte)
	for _, p := range gp.Players {
		tdpidMap[p.TDPID] = p.Number - 1
	}
	var moveCounter int
	var lastToken string
	for pr := range stream {
		if pr.IdemToken != lastToken {
			moveCounter++
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x23 {
			tmp := packet0x23{}
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &tmp)
			if err != nil {
				return
			}
			if tmp.Status == 1 {
				if tdpidMap[tmp.Player] == 0 {
					alliedTo[tdpidMap[tmp.Allied]] = true
				} else {
					alliedBy[tdpidMap[tmp.Player]] = true
				}
			} else {
				if tdpidMap[tmp.Player] == 0 {
					alliedTo[tdpidMap[tmp.Allied]] = false
				} else {
					alliedBy[tdpidMap[tmp.Player]] = false
				}
			}
		}
		for i := range alliedTo {
			if alliedTo[i] && alliedBy[i] {
				alliedTimer[i]++
			}
		}
	}
	for i := range alliedTimer {
		if float64(alliedTimer[i])/float64(gp.TotalMoves) > 0.80 {
			allies = append(allies, i)
		}
	}
	return
}

// ScoreSeriesWorker consumes 0x28 packets from a stream and adds them to a map
func ScoreSeriesWorker(stream chan PacketRec, pnameMap map[byte]string) (series map[string][]SPLite, err error) {
	series = make(map[string][]SPLite)
	seriesFull := make(map[string][]packet0x28)
	var (
		scorePacket packet0x28
		litePacket  SPLite
		ediff       float64
		mdiff       float64
		tdiff       float64
	)
	var clock int
	var lastToken string
	for pr := range stream {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x28 {
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &scorePacket)
			if err != nil {
				return nil, err
			}
			if len(series[pnameMap[pr.Sender]]) == 0 {
				series[pnameMap[pr.Sender]] = append(series[pnameMap[pr.Sender]], SPLite{Milliseconds: clock})
			}
			if len(seriesFull[pnameMap[pr.Sender]]) == 0 {
				seriesFull[pnameMap[pr.Sender]] = append(seriesFull[pnameMap[pr.Sender]], packet0x28{})
			}
			ediff = float64(scorePacket.TotalE - seriesFull[pnameMap[pr.Sender]][len(seriesFull[pnameMap[pr.Sender]])-1].TotalE)
			mdiff = float64(scorePacket.TotalM - seriesFull[pnameMap[pr.Sender]][len(seriesFull[pnameMap[pr.Sender]])-1].TotalM)
			tdiff = float64(clock - series[pnameMap[pr.Sender]][len(series[pnameMap[pr.Sender]])-1].Milliseconds)
			litePacket.Energy = (ediff / tdiff) * 1000
			litePacket.Metal = (mdiff / tdiff) * 1000
			litePacket.Kills = int(scorePacket.Kills)
			litePacket.Losses = int(scorePacket.Losses)
			litePacket.TotalE = float64(scorePacket.TotalE)
			litePacket.TotalM = float64(scorePacket.TotalM)
			litePacket.ExcessE = float64(scorePacket.ExcessE)
			litePacket.ExcessM = float64(scorePacket.ExcessM)
			if math.IsNaN(litePacket.Energy) || math.IsInf(litePacket.Energy, 1) {
				litePacket.Energy = 1
			}
			if math.IsNaN(litePacket.Metal) || math.IsInf(litePacket.Metal, 1) {
				litePacket.Metal = 1
			}
			litePacket.Milliseconds = clock
			if litePacket.Metal > 1.0 || litePacket.Energy > 1.0 {
				series[pnameMap[pr.Sender]] = append(series[pnameMap[pr.Sender]], litePacket)
			}
			seriesFull[pnameMap[pr.Sender]] = append(seriesFull[pnameMap[pr.Sender]], scorePacket)
		}
	}
	return
}

// FinalScoresWorker consumes packets from a stream and returns the final scores from the game
func FinalScoresWorker(stream chan PacketRec, pnameMap map[byte]string) (finalScores []FinalScore, foulPlay []int, err error) {
	var sp packet0x28
	var c int
	smap := make(map[byte]int)
	for k := range pnameMap {
		smap[k] = c
		c++
	}
	finalScores = make([]FinalScore, c)
	for k := range pnameMap {
		finalScores[smap[k]].Player = pnameMap[k]
	}
	for pr := range stream {
		if _, ok := pnameMap[pr.Sender]; ok && pr.Data[0] == 0x28 {
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &sp)
			if err != nil {
				return nil, nil, err
			}
			if int(sp.Losses) < finalScores[smap[pr.Sender]].Losses {
				foulPlay = append(foulPlay, int(pr.Sender)-1)
			}
			if int(sp.Kills) < finalScores[smap[pr.Sender]].Kills {
				foulPlay = append(foulPlay, int(pr.Sender)-1)
			}
			if float64(sp.TotalE) < finalScores[smap[pr.Sender]].TotalE {
				foulPlay = append(foulPlay, int(pr.Sender)-1)
			}
			if float64(sp.TotalM) < finalScores[smap[pr.Sender]].TotalM {
				foulPlay = append(foulPlay, int(pr.Sender)-1)
			}
			finalScores[smap[pr.Sender]].Status = int(sp.Status)
			finalScores[smap[pr.Sender]].Won = int(sp.ComKills)
			finalScores[smap[pr.Sender]].Lost = int(sp.ComLosses)
			finalScores[smap[pr.Sender]].Kills = int(sp.Kills)
			finalScores[smap[pr.Sender]].Losses = int(sp.Losses)
			finalScores[smap[pr.Sender]].TotalE = float64(sp.TotalE)
			finalScores[smap[pr.Sender]].ExcessE = float64(sp.ExcessE)
			finalScores[smap[pr.Sender]].TotalM = float64(sp.TotalM)
			finalScores[smap[pr.Sender]].ExcessM = float64(sp.ExcessM)
		}
	}
	return
}

// UnitCountWorker consumes packets from a stream and returns a count of units built in the game
func UnitCountWorker(stream chan PacketRec) (uc []map[int]unitTypeRecord, err error) {
	uc = make([]map[int]int, 10)
	unitmem := make(map[uint16]*TAUnit)
	for pr := range stream {
		if pr.Data[0] == 0x09 {
			tmp := &packet0x09{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			unitmem[tmp.UnitID] = &TAUnit{
				Owner:    int(pr.Sender),
				NetID:    tmp.NetID,
				Finished: false,
				ID:       uuid.New().String(),
			}
		}
		if pr.Data[0] == 0x12 {
			tmp := &packet0x12{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.BuiltID]; ok && tau != nil && !unitmem[tmp.BuiltID].Finished {
				unitmem[tmp.BuiltID].Finished = true
				if uc[int(pr.Sender)-1] == nil {
					uc[int(pr.Sender)-1] = make(map[int]int)
				}
				uc[int(pr.Sender)-1][int(unitmem[tmp.BuiltID].NetID)]++
			}
		}
	}
	return
}

// TimeToDieWorker finds out when each player dies
func TimeToDieWorker(stream chan PacketRec, gp Game) (ttd [10]int, err error) {
	var clock int
	var lastToken string
	for pr := range stream {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x0c {
			tmp := &packet0x0c{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return ttd, err
			}
			if int(tmp.Destroyed)%gp.MaxUnits == 1 {
				// pr.Sender - 1 is now dead
				ttd[int(pr.Sender)-1] = clock
			}
		}
		if pr.Data[0] == 0x1b {
			tdpid := binary.LittleEndian.Uint32(pr.Data[1:5])
			sv := pr.Data[5]
			if sv == 6 && int32(tdpid) == gp.Players[int(pr.Sender)-1].TDPID {
				// pr.Sender -1 is now rejected
				ttd[int(pr.Sender)-1] = clock
			}
		}
	}
	// add a millisecond for difference
	clock++
	for i := range gp.Players {
		if ttd[i] == 0 && gp.Players[i].Side != 2 {
			ttd[i] = clock
		}
	}
	return
}

// FramesWorker consumes packets from a stream and returns a series of PlaybackFrames for
// drawing a GIF
func FramesWorker(stream chan PacketRec, maxUnits int) (frames []PlaybackFrame, err error) {
	unitmem := make(map[uint16]*TAUnit)
	addFrame := func(tval int) {
		newFrame := PlaybackFrame{}
		newFrame.Time = tval
		newFrame.Number = len(frames)
		newFrame.Units = make(map[uint16]*TAUnit)
		for k, v := range unitmem {
			newFrame.Units[k] = new(TAUnit)
			newFrame.Units[k].Owner = v.Owner
			newFrame.Units[k].NetID = v.NetID
			newFrame.Units[k].Finished = v.Finished
			newFrame.Units[k].Pos.X = v.Pos.X
			newFrame.Units[k].Pos.Y = v.Pos.Y
			newFrame.Units[k].Pos.Time = v.Pos.Time
			newFrame.Units[k].Pos.ID = v.Pos.ID
			newFrame.Units[k].ID = v.ID
			newFrame.Units[k].Class = v.Class
		}
		frames = append(frames, newFrame)
	}
	var clock, lastTime int
	var lastToken string
	var unitSpaces [10]uint16
	for pr := range stream {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x09 {
			tmp := &packet0x09{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			unitmem[tmp.UnitID] = &TAUnit{
				Owner:    int(pr.Sender),
				NetID:    tmp.NetID,
				Finished: false,
				Pos: point{
					X:    int(tmp.XPos),
					Y:    int(tmp.YPos),
					ID:   uuid.New().String(),
					Time: clock,
				},
				ID: uuid.New().String(),
			}
			// check to see if its the first unit aka commander
			if int(tmp.UnitID)%maxUnits == 1 {
				unitmem[tmp.UnitID].Finished = true
				unitmem[tmp.UnitID].Class = commanderClass
				unitSpaces[int(pr.Sender)-1] = tmp.UnitID
			}
		}
		if pr.Data[0] == 0x12 {
			tmp := &packet0x12{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.BuiltID]; ok && tau != nil {
				unitmem[tmp.BuiltID].Finished = true
			}
			if tau, ok := unitmem[tmp.BuiltByID]; ok && tau != nil {
				if tau.Class == factoryClass {
					if tau2, ok := unitmem[tmp.BuiltID]; ok && tau2 != nil {
						unitmem[tmp.BuiltID].Class = mobileClass
					}
				}
			}

		}
		if pr.Data[0] == 0x11 {
			tmp := &packet0x11{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			// 9 == factory is building
			if tmp.State == 9 {
				if tau, ok := unitmem[tmp.UnitID]; ok && tau != nil && tau.Class == buildingClass {
					tau.Class = factoryClass
				}
			}
			if tmp.State == 2 {
				if tau, ok := unitmem[tmp.UnitID]; ok && tau != nil && tau.Class == mobileClass {
					tau.Class = airClass
				}
			}
		}
		if pr.Data[0] == 0x0c {
			tmp := &packet0x0c{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.Destroyed]; ok || tau != nil {
				delete(unitmem, tmp.Destroyed)
			}
		}
		if pr.Data[0] == 0x0d {
			tmp := &packet0x0d{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.ShooterID]; ok && tau != nil {
				tau.Pos.X = int(tmp.OriginX)
				tau.Pos.Y = int(tmp.OriginY)
				tau.Pos.Time = clock
				tau.Pos.ID = uuid.New().String()
			}
			if tau, ok := unitmem[tmp.ShotID]; ok && tau != nil && tau.Class != buildingClass {
				tau.Pos.X = int(tmp.DestX)
				tau.Pos.Y = int(tmp.DestY)
				tau.Pos.Time = clock
				tau.Pos.ID = uuid.New().String()
			}
		}
		if pr.Data[0] == 0x2c && len(pr.Data) >= 0x1a {
			// if 0x9: - 0xc00 isn't the unitid's netid, ignore
			x2cUnitID := binary.LittleEndian.Uint16(pr.Data[0x7:])
			x2cNetID := binary.LittleEndian.Uint16(pr.Data[0x9:])
			x2cXPos := binary.LittleEndian.Uint16(pr.Data[0xb:])
			x2cYPos := binary.LittleEndian.Uint16(pr.Data[0xd:])
			x2cUnitID += unitSpaces[int(pr.Sender)-1]
			if tau, ok := unitmem[x2cUnitID]; ok && tau != nil {
				if x2cNetID-0xc00 == tau.NetID {
					tau.Pos.X = int(x2cXPos) * 16
					tau.Pos.Y = int(x2cYPos) * 16
					tau.Pos.Time = clock
					tau.Pos.ID = uuid.New().String()
				}
			}
		}
		if curTime := clock / 10000; curTime > lastTime {
			addFrame(clock)
			lastTime = curTime
		}
	}
	return
}

// DrawGif writes a gif of the frames and takes the max dimension of the output picture to
// scale the points to the image from their original coordinates.
func (gp *Game) DrawGif(w io.Writer, frames []PlaybackFrame, mapPic image.Rectangle, rect image.Rectangle) error {
	outGif := gif.GIF{
		Disposal: make([]byte, len(frames)),
		Image:    make([]*image.Paletted, len(frames)),
		Delay:    make([]int, len(frames)),
	}
	for i := range outGif.Disposal {
		outGif.Disposal[i] = gif.DisposalPrevious
	}
	for i := range outGif.Delay {
		outGif.Delay[i] = 10
	}
	maxDim := math.Max(float64(mapPic.Size().X), float64(mapPic.Size().Y))
	var scale float64
	if rect.Size().X > rect.Size().Y {
		scale = maxDim / float64(rect.Size().X)
	} else {
		scale = maxDim / float64(rect.Size().Y)
	}
	outMaxDimX := mapPic.Size().X
	outMaxDimY := mapPic.Size().Y
	ts1 := time.Now()
	gifPalette := tnt.TAPalette
	gifPalette[0] = image.Transparent
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
	frameGen := func() <-chan PlaybackFrame {
		frameStream := make(chan PlaybackFrame)
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

	frameDrawer := func(done <-chan interface{}, frameStream <-chan PlaybackFrame) <-chan numberedFrame {
		palettedStream := make(chan numberedFrame)
		go func() {
			defer close(palettedStream)
			for {
				select {
				case <-done:
					return
				case incomingFrame := <-frameStream:
					dc := gg.NewContext(outMaxDimX, outMaxDimY)
					for _, tau := range incomingFrame.Units {
						drawUnit(dc, tau, scale, playerColors)
					}
					imgItem := dc.Image()
					palettedImage := image.NewPaletted(imgItem.Bounds(), gifPalette)
					draw.Draw(palettedImage, palettedImage.Rect, imgItem, imgItem.Bounds().Min, draw.Over)
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

	log.Printf("drawing %d frames took %v at %f fps", len(frames), time.Since(ts1), float64(len(frames))/time.Since(ts1).Seconds())
	gif.EncodeAll(w, &outGif)
	log.WithFields(log.Fields{
		"numFrames": len(frames),
	}).Info()

	return nil
}

// SmoothUnitMovement uses a colorMap to sync colors and adjust unit positions
func SmoothUnitMovement(frames []PlaybackFrame, colorMap map[int]int) {
	nullPoint := point{
		X:    0,
		Y:    0,
		ID:   uuid.New().String(),
		Time: 0,
	}
	for i := range frames {
		for tauID, tau := range frames[i].Units {
			tau.Owner = colorMap[tau.Owner]
			toChange := []int{}
			if tau.NextPos.ID == "" {
				nextFrame := 0
				for f := i; f < len(frames); f++ {
					if unit, ok := frames[f].Units[tauID]; !ok || unit.ID != tau.ID {
						break
					}
					if tau.Pos.ID != frames[f].Units[tauID].Pos.ID {
						nextFrame = f
						break
					}
					toChange = append(toChange, f)
				}
				if nextFrame == 0 {
					tau.NextPos = nullPoint
				} else {
					for _, f := range toChange {
						frames[f].Units[tauID].NextPos = frames[nextFrame].Units[tauID].Pos
						frames[f].Units[tauID].updatePos(frames[f].Time)
					}
				}
			}
		}
	}
}

// UnitDataSeriesWorker creates points for unit building analysis
func UnitDataSeriesWorker(stream <-chan PacketRec) (out map[int][]UDSRecord, err error) {
	out = make(map[int][]UDSRecord)
	uc := make([]map[int]int, 10)
	unitmem := make(map[uint16]*TAUnit)
	series := make(map[int]SPLite)
	seriesFull := make(map[int][]packet0x28)
	var (
		scorePacket packet0x28
		litePacket  SPLite
		ediff       float64
		mdiff       float64
		tdiff       float64
		udsMain     UDSRecord
		lastSPLite  int
		clock       int
		lastToken   string
	)
	for pr := range stream {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x28 {
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &scorePacket)
			if err != nil {
				return nil, err
			}
			if len(seriesFull[int(pr.Sender)]) == 0 {
				seriesFull[int(pr.Sender)] = append(seriesFull[int(pr.Sender)], packet0x28{})
			}
			ediff = float64(scorePacket.TotalE - seriesFull[int(pr.Sender)][len(seriesFull[int(pr.Sender)])-1].TotalE)
			mdiff = float64(scorePacket.TotalM - seriesFull[int(pr.Sender)][len(seriesFull[int(pr.Sender)])-1].TotalM)
			tdiff = float64(clock - lastSPLite)
			litePacket.Energy = (ediff / tdiff) * 1000
			litePacket.Metal = (mdiff / tdiff) * 1000
			litePacket.Kills = int(scorePacket.Kills)
			litePacket.Losses = int(scorePacket.Losses)
			litePacket.TotalE = float64(scorePacket.TotalE)
			litePacket.TotalM = float64(scorePacket.TotalM)
			litePacket.ExcessE = float64(scorePacket.ExcessE)
			litePacket.ExcessM = float64(scorePacket.ExcessM)
			if math.IsNaN(litePacket.Energy) || math.IsInf(litePacket.Energy, 1) {
				litePacket.Energy = 1
			}
			if math.IsNaN(litePacket.Metal) || math.IsInf(litePacket.Metal, 1) {
				litePacket.Metal = 1
			}
			litePacket.Milliseconds = clock
			if litePacket.Metal > 1.0 || litePacket.Energy > 1.0 {
				series[int(pr.Sender)] = litePacket
			}
			seriesFull[int(pr.Sender)] = append(seriesFull[int(pr.Sender)], scorePacket)
			lastSPLite = clock
		}
		if pr.Data[0] == 0x09 {
			tmp := &packet0x09{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			unitmem[tmp.UnitID] = &TAUnit{
				Owner:    int(pr.Sender),
				NetID:    tmp.NetID,
				Finished: false,
				ID:       uuid.New().String(),
			}
		}
		if pr.Data[0] == 0x0c {
			tmp := &packet0x0c{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.Destroyed]; ok || tau != nil {
				if uc[int(pr.Sender)-1] == nil {
					uc[int(pr.Sender)-1] = make(map[int]int)
				}
				uc[int(pr.Sender)-1][int(unitmem[tmp.Destroyed].NetID)]--
				udsMain = UDSRecord{
					NetID:  int(unitmem[tmp.Destroyed].NetID),
					Count:  uc[int(pr.Sender)-1][int(unitmem[tmp.Destroyed].NetID)],
					SPLite: series[int(pr.Sender)],
				}
				delete(unitmem, tmp.Destroyed)
				if out[int(pr.Sender)] == nil {
					out[int(pr.Sender)] = []UDSRecord{}
				}
				out[int(pr.Sender)] = append(out[int(pr.Sender)], udsMain)
			}
		}
		if pr.Data[0] == 0x12 {
			tmp := &packet0x12{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				return nil, err
			}
			if tau, ok := unitmem[tmp.BuiltID]; ok && tau != nil && !unitmem[tmp.BuiltID].Finished {
				unitmem[tmp.BuiltID].Finished = true
				if uc[int(pr.Sender)-1] == nil {
					uc[int(pr.Sender)-1] = make(map[int]int)
				}
				uc[int(pr.Sender)-1][int(unitmem[tmp.BuiltID].NetID)]++
				udsMain = UDSRecord{
					NetID:  int(unitmem[tmp.BuiltID].NetID),
					Count:  uc[int(pr.Sender)-1][int(unitmem[tmp.BuiltID].NetID)],
					SPLite: series[int(pr.Sender)],
				}
				if out[int(pr.Sender)] == nil {
					out[int(pr.Sender)] = []UDSRecord{}
				}
				out[int(pr.Sender)] = append(out[int(pr.Sender)], udsMain)
			}
		}
	}
	return
}
