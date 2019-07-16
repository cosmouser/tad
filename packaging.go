package tad

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/google/uuid"
	"io"
	"math"
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
func FinalScoresWorker(stream chan PacketRec, pnameMap map[byte]string) (finalScores []FinalScore, foulPlay []string, err error) {
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
				foulPlay = append(foulPlay, pnameMap[pr.Sender])
			}
			if int(sp.Kills) < finalScores[smap[pr.Sender]].Kills {
				foulPlay = append(foulPlay, pnameMap[pr.Sender])
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
func UnitCountWorker(stream chan PacketRec) (uc []map[int]int, err error) {
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
func smoothUnitMovement(frames []PlaybackFrame, colorMap map[int]int) {
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
