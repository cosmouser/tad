package tad

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

// Unit destroyed
type packet0x0c struct {
	Marker    byte
	Destroyed uint16
	Unknown1  uint32
	Destroyer uint16
	Unkonwn2  uint16
}

// Map view position
type packet0xfc struct {
	Marker byte
	XPos   uint16
	YPos   uint16
}

// Unit has been built
type packet0x12 struct {
	Marker    byte
	BuiltID   uint16
	BuiltByID uint16
}

// Damage
type packet0x0b struct {
	Marker       byte
	DamagerID    uint16
	DamagedID    uint16
	Damage       uint16
	Unknown      byte
	WeaponNumber uint8
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
