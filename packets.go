package tad

import (
	"fmt"
)

type taPacket interface {
	printMessage(map[uint16]string, map[uint16]uint16) string
	GetMarker() byte
}

type packetDefault struct {
	Marker byte
	Data   []byte
}

func (p *packetDefault) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: %v", p.Marker, p.Data)
}
func (p *packetDefault) GetMarker() byte {
	return p.Marker
}

// Player status
type packet0x28 struct {
	Marker    byte
	Status    byte
	Kills     int32
	Losses    int32
	ComKills  int32
	ComLosses int32
	StoredM   float32
	StoredE   float32
	StorageM  float32
	StorageE  float32
	TotalE    float32
	Unknown1  [4]byte
	ExcessE   float32
	TotalM    float32
	Unknown2  [4]byte
	ExcessM   float32
}

func (p *packet0x28) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: reported %d kills, %d losses, StoredM: %f, StoredE: %f and Status: %v",
		p.Marker,
		p.Kills,
		p.Losses,
		p.StoredM,
		p.StoredE,
		p.Status)
}
func (p *packet0x28) GetMarker() byte {
	return p.Marker
}

// Starting to build unit
type packet0x09 struct {
	Marker   byte
	NetID    uint16
	UnitID   uint16
	Unknown1 uint16
	XPos     uint16
	Unknown2 uint16
	ZPos     uint16
	Unknown3 uint16
	YPos     uint16
	Unknown4 uint16
	Unknown5 [4]byte
}

func (p *packet0x09) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: started building a %v  at X: %v, Y: %v, Z: %v and assigned it an ID of %v",
		p.Marker,
		unitNames[p.NetID],
		p.UnitID,
		p.XPos,
		p.YPos,
		p.ZPos)
}
func (p *packet0x09) GetMarker() byte {
	return p.Marker
}

// Unit destroyed
type packet0x0c struct {
	Marker    byte
	Destroyed uint16
	Unknown1  uint32
	Destroyer uint16
	Unkonwn2  uint16
}

func (p *packet0x0c) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: %v (%04x) destroyed %v (%04x)",
		p.Marker,
		unitNames[unitMem[p.Destroyer]],
		p.Destroyer,
		unitNames[unitMem[p.Destroyed]],
		p.Destroyed)
}
func (p *packet0x0c) GetMarker() byte {
	return p.Marker
}

// Map view position
type packet0xfc struct {
	Marker byte
	XPos   uint16
	YPos   uint16
}

func (p *packet0xfc) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: moved screen to X: %v, Y: %v",
		p.Marker,
		p.XPos,
		p.YPos)
}
func (p *packet0xfc) GetMarker() byte {
	return p.Marker
}

// Unit has been built
type packet0x12 struct {
	Marker    byte
	BuiltID   uint16
	BuiltByID uint16
}

func (p *packet0x12) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: %v (%04x) was built in part or in full by %v (%04x)",
		p.Marker,
		unitNames[unitMem[p.BuiltID]],
		p.BuiltID,
		unitNames[unitMem[p.BuiltByID]],
		p.BuiltByID)
}

func (p *packet0x12) GetMarker() byte {
	return p.Marker
}

// Damage
type packet0x0b struct {
	Marker       byte
	DamagedID    uint16
	DamagerID    uint16
	Damage       uint16
	Unknown      byte
	WeaponNumber uint8
}

func (p *packet0x0b) printMessage(unitNames map[uint16]string, unitMem map[uint16]uint16) string {
	return fmt.Sprintf("%02x: %v (%04x) dealt %d damage to %v (%04x) with weapon %d",
		p.Marker,
		unitNames[unitMem[p.DamagerID]],
		p.DamagerID,
		p.Damage,
		unitNames[unitMem[p.DamagedID]],
		p.DamagedID,
		p.WeaponNumber)
}
func (p *packet0x0b) GetMarker() byte {
	return p.Marker
}

// Unknown
type packet0x0d struct {
	Marker   byte
	Unknown1 [32]byte
	UnitID1  uint16
	UnitID2  uint16
	Unknown2 byte
}

// Unknown
type packet0x11 struct {
	Marker  byte
	UnitID  uint16
	Unknown byte
}

// Explosions ("displayed in wrong place?" - SY)
type packet0x10 struct {
	Marker    byte
	UnitID    uint16
	Unknown1  byte
	Unknown2  byte
	Unknown3  byte
	Unknown4  uint16
	Unknown5  uint16
	Unknown6  uint16
	Unknown8  uint32
	Unknown9  uint32
	Unknown10 uint16
}