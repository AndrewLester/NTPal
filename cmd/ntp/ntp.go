package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"net"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"time"
)

const PORT = 123         // NTP port number
const VERSION byte = 4   // NTP version number
const TOLERANCE = 15e-6  //frequency tolerance PHI (s/s)
const MINPOLL int8 = 4   //minimum poll exponent (16 s)
const MAXPOLL int8 = 17  // maximum poll exponent (36 h)
const MAXDISP byte = 16  // maximum dispersion (16 s)
const MINDISP = 0.005    // minimum dispersion increment (s)
const NOSYNC byte = 0x3  // leap unsync
const MAXDIST byte = 1   // distance threshold (1 s)
const MAXSTRAT byte = 16 // maximum stratum number

const halfEraLength int64 = 65536         // 2^16
const eraLength int64 = 4_294_967_296     // 2^32
const unixEraOffset int64 = 2_208_988_800 // 1970 - 1900 in seconds

const SGATE = 3     /* spike gate (clock filter */
const BDELAY = .004 /* broadcast delay (s) */
const PHI = 15e-6   /* % frequency tolerance (15 ppm) */

type Mode byte

const (
	RESERVED Mode = iota
	SYMMETRIC_ACTIVE
	SYMMETRIC_PASSIVE
	CLIENT
	SERVER
	BROADCAST_SERVER
	NTP_CONTROL_MESSAGE
	RESERVED_PRIVAT_EUSE
)

type DispatchCode int

const ERR DispatchCode = -1
const (
	DSCRD = iota
	PROC
	BCST
	FXMIT
	MANY
	NEWPS
	NEWBC
)

// Index with [associationMode][packetMode]
var dispatchTable = [][]DispatchCode{
	{NEWPS, DSCRD, FXMIT, MANY, NEWBC},
	{PROC, PROC, DSCRD, DSCRD, DSCRD},
	{PROC, ERR, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, PROC, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, PROC},
}

type NTPDate struct {
	eraNumber int32
	eraOffset uint32
	fraction  uint64
}

type NTPTimestamp struct {
	seconds  uint32
	fraction uint32
}

type NTPTimestampEncoded = uint64

type NTPShort struct {
	seconds  uint16
	fraction uint16
}

type NTPShortEncoded = uint32

type Digest = uint32

type IPAddr = uint32

type NTPSystem struct {
	t            NTPTimestampEncoded /* update time */
	leap         byte                /* leap indicator */
	stratum      byte                /* stratum */
	poll         int8                /* poll interval */
	precision    int8                /* precision */
	rootdelay    float64             /* root delay */
	rootdisp     float64             /* root dispersion */
	refid        byte                /* reference ID */
	reftime      NTPTimestampEncoded /* reference time */
	m            [NMAX]M             /* chime list */
	v            [NMAX]V             /* survivor list */
	p            *P                  /* association ID */
	offset       float64             /* combined offset */
	jitter       float64             /* combined jitter */
	flags        int                 /* option flags */
	n            int                 /* number of survivors */
	associations map[netip.AddrPort]*Association
}

type Association struct {
	leap  byte /* leap indicator */
	hmode byte // HOST (Self) mode
	hpoll int8

	// Values set by received packet
	ReceivePacket

	reach byte
}

// Fields that can be read directly from the packet bytes
type EncodedReceivePacket struct {
	stratum   byte                /* stratum */
	poll      int8                /* poll interval */
	precision int8                /* precision */
	rootdelay NTPShortEncoded     /* root delay */
	rootdisp  NTPShortEncoded     /* root dispersion */
	refid     uint32              /* reference ID */
	reftime   NTPTimestampEncoded /* reference time */
	org       NTPTimestampEncoded /* origin timestamp */
	rec       NTPTimestampEncoded /* receive timestamp */
	xmt       NTPTimestampEncoded /* transmit timestamp */
}

type ReceivePacket struct {
	srcaddr netip.AddrPort      /* source (remote) address */
	dstaddr IPAddr              /* destination (local) address */
	leap    byte                /* leap indicator */
	version byte                /* version number */
	mode    Mode                /* mode */
	keyid   int32               /* key ID */
	mac     Digest              /* message digest */
	dst     NTPTimestampEncoded /* destination timestamp */
	EncodedReceivePacket
}

type TransmitPacket struct {
	dstaddr   IPAddr              /* source (local) address */
	srcaddr   IPAddr              /* destination (remote) address */
	leap      byte                /* leap indicator */
	version   byte                /* version number */
	mode      Mode                /* mode */
	stratum   byte                /* stratum */
	poll      int8                /* poll interval */
	precision int8                /* precision */
	rootdelay NTPShortEncoded     /* root delay */
	rootdisp  NTPShortEncoded     /* root dispersion */
	refid     byte                /* reference ID */
	reftime   NTPTimestampEncoded /* reference time */
	org       NTPTimestampEncoded /* origin timestamp */
	rec       NTPTimestampEncoded /* receive timestamp */
	xmt       NTPTimestampEncoded /* transmit timestamp */
	keyid     int                 /* key ID */
	dgst      Digest              /* message digest */
	EncodedReceivePacket
}

func SetupAsssociation(server string, wg *sync.WaitGroup) {
	wg.Add(2)
	go SetupPeer()
	go SetupPoll()
}

func DecodeRecvPacket(encoded []byte, clientAddr net.Addr, con net.PacketConn) (*ReceivePacket, error) {
	clientAddrPort, err := netip.ParseAddrPort(clientAddr.String())
	localAddrPort, err := netip.ParseAddrPort(con.LocalAddr().String())
	if err != nil {
		return nil, err
	}

	clientUDPAddr := net.UDPAddrFromAddrPort(clientAddrPort)
	localUDPAddr := net.UDPAddrFromAddrPort(localAddrPort)

	var leap, version, mode byte

	reader := bytes.NewReader(encoded)
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	leap = firstByte >> 6
	version = (firstByte >> 3) & 0b111
	mode = firstByte & 0b111

	encodedReceivePacket := EncodedReceivePacket{}
	if err := binary.Read(reader, binary.BigEndian, &encodedReceivePacket); err != nil {
		return nil, err
	}

	return &ReceivePacket{
		srcaddr:              clientAddrPort,
		dstaddr:              binary.BigEndian.Uint32(localUDPAddr.IP),
		leap:                 leap,
		version:              version,
		mode:                 Mode(mode),
		EncodedReceivePacket: encodedReceivePacket,
	}, nil
}

func EncodeTransmitPacket(packet TransmitPacket) []byte {
	var buffer bytes.Buffer
	firstByte := (packet.leap << 6) | (packet.version << 3) | byte(packet.mode)
	binary.Write(&buffer, binary.BigEndian, firstByte)
	binary.Write(&buffer, binary.BigEndian, &packet.EncodedReceivePacket)
	return buffer.Bytes()
}

func (system *NTPSystem) Receive(packet ReceivePacket) *TransmitPacket {
	if packet.version > VERSION {
		return nil
	}

	association, contains := system.associations[packet.srcaddr]
	hmode := byte(0)
	if contains {
		hmode = association.hmode
	}

	switch dispatchTable[hmode][packet.mode] {
	case FXMIT:
		// If the destination address is not a broadcast
		//    address

		/* not multicast dstaddr */

		// ignore auth
		// if (AUTH(p->flags & P_NOTRUST, auth))
		return system.reply(packet, SERVER)
		// else if (auth == A_ERROR)
		// 		fast_xmit(r, M_SERV, A_CRYPTO);
		// return;         /* M_SERV packet sent */
	case PROC:
		break
	case DSCRD:
		return nil
	}

	if packet.xmt == 0 {
		return nil
	}

	if packet.xmt == association.xmt {
		return nil
	}

	unsynch := packet.mode != BROADCAST_SERVER && (packet.org == 0 || packet.org != association.xmt)

	association.org = packet.xmt
	association.rec = packet.dst

	if unsynch {
		return nil
	}

	system.process(association, packet)
	return nil
}

func (system *NTPSystem) process(association *Association, packet ReceivePacket) {
	var offset float64 /* sample offsset */
	var delay float64  /* sample delay */
	var disp float64   /* sample dispersion */

	association.leap = packet.leap
	if packet.stratum == 0 {
		association.stratum = MAXSTRAT
	} else {
		association.stratum = packet.stratum
	}
	association.mode = packet.mode
	association.poll = packet.poll
	association.rootdelay = uint32(float64(packet.rootdelay) / float64(halfEraLength))
	association.rootdisp = uint32(float64(packet.rootdisp) / float64(halfEraLength))
	association.refid = packet.refid
	association.reftime = packet.reftime

	/*
	 * Verify the server is synchronized with valid stratum and
	 * reference time not later than the transmit time.
	 */
	if association.leap == NOSYNC || association.stratum >= MAXSTRAT {
		return /* unsynchronized */
	}

	/*
	 * Verify valid root distance.
	 */
	if packet.rootdelay/2+packet.rootdisp >= uint32(MAXDISP) || association.reftime >
		packet.xmt {

		return /* invalid header values */
	}

	poll_update(association, association.hpoll)
	association.reach |= 1

	/*
	 * Calculate offset, delay and dispersion, then pass to the
	 * clock filter.  Note carefully the implied processing.  The
	 * first-order difference is done directly in 64-bit arithmetic,
	 * then the result is converted to floating double.  All further
	 * processing is in floating-double arithmetic with rounding
	 * done by the hardware.  This is necessary in order to avoid
	 * overflow and preserve precision.
	 *
	 * The delay calculation is a special case.  In cases where the
	 * server and client clocks are running at different rates and
	 * with very fast networks, the delay can appear negative.  In
	 * order to avoid violating the Principle of Least Astonishment,
	 * the delay is clamped not less than the system precision.
	 */
	if association.mode == BROADCAST_SERVER {
		offset = NTPTimestampEncodedToDouble(packet.xmt - packet.dst)
		delay = BDELAY
		disp = Log2ToDouble(packet.precision) + Log2ToDouble(system.precision) + PHI*
			2*BDELAY
	} else {
		offset = (NTPTimestampEncodedToDouble(packet.rec-packet.org) + NTPTimestampEncodedToDouble(packet.dst-
			packet.xmt)) / 2
		delay = math.Max(NTPTimestampEncodedToDouble(packet.dst-packet.org)-NTPTimestampEncodedToDouble(packet.rec-
			packet.xmt), Log2ToDouble(system.precision))
		disp = Log2ToDouble(packet.precision) + Log2ToDouble(system.precision) + PHI*
			NTPTimestampEncodedToDouble(packet.dst-packet.org)
	}
	clock_filter(association, offset, delay, disp)
}

// TODO: Add auth
func (system *NTPSystem) reply(receivePacket ReceivePacket, mode Mode) *TransmitPacket {
	var transmitPacket TransmitPacket

	transmitPacket.version = receivePacket.version
	transmitPacket.srcaddr = receivePacket.dstaddr
	transmitPacket.dstaddr = receivePacket.srcaddr
	transmitPacket.leap = system.leap
	transmitPacket.mode = mode
	if system.stratum == MAXSTRAT {
		transmitPacket.stratum = 0
	} else {
		transmitPacket.stratum = system.stratum
	}
	transmitPacket.poll = receivePacket.poll
	transmitPacket.precision = system.precision
	transmitPacket.rootdelay = NTPShortEncoded(system.rootdelay * float64(halfEraLength))
	transmitPacket.rootdisp = NTPShortEncoded(system.rootdisp * float64(halfEraLength))
	transmitPacket.refid = system.refid
	transmitPacket.reftime = system.reftime
	transmitPacket.org = receivePacket.xmt
	transmitPacket.rec = receivePacket.dst
	transmitPacket.xmt = GetSystemTime()

	/*
	 * If the authentication code is A.NONE, include only the
	 * header; if A.CRYPTO, send a crypto-NAK; if A.OK, send a valid
	 * MAC.  Use the key ID in the received packet and the key in
	 * the local key cache.
	 */
	// 	 if (auth != A_NONE) {
	// 		if (auth == A_CRYPTO) {
	// 				x.keyid = 0;
	// 		} else {
	// 				x.keyid = r->keyid;
	// 				x.dgst = md5(x.keyid);
	// 		}
	// }

	return &transmitPacket
}

func GetSystemTime() NTPTimestampEncoded {
	var unixTime syscall.Timeval
	syscall.Gettimeofday(&unixTime)
	return (UnixToNTPTimestampEncoded(unixTime))
}

func StepTime(offset float64) {
	var unixTime syscall.Timeval
	syscall.Gettimeofday(&unixTime)

	ntpTime := DoubleToNTPTimestampEncoded(offset) + UnixToNTPTimestampEncoded(unixTime)
	unixTime.Sec = int64(ntpTime >> 32)
	unixTime.Usec = int32(((ntpTime - unixTime.Sec) <<
		32) / eraLength * 1e6)

	if os.Getenv("ENABLED") == "1" {
		syscall.Settimeofday(&unixTime)
	}
}

func UnixToNTPTimestampEncoded(time syscall.Timeval) NTPTimestampEncoded {
	return uint64((time.Sec+unixEraOffset)<<32) +
		uint64(int64(time.Usec/1e6)*eraLength)
}

func DoubleToNTPTimestampEncoded(offset float64) NTPTimestampEncoded {
	return NTPTimestampEncoded(offset * float64(eraLength))
}

func NTPTimestampEncodedToDouble(ntpTimestamp NTPTimestampEncoded) float64 {
	return float64(ntpTimestamp) / float64(eraLength)
}

func Log2ToDouble(a int8) float64 {
	if a < 0 {
		return 1.0 / float64(int32(1)<<-a)
	}
	return float64(int32(1) << a)
}

func TimeToNTPDate(time time.Time) NTPDate {
	s := time.Unix() + unixEraOffset
	era := s / eraLength
	timestamp := s - era*eraLength
	// TODO: Need to handle fraction
	return NTPDate{eraNumber: int32(era), eraOffset: uint32(timestamp)}
}

func NTPDateToTime(ntpDate NTPDate) time.Time {
	s := int64(ntpDate.eraNumber)*eraLength + int64(ntpDate.eraOffset)
	// TODO: Need to handle fraction
	time := time.Unix(s, 0)
	return time
}
