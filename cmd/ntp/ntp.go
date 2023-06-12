package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"time"
)

const PORT = 123           // NTP port number
const VERSION byte = 4     // NTP version number
const TOLERANCE = 15e-6    //frequency tolerance PHI (s/s)
const MINPOLL int8 = 4     //minimum poll exponent (16 s)
const MAXPOLL int8 = 17    // maximum poll exponent (36 h)
const MAXDISP float64 = 16 // maximum dispersion (16 s)
const MINDISP = 0.005      // minimum dispersion increment (s)
const NOSYNC byte = 0x3    // leap unsync
const MAXDIST byte = 1     // distance threshold (1 s)
const MAXSTRAT byte = 16   // maximum stratum number

const NTPShortLength float64 = 65536      // 2^16
const eraLength int64 = 4_294_967_296     // 2^32
const unixEraOffset int64 = 2_208_988_800 // 1970 - 1900 in seconds

const SGATE = 3     /* spike gate (clock filter */
const BDELAY = .004 /* broadcast delay (s) */
const PHI = 15e-6   /* % frequency tolerance (15 ppm) */
const NSTAGE = 8    /* clock register stages */
const NMAX = 50     /* maximum number of peers */
const NSANE = 1     /* % minimum intersection survivors */
const NMIN = 3      /* % minimum cluster survivors */

const UNREACH = 12 /* unreach counter threshold */
const BCOUNT = 8   /* packets in a burst */
const BTIME = 2    /* burst interval (s) */

const STEPT = .128      /* step threshold (s) */
const WATCH = 900       /* stepout threshold (s) */
const PANICT = 1000     /* panic threshold (s) */
const PLL = 65536       /* PLL loop gain */
const FLL = MAXPOLL + 1 /* FLL loop gain */
const AVG = 4           /* parameter averaging constant */
const ALLAN = 1500      /* compromise Allan intercept (s) */
const LIMIT = 30        /* poll-adjust threshold */
const MAXFREQ = 500e-6  /* frequency tolerance (500 ppm) */
const PGATE = 4         /* poll-adjust gate */

const MINCLOCK = 3  /* minimum manycast survivors */
const MAXCLOCK = 10 /* maximum manycast candidates */
const TTLMAX = 8    /* max ttl manycast */
const BEACON = 15   /* max interval between beacons */

type Mode byte

const (
	RESERVED Mode = iota
	SYMMETRIC_ACTIVE
	SYMMETRIC_PASSIVE
	CLIENT
	SERVER
	BROADCAST_SERVER
	BROADCAST_CLIENT // Also NTP_CONTROL_MESSAGE?
	RESERVED_PRIVATE_USE
)

type DispatchCode int

const ERR DispatchCode = -1
const (
	DSCRD DispatchCode = iota
	PROC
	BCST
	FXMIT
	MANY
	NEWPS
	NEWBC
)

type AssociationStateCode byte

const (
	INIT   AssociationStateCode = iota /* initialization */
	STALE                              /* timeout */
	STEP                               /* time step */
	ERROR                              /* authentication error */
	CRYPTO                             /* crypto-NAK received */
	NKEY                               /* untrusted key */
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

type NTPSystem struct {
	address      *net.UDPAddr
	t            NTPTimestampEncoded /* update time */
	leap         byte                /* leap indicator */
	stratum      byte                /* stratum */
	poll         int8                /* poll interval */
	precision    int8                /* precision */
	rootdelay    float64             /* root delay */
	rootdisp     float64             /* root dispersion */
	refid        byte                /* reference ID */
	reftime      NTPTimestampEncoded /* reference time */
	m            [NMAX]Chime         /* chime list */
	v            [NMAX]Survivor      /* survivor list */
	p            *Association        /* association ID */
	offset       float64             /* combined offset */
	jitter       float64             /* combined jitter */
	flags        int                 /* option flags */
	n            int                 /* number of survivors */
	associations []*Association
	clock        Clock
	conn         *net.UDPConn
}

type Association struct {
	leap  byte /* leap indicator */
	hmode Mode // HOST (Self) mode
	// Values set by received packet
	ReceivePacket

	/*
	 * Computed data
	 */
	t      float64             /* update time */
	f      [NSTAGE]FilterStage /* clock filter */
	offset float64             /* peer offset */
	delay  float64             /* peer delay */
	disp   float64             /* peer dispersion */
	jitter float64             /* RMS jitter */

	/*
	 * Poll process variables
	 */
	hpoll    int8
	reach    byte
	burst    int
	ttl      int
	unreach  int
	outdate  int32
	nextdate int32

	burstEnabled  bool
	iburstEnabled bool
	isMany        bool // manycast client association
	ephemeral     bool
}

type Chime struct { /* m is for Marzullo */
	association *Association /* peer structure pointer */
	levelType   int          /* high +1, mid 0, low -1 */
	edge        float64      /* correctness interval edge */
}

type Survivor struct {
	association *Association /* peer structure pointer */
	metric      float64
}

// "t" is process time, not realtime. Only second incrementing
type Clock struct {
	t      NTPTimestampEncoded
	state  int
	offset float64
	last   float64
	count  int
	freq   float64
	jitter float64
	wander float64
}

type FilterStage struct {
	t      NTPTimestampEncoded /* update time */
	offset float64             /* clock ofset */
	delay  float64             /* roundtrip delay */
	disp   float64             /* dispersion */
}

// Fields that can be read directly from the packet bytes
type EncodedReceivePacket struct {
	Stratum   byte                /* stratum */
	Poll      int8                /* poll interval */
	Precision int8                /* precision */
	Rootdelay NTPShortEncoded     /* root delay */
	Rootdisp  NTPShortEncoded     /* root dispersion */
	Refid     NTPShortEncoded     /* reference ID */
	Reftime   NTPTimestampEncoded /* reference time */
	Org       NTPTimestampEncoded /* origin timestamp */
	Rec       NTPTimestampEncoded /* receive timestamp */
	Xmt       NTPTimestampEncoded /* transmit timestamp */
}

type ReceivePacket struct {
	srcaddr *net.UDPAddr        /* source (remote) address */
	dstaddr *net.UDPAddr        /* destination (local) address */
	leap    byte                /* leap indicator */
	version byte                /* version number */
	mode    Mode                /* mode */
	keyid   int32               /* key ID */
	mac     Digest              /* message digest */
	dst     NTPTimestampEncoded /* destination timestamp */
	EncodedReceivePacket
}

type TransmitPacket struct {
	dstaddr *net.UDPAddr /* source (local) address */
	srcaddr *net.UDPAddr /* destination (remote) address */
	leap    byte         /* leap indicator */
	version byte         /* version number */
	mode    Mode         /* mode */
	keyid   int          /* key ID */
	dgst    Digest       /* message digest */
	EncodedReceivePacket
}

func (system *NTPSystem) CreateAssociations(associationConfigs []ServerAssociationConfig) []*Association {
	associations := []*Association{}

	for _, associationConfig := range associationConfigs {
		association := &Association{
			hmode: associationConfig.hmode,
			hpoll: int8(associationConfig.minpoll),
			ReceivePacket: ReceivePacket{
				srcaddr: associationConfig.address,
				dstaddr: system.address,
				version: byte(associationConfig.version),
				keyid:   int32(associationConfig.key),
			},
		}
		system.clear(association, INIT)

		association.burstEnabled = associationConfig.burst
		association.iburstEnabled = associationConfig.iburst

		associations = append(associations, association)
	}

	return associations
}

func (system *NTPSystem) SetupAsssociations(associations []*Association, wg *sync.WaitGroup) {
	system.associations = associations
	wg.Add(1)
	// go SetupPeer()
	go func() {
		for {
			system.clockAdjust()
			time.Sleep(time.Second)
		}
	}()
}

func (system *NTPSystem) clockAdjust() {
	fmt.Println("Clock adjust at:", time.Now())

	/*
	 * Update the process time c.t.  Also increase the dispersion
	 * since the last update.  In contrast to NTPv3, NTPv4 does not
	 * declare unsynchronized after one day, since the dispersion
	 * threshold serves this function.  When the dispersion exceeds
	 * MAXDIST (1 s), the server is considered unfit for
	 * synchronization.
	 */
	system.clock.t++
	system.rootdisp += PHI

	/*
	 * Implement the phase and frequency adjustments.  The gain
	 * factor (denominator) is not allowed to increase beyond the
	 * Allan intercept.  It doesn't make sense to average phase
	 * noise beyond this point and it helps to damp residual offset
	 * at the longer poll intervals.
	 */
	dtemp := system.clock.offset / (float64(PLL) * math.Min(Log2ToDouble(system.poll), float64(ALLAN)))
	system.clock.offset -= dtemp

	/*
	 * This is the kernel adjust time function, usually implemented
	 * by the Unix adjtime() system call.
	 */
	adjustTime(system.clock.freq + dtemp)

	/*
	 * Peer timer.  Call the poll() routine when the poll timer
	 * expires.
	 */
	for _, association := range system.associations {
		if system.clock.t >= uint64(association.nextdate) {
			fmt.Println("Polling:", association.srcaddr.IP)
			system.sendPoll(association)
		}
	}

	/*
		TODO
		 * Once per hour, write the clock frequency to a file.
	*/
	/*
	 * if (c.t % 3600 == 3599)
	 *   write c.freq to file
	 */
}

func (system *NTPSystem) sendPoll(association *Association) {
	/*
	 * This routine is called when the current time c.t catches up
	 * to the next poll time p->nextdate.  The value p->outdate is
	 * the last time this routine was executed.  The poll_update()
	 * routine determines the next execution time p->nextdate.
	 *
	 * If broadcasting, just do it, but only if we are synchronized.
	 */
	hpoll := association.hpoll
	if association.hmode == BROADCAST_SERVER {
		association.outdate = int32(system.clock.t)
		if system.p != nil {
			system.pollPeer(association)
		}
		system.pollUpdate(association, hpoll)
		return
	}

	/*
	 * If manycasting, start with ttl = 1.  The ttl is increased by
	 * one for each poll until MAXCLOCK servers have been found or
	 * ttl reaches TTLMAX.  If reaching MAXCLOCK, stop polling until
	 * the number of servers falls below MINCLOCK, then start all
	 * over.
	 */
	if association.hmode == CLIENT && association.isMany {
		association.outdate = int32(system.clock.t)
		if association.unreach > BEACON {
			association.unreach = 0
			association.ttl = 1
			system.pollPeer(association)
		} else if system.n < MINCLOCK {
			if association.ttl < TTLMAX {
				association.ttl++
			}
			system.pollPeer(association)
		}

		association.unreach++
		system.pollUpdate(association, hpoll)
		return
	}
	if association.burst == 0 {

		/*
		 * We are not in a burst.  Shift the reachability
		 * register to the left.  Hopefully, some time before
		 * the next poll a packet will arrive and set the
		 * rightmost bit.
		 */
		// TODO: Not sure what oreach is for
		// oreach := association.reach
		association.outdate = int32(system.clock.t)
		association.reach = association.reach << 1

		// & with 0x7 to check if the last 3 attempts were unsuccessful
		if association.reach&0x7 == 0 {
			system.clockFilter(association, 0, 0, MAXDISP)
		}

		// Unreachable
		if association.reach == 0 {
			/*
			 * The server is unreachable, so bump the
			 * unreach counter.  If the unreach threshold
			 * has been reached, double the poll interval
			 * to minimize wasted network traffic.  Send a
			 * burst only if enabled and the unreach
			 * threshold has not been reached.
			 */
			if association.iburstEnabled && association.unreach == 0 {
				association.burst = BCOUNT
			} else if association.unreach < UNREACH {
				association.unreach++
			} else {
				hpoll++
			}
			association.unreach++
		} else {
			/*
			 * The server is reachable.  Set the poll
			 * interval to the system poll interval.  Send a
			 * burst only if enabled and the peer is fit.
			 */
			association.unreach = 0
			hpoll = system.poll
			if association.burstEnabled && system.fit(association) {
				association.burst = BCOUNT
			}
		}
	} else {
		/*
		 * If in a burst, count it down.  When the reply comes
		 * back the clock_filter() routine will call
		 * clock_select() to process the results of the burst.
		 */
		association.burst--
	}
	/*
	 * Do not transmit if in broadcast client mode.
	 */
	if association.hmode != BROADCAST_CLIENT {
		system.pollPeer(association)
	}
	system.pollUpdate(association, hpoll)
}

func DecodeRecvPacket(encoded []byte, clientAddr net.Addr, con net.PacketConn) (*ReceivePacket, error) {
	clientAddrPort, err := netip.ParseAddrPort(clientAddr.String())
	if err != nil {
		return nil, err
	}
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
		srcaddr:              clientUDPAddr,
		dstaddr:              localUDPAddr,
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
	fmt.Println("Received packet from:", packet.srcaddr.IP)

	if packet.version > VERSION {
		return nil
	}

	var association *Association
	for _, possibleAssociation := range system.associations {
		if packet.srcaddr.IP.Equal(possibleAssociation.srcaddr.IP) {
			association = possibleAssociation
			break
		}
	}

	hmode := RESERVED
	if association != nil {
		hmode = association.hmode
	}

	switch dispatchTable[hmode][packet.mode-1] {
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

	if packet.Xmt == 0 {
		return nil
	}

	if packet.Xmt == association.Xmt {
		return nil
	}

	unsynch := packet.mode != BROADCAST_SERVER && (packet.Org == 0 || packet.Org != association.Xmt)

	association.Org = packet.Xmt
	association.Rec = packet.dst

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
	if packet.Stratum == 0 {
		association.Stratum = MAXSTRAT
	} else {
		association.Stratum = packet.Stratum
	}
	association.mode = packet.mode
	association.Poll = packet.Poll
	association.Rootdelay = uint32(float64(packet.Rootdelay) / NTPShortLength)
	association.Rootdisp = uint32(float64(packet.Rootdisp) / NTPShortLength)
	association.Refid = packet.Refid
	association.Reftime = packet.Reftime

	/*
	 * Verify the server is synchronized with valid stratum and
	 * reference time not later than the transmit time.
	 */
	if association.leap == NOSYNC || association.Stratum >= MAXSTRAT {
		return /* unsynchronized */
	}

	/*
	 * Verify valid root distance.
	 */
	if association.Rootdelay/2+association.Rootdisp >= uint32(MAXDISP) || association.Reftime >
		packet.Xmt {

		return /* invalid header values */
	}

	system.pollUpdate(association, association.hpoll)
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
		offset = NTPTimestampEncodedToDouble(packet.Xmt - packet.dst)
		delay = BDELAY
		disp = Log2ToDouble(packet.Precision) + Log2ToDouble(system.precision) + PHI*
			2*BDELAY
	} else {
		offset = (NTPTimestampEncodedToDouble(packet.Rec-packet.Org) + NTPTimestampEncodedToDouble(packet.Xmt-
			packet.dst)) / 2
		delay = math.Max(NTPTimestampEncodedToDouble(packet.dst-packet.Org)-NTPTimestampEncodedToDouble(packet.Xmt-
			packet.Rec), Log2ToDouble(system.precision))
		disp = Log2ToDouble(packet.Precision) + Log2ToDouble(system.precision) + PHI*
			NTPTimestampEncodedToDouble(packet.dst-packet.Org)
	}

	system.clockFilter(association, offset, delay, disp)
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
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Poll = receivePacket.Poll
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = NTPShortEncoded(system.rootdelay * NTPShortLength)
	transmitPacket.Rootdisp = NTPShortEncoded(system.rootdisp * NTPShortLength)
	transmitPacket.Refid = uint32(system.refid)
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = receivePacket.Xmt
	transmitPacket.Rec = receivePacket.dst
	transmitPacket.Xmt = GetSystemTime()

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

func (system *NTPSystem) pollPeer(association *Association) {
	var transmitPacket TransmitPacket

	/*
	 * Initialize header and transmit timestamp
	 */
	transmitPacket.srcaddr = association.dstaddr
	transmitPacket.dstaddr = association.srcaddr
	transmitPacket.leap = system.leap
	transmitPacket.version = association.version
	transmitPacket.mode = association.hmode
	if system.stratum == MAXSTRAT {
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Poll = association.hpoll
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = NTPShortEncoded(system.rootdelay * NTPShortLength)
	transmitPacket.Rootdisp = NTPShortEncoded(system.rootdisp * NTPShortLength)
	transmitPacket.Refid = uint32(system.refid)
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = association.Org
	transmitPacket.Rec = association.Rec
	transmitPacket.Xmt = GetSystemTime()
	association.Xmt = transmitPacket.Xmt

	/*
	 * If the key ID is nonzero, send a valid MAC using the key ID
	 * of the association and the key in the local key cache.  If
	 * something breaks, like a missing trusted key, don't send the
	 * packet; just reset the association and stop until the problem
	 * is fixed.
	 */
	if association.keyid != 0 {
		// if (/* p->keyid invalid */ 0) {
		//         clear(p, X_NKEY);
		//         return;
		// }
		// x.dgst = md5(p->keyid);
	}

	go func() {
		var encoded bytes.Buffer
		writer := bufio.NewWriter(&encoded)

		var firstByte byte
		firstByte = transmitPacket.leap << 6
		firstByte |= transmitPacket.version << 3
		firstByte |= byte(transmitPacket.mode)

		writer.WriteByte(firstByte)
		if err := binary.Write(writer, binary.BigEndian, transmitPacket.EncodedReceivePacket); err != nil {
			panic("encoded transmit packet err")
		}

		writer.Flush()

		written, err := system.conn.WriteTo(encoded.Bytes(), transmitPacket.dstaddr)
		if err != nil {
			fmt.Println("Error", err)
		}
		fmt.Println("written", written, len(encoded.Bytes()), encoded.Bytes())
	}()
}

func (system *NTPSystem) pollUpdate(association *Association, poll int8) {
	association.hpoll = int8(math.Max(math.Min(float64(MAXPOLL), float64(poll)), float64(MINPOLL)))
	if association.burst > 0 {
		if uint64(association.nextdate) != system.clock.t {
			return
		} else {
			association.nextdate += BTIME
		}
	} else {
		association.nextdate = association.outdate + (1 << int32(math.Max(math.Min(float64(association.Poll),
			float64(association.hpoll)), float64(MINPOLL))))
	}

	if uint64(association.nextdate) <= system.clock.t {
		association.nextdate = int32(system.clock.t + 1)
	}
}

func (system *NTPSystem) clear(association *Association, kiss AssociationStateCode) {
	/*
	 * The first thing to do is return all resources to the bank.
	 * Typical resources are not detailed here, but they include
	 * dynamically allocated structures for keys, certificates, etc.
	 * If an ephemeral association and not initialization, return
	 * the association memory as well.
	 */
	/* return resources */
	if system.p == association {
		system.p = nil
	}

	if kiss != INIT && association.ephemeral {
		return
	}

	/*
	 * Initialize the association fields for general reset.
	 */
	association.Org = 0
	association.Rec = 0
	association.Xmt = 0
	association.t = 0
	association.f = [NSTAGE]FilterStage{}
	association.offset = 0
	association.delay = 0
	association.disp = 0
	association.jitter = 0
	association.hpoll = 0
	association.burst = 0
	association.reach = 0
	association.ttl = 0

	association.leap = NOSYNC
	association.Stratum = MAXSTRAT
	association.Poll = MAXPOLL
	association.hpoll = MINPOLL
	association.disp = MAXDISP
	association.jitter = Log2ToDouble(system.precision)
	association.Refid = uint32(kiss)
	for i := 0; i < NSTAGE; i++ {
		association.f[i].disp = MAXDISP
	}

	/*
	 * Randomize the first poll just in case thousands of broadcast
	 * clients have just been stirred up after a long absence of the
	 * broadcast server.
	 */
	association.t = float64(system.clock.t)
	association.outdate = int32(association.t)
	association.nextdate = association.outdate + rand.Int31n(1<<MINPOLL)
}

func (system *NTPSystem) clockFilter(association *Association, offset float64, delay float64, disp float64) {
	var f [NSTAGE]FilterStage
	var dtemp float64

	/*
	 * The clock filter contents consist of eight tuples (offset,
	 * delay, dispersion, time).  Shift each tuple to the left,
	 * discarding the leftmost one.  As each tuple is shifted,
	 * increase the dispersion since the last filter update.  At the
	 * same time, copy each tuple to a temporary list.  After this,
	 * place the (offset, delay, disp, time) in the vacated
	 * rightmost tuple.
	 */
	for i := 1; i < NSTAGE; i++ {
		association.f[i] = association.f[i-1]
		association.f[i].disp += PHI * (float64(system.clock.t) - association.t)
		f[i] = association.f[i]
	}
	association.f[0].t = system.clock.t
	association.f[0].offset = offset
	association.f[0].delay = delay
	association.f[0].disp = disp
	f[0] = association.f[0]

	/*
	 * Sort the temporary list of tuples by increasing f[].delay.
	 * The first entry on the sorted list represents the best
	 * sample, but it might be old.
	 */
	dtemp = association.offset
	association.offset = f[0].offset
	association.delay = f[0].delay
	for i := 0; i < NSTAGE; i++ {
		association.disp += f[i].disp / (math.Pow(2, float64(i+1)))
		association.jitter += math.Pow((f[i].offset - f[0].offset), 2)
	}
	association.jitter = math.Max(math.Sqrt(association.jitter), Log2ToDouble(system.precision))

	/*
	 * Prime directive: use a sample only once and never a sample
	 * older than the latest one, but anything goes before first
	 * synchronized.
	 */
	if float64(f[0].t)-association.t <= 0 && system.leap != NOSYNC {
		return
	}

	/*
	 * Popcorn spike suppressor.  Compare the difference between the
	 * last and current offsets to the current jitter.  If greater
	 * than SGATE (3) and if the interval since the last offset is
	 * less than twice the system poll interval, dump the spike.
	 * Otherwise, and if not in a burst, shake out the truechimers.
	 */
	if math.Abs(association.offset-dtemp) > SGATE*association.jitter && (float64(f[0].t)-
		association.t) < float64(2*system.poll) {

		return
	}

	association.t = float64(f[0].t)
	if association.burst == 0 {
		// TODO clock_select
		// clock_select()
	}
}

/*
 * fit() - test if association p is acceptable for synchronization
 */
func (system *NTPSystem) fit(association *Association) bool {
	/*
	 * A stratum error occurs if (1) the server has never been
	 * synchronized, (2) the server stratum is invalid.
	 */
	if association.leap == NOSYNC || association.Stratum >= MAXSTRAT {
		return false
	}

	/*
	 * A distance error occurs if the root distance exceeds the
	 * distance threshold plus an increment equal to one poll
	 * interval.
	 */
	if system.rootDist(association) > float64(MAXDIST)+PHI*Log2ToDouble(system.poll) {
		return false
	}

	/*
	 * A loop error occurs if the remote peer is synchronized to the
	 * local peer or the remote peer is synchronized to the current
	 * system peer.  Note this is the behavior for IPv4; for IPv6
	 * the MD5 hash is used instead.
	 */
	if association.Refid == binary.BigEndian.Uint32(association.dstaddr.IP) || association.Refid == uint32(system.refid) {
		return false
	}

	/*
	 * An unreachable error occurs if the server is unreachable.
	 */
	if association.reach == 0 {

		return false
	}

	return true
}

func (system *NTPSystem) rootDist(association *Association) float64 {
	/*
	 * The root synchronization distance is the maximum error due to
	 * all causes of the local clock relative to the primary server.
	 * It is defined as half the total delay plus total dispersion
	 * plus peer jitter.
	 */
	return (math.Max(MINDISP, float64(association.Rootdelay)+association.delay)/2 +
		float64(association.Rootdisp) + association.disp + PHI*float64(float64(system.clock.t)-association.t) + association.jitter)
}

func GetSystemTime() NTPTimestampEncoded {
	var unixTime syscall.Timeval
	syscall.Gettimeofday(&unixTime)
	return (UnixToNTPTimestampEncoded(unixTime))
}

func stepTime(offset float64) {
	var unixTime syscall.Timeval
	syscall.Gettimeofday(&unixTime)

	ntpTime := DoubleToNTPTimestampEncoded(offset) + UnixToNTPTimestampEncoded(unixTime)
	unixTime.Sec = int64(ntpTime >> 32)
	unixTime.Usec = int32(int64((ntpTime-uint64(unixTime.Sec))<<
		32) / eraLength * 1e6)

	if os.Getenv("ENABLED") == "1" {
		syscall.Settimeofday(&unixTime)
	}
}

func adjustTime(offset float64) {
	var unixTime syscall.Timeval

	ntpTime := DoubleToNTPTimestampEncoded(offset)
	unixTime.Sec = int64(ntpTime >> 32)
	unixTime.Usec = int32(int64((ntpTime-uint64(unixTime.Sec))<<
		32) / eraLength * 1e6)

	if os.Getenv("ENABLED") == "1" {
		syscall.Adjtime(&unixTime, nil)
	} else {
		var now syscall.Timeval
		syscall.Gettimeofday(&now)
		fmt.Println("ADJTIME:", time.Unix(now.Sec, int64(now.Usec)*1000).Add(time.Duration(unixTime.Sec)*time.Second).Add(time.Duration(unixTime.Usec)*time.Microsecond))
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

func NTPTimestampToTime(ntpTimestamp NTPTimestampEncoded) time.Time {
	now := NTPTimestampEncodedToDouble(ntpTimestamp)
	return time.Unix(int64(now)-unixEraOffset, int64(now*1e6)%1e6)
}
