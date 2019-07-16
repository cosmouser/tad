package tad

import (
	"bytes"
	"context"
	"errors"
	"io"
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
