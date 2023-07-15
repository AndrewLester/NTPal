package ntpal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/AndrewLester/ntpal/internal/ntp"
	"github.com/AndrewLester/ntpal/internal/rpc"
)

const VERSION byte = 4     // NTP version number
const TOLERANCE = 15e-6    //frequency tolerance PHI (s/s)
const MINPOLL int8 = 3     //minimum poll exponent (64 s)
const MAXPOLL int8 = 17    // maximum poll exponent (36 h)
const MAXDISP float64 = 16 // maximum dispersion (16 s)
const MINDISP = 0.005      // minimum dispersion increment (s)
const NOSYNC byte = 0x3    // leap unsync
const MAXDIST byte = 1     // distance threshold (1 s)
const MAXSTRAT byte = 16   // maximum stratum number

const NTPShortLength float64 = 65536 // 2^16

const SGATE = 3     /* spike gate (clock Filter */
const BDELAY = .004 /* broadcast delay (s) */
const PHI = 15e-6   /* % frequency tolerance (15 ppm) */
const NSTAGE = 8    /* clock Register stages */
const NMAX = 50     /* maximum number of peers */
const NSANE = 1     /* % minimum intersection survivors */
const NMIN = 3      /* % minimum cluster survivors */
const NTP_FWEIGHT = 0.5
const UNREACH = 8 /* unreach counter threshold */
const BCOUNT = 8  /* packets in a burst */
const BTIME = 2   /* burst interval (s) */

const STEPT = .128     /* step threshold (s) */
const WATCH = 600      /* stepout threshold (s) */
const PANICT = 1000    /* panic threshold (s) */
const PLL = 16         /* PLL loop gain */
const FLL = 0.25       /* FLL loop gain */
const AVG = 4          /* parameter averaging constant */
const ALLAN = 2048     /* compromise Allan intercept (s) */
const LIMIT = 30       /* poll-adjust threshold */
const MAXFREQ = 500e-6 /* frequency tolerance (500 ppm) */
const PGATE = 4        /* poll-adjust gate */

const MINCLOCK = 3  /* minimum manycast survivors */
const MAXCLOCK = 10 /* maximum manycast candidates */
const TTLMAX = 8    /* max ttl manycast */
const BEACON = 15   /* max interval between beacons */

const MTU = 1300
const PRECISION = -18 /* precision (log2 s)  */

const STARTUP_OFFSET_MAX = 5e-4

const (
	NSET int = iota /* clock Never set */
	FSET            /* frequency set from file */
	SPIK            /* spike detected */
	FREQ            /* frequency mode */
	SYNC            /* clock Synchronized */
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

type LocalClockReturnCode int

const (
	IGNORE LocalClockReturnCode = iota /* ignore */
	SLEW                               /* slew adjustment */
	LSTEP                              /* step adjustment  TODO: RENAME THIS BACK TO STEP */
	PANIC                              /* panic - no adjustment */
)

// Index with [associationMode][packetMOde]
var dispatchTable = [][]DispatchCode{
	{NEWPS, DSCRD, FXMIT, MANY, NEWBC},
	{PROC, PROC, DSCRD, DSCRD, DSCRD},
	{PROC, ERR, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, PROC, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, DSCRD},
	{DSCRD, DSCRD, DSCRD, DSCRD, PROC},
}

type NTPalSystem struct {
	Clock Clock

	address   *net.UDPAddr
	t         ntp.TimestampEncoded /* update time */
	leap      byte                 /* leap indicator */
	stratum   byte                 /* stratum */
	poll      int8                 /* poll interval */
	precision int8                 /* precision */
	rootdelay float64              /* root delay */
	rootdisp  float64              /* root dispersion */
	refid     ntp.ShortEncoded     /* reference ID */
	reftime   ntp.TimestampEncoded /* reference time */
	// Max of the two below is NMAX, but using slice type becaues it's not always full, and nils cant be sorted
	m            []Chime      /* chime list */
	v            []Survivor   /* survivor list */
	association  *Association /* association ID */
	offset       float64      /* combined offset */
	jitter       float64      /* combined jitter */
	flags        int          /* option flags */
	n            int          /* number of survivors */
	associations []*Association
	conn         *net.UDPConn

	host string
	port string

	mode ntp.Mode

	drift  string
	config string

	lock sync.Mutex
	wg   sync.WaitGroup

	hold int64

	// QUERY

	query            bool
	filtered         chan any
	FilteredProgress chan any

	// RPC
	socket string
}

type Association struct {
	hmode ntp.Mode // HOST (Self) mode

	ntp.Association

	/*
	 * Computed data
	 */
	t float64             /* clock.T of last used sample */
	f [NSTAGE]FilterStage /* clock Filter */

	/*
	 * Poll process variables
	 */
	burst    int
	ttl      int
	unreach  int
	outdate  int32
	nextdate int32
	maxpoll  int8
	minpoll  int8

	isMany    bool // manycast client association
	ephemeral bool
}

type Chime struct { /* m is for Marzullo */
	association *Association /* peer structure pointer */
	levelType   int          /* high +1, mid 0, low -1 */
	edge        float64      /* correctness interval edge */
}

type Chimers []Chime

func (s Chimers) Len() int      { return len(s) }
func (s Chimers) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByEdge struct {
	Chimers
}

func (s ByEdge) Less(i, j int) bool {
	return s.Chimers[i].edge < s.Chimers[j].edge
}

type Survivor struct {
	association *Association /* peer structure pointer */
	metric      float64
}

type Survivors []Survivor

func (s Survivors) Len() int      { return len(s) }
func (s Survivors) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByMetric struct {
	Survivors
}

func (s ByMetric) Less(i, j int) bool {
	return s.Survivors[i].metric < s.Survivors[j].metric
}

// "t" is process time, not realtime. Only second incrementing
type Clock struct {
	lock sync.Mutex
	ntp.Clock
}

type FilterStage struct {
	t      ntp.TimestampEncoded /* update time */
	offset float64              /* clock Ofset */
	delay  float64              /* roundtrip delay */
	disp   float64              /* dispersion */
}

type FilterStages [NSTAGE]FilterStage

func (s *FilterStages) Len() int      { return len(s) }
func (s *FilterStages) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByDelay struct {
	*FilterStages
}

func (s ByDelay) Less(i, j int) bool {
	return s.FilterStages[i].delay < s.FilterStages[j].delay
}

func NewSystem(host, port, config, drift, socket string) *NTPalSystem {
	return &NTPalSystem{
		host:             host,
		port:             port,
		config:           config,
		socket:           socket,
		drift:            drift,
		mode:             ntp.SERVER,
		leap:             NOSYNC,
		poll:             MINPOLL,
		precision:        PRECISION,
		hold:             WATCH,
		filtered:         make(chan any),
		FilteredProgress: make(chan any),
	}
}

func (system *NTPalSystem) Start() {
	config := parseConfig(system.config)

	if system.drift == "" {
		system.drift = config.driftfile
	}

	freq := readDriftInfo(system)
	if freq != 0 {
		system.Clock.Freq = freq
		system.rstclock(FSET, 0, 0)
	} else {
		system.rstclock(NSET, 0, 0)
	}

	system.Clock.Jitter = ntp.Log2ToDouble(system.precision)

	rand.Seed(time.Now().UnixNano())

	address, err := net.ResolveUDPAddr("udp", net.JoinHostPort(system.host, system.port))
	if err != nil {
		log.Fatal("Could not resolve NTP_HOST + NTP_PORT")
	}
	system.address = address

	system.setupAssociations(config.servers)

	if system.mode == ntp.SERVER {
		system.listen()

		system.wg.Add(1)
		go system.setupServer()

		rpcServer := &rpc.NTPalRPCServer{Socket: system.socket, System: &ntp.System{Clock: &system.Clock.Clock}, GetAssociations: system.GetAssociations}

		system.wg.Add(1)
		go rpcServer.Listen(&system.wg)
	}

	system.wg.Wait()
}

func (system *NTPalSystem) setupAssociations(associationConfigs []serverAssociationConfig) {
	for _, associationConfig := range associationConfigs {
		association := &Association{
			hmode:   associationConfig.hmode,
			maxpoll: int8(associationConfig.maxpoll),
			minpoll: int8(associationConfig.minpoll),
			Association: ntp.Association{
				Hpoll:    int8(associationConfig.minpoll),
				Hostname: associationConfig.hostname,
				ReceivePacket: ntp.ReceivePacket{
					Srcaddr: associationConfig.address,
					Dstaddr: system.address,
					Version: byte(associationConfig.version),
					Keyid:   int32(associationConfig.key),
				},
			},
		}
		system.clear(association, INIT)

		association.BurstEnabled = associationConfig.burst
		association.IburstEnabled = associationConfig.iburst

		system.associations = append(system.associations, association)
	}

	system.wg.Add(1)

	go func() {
		for {
			system.clockAdjust()
			time.Sleep(time.Second)
		}
	}()
}

func (system *NTPalSystem) listen() {
	udp, err := net.ListenUDP("udp", system.address)
	if err != nil {
		log.Fatalf("can't listen on %v/udp: %s", system.address, err)
	}

	system.conn = udp
}

func (system *NTPalSystem) setupServer() {
	defer system.wg.Done()

	packet := make([]byte, MTU)

	for {
		_, addr, err := system.conn.ReadFrom(packet)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			log.Printf("error reading on %s/udp: %s", addr, err)
			continue
		}

		recvPacket, err := ntp.DecodeRecvPacket(packet, addr, system.conn)
		if err != nil {
			log.Printf("Error reading packet: %v", err)
		}
		recvPacket.Dst = GetSystemTime()
		reply := system.receive(*recvPacket)
		if reply == nil {
			continue
		}
		encoded := ntp.EncodeTransmitPacket(*reply)
		n, e := system.conn.WriteTo(encoded, addr)
		info("(Reply) Bytes written:", n, "Err:", e)
	}

}

func (system *NTPalSystem) clockAdjust() {
	/*
	 * Update the process time c.t.  Also increase the dispersion
	 * since the last update.  In contrast to NTPv3, NTPv4 does not
	 * declare unsynchronized after one day, since the dispersion
	 * threshold serves this function.  When the dispersion exceeds
	 * MAXDIST (1 s), the server is considered unfit for
	 * synchronization.
	 */
	system.Clock.lock.Lock()

	system.Clock.T++
	system.rootdisp += PHI

	/*
	 * Implement the phase and frequency adjustments.  The gain
	 * factor (denominator) is not allowed to increase beyond the
	 * Allan intercept.  It doesn't make sense to average phase
	 * noise beyond this point and it helps to damp residual offset
	 * at the longer poll intervals.
	 */
	dtemp := system.Clock.Offset / (float64(PLL) * ntp.Log2ToDouble(system.poll))
	if system.Clock.State != SYNC {
		dtemp = 0
	} else if system.hold > 0 {
		dtemp = system.Clock.Offset / (float64(PLL) * ntp.Log2ToDouble(1))
		system.hold--
	}

	/*
	* This is the kernel adjust time function, usually implemented
	* by the Unix adjtime() system call.
	 */
	//  TODO: Might need to wrap this in system.hold == 0 ??
	system.Clock.Offset -= dtemp
	debug("*****ADJUSTING:")
	debug("TIME:", system.Clock.T, "SYS OFFSET:", system.offset, "CLOCK OFFSET:", system.Clock.Offset)
	debug("FREQ: ", system.Clock.Freq*1e6, "OFFSET (dtemp):", dtemp*1e6)
	adjustTime(system.Clock.Freq + dtemp)

	system.Clock.lock.Unlock()

	/*
	 * Peer timer.  Call the poll() routine when the poll timer
	 * expires.
	 */
	for _, association := range system.associations {
		if system.Clock.T >= ntp.TimestampEncoded(association.nextdate) {
			info("sendPoll:", association.Srcaddr.IP)
			system.sendPoll(association)
		}
	}

	// Once per hour, write the clock Frequency to a file.
	if system.Clock.T%3600 == 3599 {
		writeDriftInfo(system)
	}

	if system.Clock.T%10 == 0 {
		sysPeerIP := "NONE"
		if system.association != nil {
			sysPeerIP = system.association.Srcaddr.IP.String()
		}
		info("*****REPORT:")
		info(
			"(SYSTEM):",
			"T:", system.t,
			"OFFSET:", system.offset,
			"JITTER:", system.jitter,
			"PEER:", sysPeerIP,
			"POLL:", system.poll,
			"HOLD:", system.hold,
		)
		info(
			"(CLOCK):",
			"T:", system.Clock.T,
			"STATE:", system.Clock.State,
			"FREQ:", system.Clock.Freq,
			"OFFSET:", system.Clock.Offset,
			"JITTER:", system.Clock.Jitter,
			"WANDER:", system.Clock.Wander,
			"COUNT:", system.Clock.Count,
		)
		for _, association := range system.associations {
			ip := refIDToIP(association.Refid)
			refid := ip.String()
			if association.Stratum < 2 {
				refid = string(ip)
			}
			sync := "SYNC"
			if association.Leap == NOSYNC {
				sync = "NOSYNC"
			}
			info(
				"("+association.Srcaddr.IP.String()+"):",
				sync,
				"POLL:", strconv.Itoa(int(ntp.Log2ToDouble(association.Poll)))+"s",
				"hPOLL:", strconv.Itoa(int(ntp.Log2ToDouble(association.Poll)))+"s",
				"STRATUM:", association.Stratum,
				"REFID:", refid,
				"OFFSET:", association.Offset,
				"REACH:", strconv.FormatInt(int64(association.Reach), 2),
				"UNREACH:", association.unreach,
				"TIME FILTERED:", association.t,
			)
		}
	}
}

func (system *NTPalSystem) sendPoll(association *Association) {
	/*
	 * This routine is called when the current time c.t catches up
	 * to the next poll time p->nextdate.  The value p->outdate is
	 * the last time this routine was executed.  The poll_update()
	 * routine determines the next execution time p->nextdate.
	 *
	 * If broadcasting, just do it, but only if we are synchronized.
	 */
	hpoll := association.Hpoll
	if association.hmode == ntp.BROADCAST_SERVER {
		association.outdate = int32(system.Clock.T)
		if system.association != nil {
			system.pollPeer(association)
		}
		system.pollUpdate(association, hpoll)
		return
	}

	/*
	 * If manycasting, start with ttl = 1.  The ttl is increased by
	 * one for each poll until MAXCLOCK Servers have been found or
	 * ttl reaches TTLMAX.  If reaching MAXCLOCK, stop polling until
	 * the number of servers falls below MINCLOCK, then start all
	 * over.
	 */
	if association.hmode == ntp.CLIENT && association.isMany {
		association.outdate = int32(system.Clock.T)
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
		 * the next poll a packet Will arrive and set the
		 * rightmost bit.
		 */

		association.outdate = int32(system.Clock.T)
		association.Reach = association.Reach << 1

		// Unreachable
		if association.Reach == 0 {
			system.clockFilter(association, 0, 0, MAXDISP)
			/*
			 * The server is unreachable, so bump the
			 * unreach counter.  If the unreach threshold
			 * has been reached, double the poll interval
			 * to minimize wasted network traffic.  Send a
			 * burst only if enabled and the unreach
			 * threshold has not been reached.
			 */
			if association.IburstEnabled && association.unreach == 0 {
				association.burst = BCOUNT
			}

			if association.unreach < UNREACH {
				association.unreach++
			} else {
				// Try to get a new IP
				addr, err := net.ResolveUDPAddr("udp", association.Hostname+":123")
				if err != nil {
					log.Fatal("Invalid address: ", association.Hostname)
				}
				if association.Srcaddr == addr {
					hpoll++
				}
				association.Srcaddr = addr
				association.unreach = 0
			}
		} else {
			/*
			 * The server is reachable.  Set the poll
			 * interval to the system poll interval.  Send a
			 * burst only if enabled and the peer is fit.
			 */
			association.unreach = 0
			hpoll = system.poll
			if association.BurstEnabled && system.fit(association) {
				association.burst = BCOUNT
			}
		}
	} else {
		/*
		 * If in a burst, count it down.  When the reply comes
		 * back the clock_Filter() routine will call
		 * clock_Select() to process the results of the burst.
		 */
		association.burst--
	}
	/*
	 * Do not transmit if in broadcast client mode.
	 */
	if association.hmode != ntp.BROADCAST_CLIENT {
		system.pollPeer(association)
	}
	system.pollUpdate(association, hpoll)
}

func (system *NTPalSystem) receive(packet ntp.ReceivePacket) *ntp.TransmitPacket {
	if packet.Version > VERSION {
		return nil
	}

	var association *Association
	for _, possibleAssociation := range system.associations {
		if packet.Srcaddr.IP.Equal(possibleAssociation.Srcaddr.IP) {
			association = possibleAssociation
			break
		}
	}

	hmode := ntp.RESERVED
	if association != nil {
		hmode = association.hmode
	}

	if packet.Mode > 5 {
		info("ERROR: Received packet.Mode > 5 for association with addr:", packet.Srcaddr.IP)
		info("Packet", packet)
		return nil
	}

	switch dispatchTable[hmode][packet.Mode-1] {
	case FXMIT:
		info("Received request to sync from:", packet.Srcaddr.IP)
		// If the destination address is not a broadcast
		//    address

		/* not multicast dstaddr */

		// ignore auth
		// if (AUTH(p->flags & P_NOTRUST, auth))
		return system.reply(packet, ntp.SERVER)
		// else if (auth == A_ERROR)
		// 		fast_xmit(r, M_SERV, A_CRYPTO);
		// return;         /* M_SERV packet Sent */
	case NEWPS:
		if !isSymmetricEnabled() {
			return nil
		}

		association = &Association{
			Association: ntp.Association{ReceivePacket: packet},
			hmode:       ntp.SYMMETRIC_PASSIVE,
			ephemeral:   true,
		}
		system.clear(association, INIT)
		system.associations = append(system.associations, association)
	case PROC:
	default:
		return nil
	}

	info("****Processing packet From:", packet.Srcaddr.IP)

	if association == nil {
		info("Association shouldn't be nil here...")
		return nil
	}

	if packet.Xmt == 0 {
		return nil
	}

	if packet.Xmt == association.Xmt {
		return nil
	}

	unsynch := packet.Mode != ntp.BROADCAST_SERVER && (packet.Org == 0 || packet.Org != association.Xmt)

	association.Org = packet.Xmt
	association.Rec = packet.Dst

	if unsynch {
		return nil
	}

	system.process(association, packet)
	return nil
}

func (system *NTPalSystem) process(association *Association, packet ntp.ReceivePacket) {
	var offset float64 /* sample offsset */
	var delay float64  /* sample delay */
	var disp float64   /* sample dispersion */

	kod := false

	association.Leap = packet.Leap
	association.Poll = packet.Poll
	if packet.Stratum == 0 {
		association.Stratum = MAXSTRAT

		// Process KoD code
		kod = true

		code := string(refIDToIP(packet.Refid).To4())

		switch code {
		case "DENY", "RSTR":
			associationIdx := 0
			for idx, assoc := range system.associations {
				if assoc == association {
					associationIdx = idx
					break
				}
			}
			removeIndex(&system.associations, associationIdx)
			return
		case "RATE":
			association.Poll++
			system.pollUpdate(association, association.Poll)
		default:
			kod = false
		}
	} else {
		association.Stratum = packet.Stratum
	}
	association.Mode = packet.Mode
	association.Rootdelay = float64(packet.Rootdelay) / NTPShortLength
	association.Rootdisp = float64(packet.Rootdisp) / NTPShortLength
	association.Refid = packet.Refid
	association.Reftime = packet.Reftime

	// Server must be synchronized with valid stratum
	if association.Leap == NOSYNC || association.Stratum >= MAXSTRAT {
		system.filtered <- 0
		return
	}

	if association.Rootdelay/2+association.Rootdisp >= MAXDISP || association.Reftime >
		packet.Xmt {

		return /* invalid header values */
	}

	system.pollUpdate(association, association.Hpoll)
	association.Reach |= 1

	/*
	 * Calculate offset, delay and dispersion, then pass to the
	 * clock Filter.  Note carefully the implied processing.  The
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
	if association.Mode == ntp.BROADCAST_SERVER {
		offset = ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt - packet.Dst))
		delay = BDELAY
		disp = ntp.Log2ToDouble(packet.Precision) + ntp.Log2ToDouble(system.precision) + PHI*
			2*BDELAY
	} else {
		offset = (ntp.NTPTimestampDifferenceToDouble(int64(packet.Rec-packet.Org)) + ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt-
			packet.Dst))) / 2
		delay = math.Max(ntp.NTPTimestampDifferenceToDouble(int64(packet.Dst-packet.Org))-ntp.NTPTimestampDifferenceToDouble(int64(packet.Xmt-
			packet.Rec)), ntp.Log2ToDouble(system.precision))
		disp = ntp.Log2ToDouble(packet.Precision) + ntp.Log2ToDouble(system.precision) + PHI*delay
	}

	// Don't use this offset/delay if KoD, probably invalid
	if kod {
		info("KoD packet, skipping filter")
		return
	}

	system.clockFilter(association, offset, delay, disp)
}

// TODO: Add auth
func (system *NTPalSystem) reply(receivePacket ntp.ReceivePacket, mode ntp.Mode) *ntp.TransmitPacket {
	var transmitPacket ntp.TransmitPacket

	transmitPacket.Version = receivePacket.Version
	transmitPacket.Srcaddr = receivePacket.Dstaddr
	transmitPacket.Dstaddr = receivePacket.Srcaddr
	transmitPacket.Leap = system.leap
	transmitPacket.Mode = mode
	if system.stratum == MAXSTRAT {
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Poll = receivePacket.Poll
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = ntp.ShortEncoded(system.rootdelay * NTPShortLength)
	transmitPacket.Rootdisp = ntp.ShortEncoded(system.rootdisp * NTPShortLength)
	transmitPacket.Refid = system.refid
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = receivePacket.Xmt
	transmitPacket.Rec = receivePacket.Dst
	transmitPacket.Xmt = GetSystemTime()

	/*
	 * If the authentication code is A.NONE, include only the
	 * header; if A.CRYPTO, send a crypto-NAK; if A.OK, send a valid
	 * MAC.  Use the key ID in the received packet And the key in
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

func (system *NTPalSystem) pollPeer(association *Association) {
	var transmitPacket ntp.TransmitPacket

	/*
	 * Initialize header and transmit timestamp
	 */
	transmitPacket.Srcaddr = association.Dstaddr
	transmitPacket.Dstaddr = association.Srcaddr
	transmitPacket.Leap = system.leap
	transmitPacket.Version = association.Version
	transmitPacket.Mode = association.hmode
	if system.stratum == MAXSTRAT {
		transmitPacket.Stratum = 0
	} else {
		transmitPacket.Stratum = system.stratum
	}
	transmitPacket.Poll = association.Hpoll
	transmitPacket.Precision = system.precision
	transmitPacket.Rootdelay = ntp.ShortEncoded(system.rootdelay * NTPShortLength)
	transmitPacket.Rootdisp = ntp.ShortEncoded(system.rootdisp * NTPShortLength)
	transmitPacket.Refid = system.refid
	transmitPacket.Reftime = system.reftime
	transmitPacket.Org = association.Org
	transmitPacket.Rec = association.Rec

	// Xmt set lower down

	/*
	 * If the key ID is nonzero, send a valid MAC using the key ID
	 * of the association and the key in the local key cache.  If
	 * something breaks, like a missing trusted key, don't send the
	 * packet; just reset the association and stop until the problem
	 * is fixed.
	 */
	if association.Keyid != 0 {
		// if (/* p->keyid invalid */ 0) {
		//         clear(p, X_NKEY);
		//         return;
		// }
		// x.dgst = md5(p->keyid);
	}

	transmitPacket.Xmt = GetSystemTime()
	association.Xmt = transmitPacket.Xmt

	_, err := system.conn.WriteTo(ntp.EncodeTransmitPacket(transmitPacket), transmitPacket.Dstaddr)
	if err != nil {
		fmt.Println("Error", err)
	}
}

func (system *NTPalSystem) pollUpdate(association *Association, poll int8) {
	association.Hpoll = int8(math.Max(math.Min(float64(association.maxpoll), float64(poll)), float64(association.minpoll)))
	if association.burst > 0 {
		if uint64(association.nextdate) != system.Clock.T {
			return
		} else {
			association.nextdate += BTIME
		}
	} else {
		// info("Next date based on poll:", 1<<int32(math.Max(math.Min(float64(association.Poll),
		// 	float64(association.Hpoll)), float64(MINPOLL))), association.Poll, association.Hpoll)
		association.nextdate = association.outdate + (1 << int32(math.Max(math.Min(float64(association.Poll),
			float64(association.Hpoll)), float64(MINPOLL))))
	}

	if uint64(association.nextdate) <= system.Clock.T {
		association.nextdate = int32(system.Clock.T + 1)
	}
}

func (system *NTPalSystem) clear(association *Association, kiss AssociationStateCode) {
	/*
	 * The first thing to do is return all resources to the bank.
	 * Typical resources are not detailed here, but they include
	 * dynamically allocated structures for keys, certificates, etc.
	 * If an ephemeral association and not initialization, return
	 * the association memory as well.
	 */
	/* return resources */
	if system.association == association {
		system.association = nil
	}

	if kiss != INIT && association.ephemeral {
		return
	}

	// Initialize the association fields for general reset.
	// Some of these are overriden below, but keep reset here for now too, as that's what the
	// RFC does.
	association.Org = 0
	association.Rec = 0
	association.Xmt = 0
	association.t = 0
	association.Update = 0
	association.f = [NSTAGE]FilterStage{}
	association.Offset = 0
	association.Delay = 0
	association.Disp = 0
	association.Jitter = 0
	association.Hpoll = 0
	association.burst = 0
	association.Reach = 0
	association.unreach = 0
	association.ttl = 0

	association.Leap = NOSYNC
	association.Stratum = MAXSTRAT
	association.Poll = association.maxpoll
	association.Hpoll = association.minpoll
	association.Disp = MAXDISP
	association.Jitter = ntp.Log2ToDouble(system.precision)
	association.Refid = uint32(kiss)
	for i := 0; i < NSTAGE; i++ {
		association.f[i].disp = MAXDISP
		association.f[i].delay = MAXDISP
	}

	/*
	 * Randomize the first poll just in case thousands of broadcast
	 * clients have just been stirred up after a long absence of the
	 * broadcast server.
	 */
	association.t = float64(system.Clock.T)
	association.Update = float64(system.Clock.T)
	association.outdate = int32(association.t)
	association.nextdate = association.outdate + rand.Int31n(1<<association.minpoll)
}

func (system *NTPalSystem) clockFilter(association *Association, offset float64, delay float64, disp float64) {
	var f FilterStages

	/*
	 * The clock Filter contents consist of eight tuples (offset,
	 * delay, dispersion, time).  Shift each tuple to the left,
	 * discarding the leftmost one.  As each tuple is shifted,
	 * increase the dispersion since the last filter update.  At the
	 * same time, copy each tuple to a temporary list.  After this,
	 * place the (offset, delay, disp, time) in the vacated
	 * rightmost tuple.
	 */
	dtemp := PHI * (float64(system.Clock.T) - association.Update)
	association.Update = float64(system.Clock.T)
	for i := NSTAGE - 1; i > 0; i-- {
		association.f[i] = association.f[i-1]
		association.f[i].disp += dtemp
		f[i] = association.f[i]
		if association.f[i].disp >= MAXDISP {
			association.f[i].disp = MAXDISP
			f[i].delay = MAXDISP
		} else if association.Update-association.t > ALLAN {
			f[i].delay = association.f[i].delay +
				association.f[i].disp
		}
	}

	association.f[0].t = system.Clock.T
	association.f[0].offset = offset
	association.f[0].delay = delay
	association.f[0].disp = disp
	f[0] = association.f[0]

	// If the clock Has stabilized, sort the samples by delay
	if system.hold == 0 {
		sort.Sort(ByDelay{&f})
	}

	m := 0
	for i := 0; i < NSTAGE; i++ {
		if f[i].delay >= MAXDISP || (m >= 2 && f[i].delay >= float64(MAXDIST)) {
			continue
		}
		m++
	}

	association.Disp = 0
	association.Jitter = 0
	for i := NSTAGE - 1; i >= 0; i-- {
		association.Disp = 0.5 * (association.Disp + f[i].disp)
		if i < m {
			association.Jitter += math.Pow((f[0].offset - f[i].offset), 2)
		}
	}

	if m == 0 {
		system.clockSelect()
		return
	}

	etemp := math.Abs(association.Offset - f[0].offset)
	association.Offset = f[0].offset
	association.Delay = f[0].delay
	if m > 1 {
		association.Jitter /= float64(m - 1)
	}
	association.Jitter = math.Max(math.Sqrt(association.Jitter), ntp.Log2ToDouble(system.precision))

	if system.query {
		system.filtered <- 0
	}

	/*
	 * Popcorn spike suppressor.  Compare the difference between the
	 * last and current offsets to the current jitter.  If greater
	 * than SGATE (3) and if the interval since the last offset is
	 * less than twice the system poll interval, dump the spike.
	 * Otherwise, and if not in a burst, shake out the truechimers.
	 */
	if association.Disp < float64(MAXDIST) && f[0].disp < float64(MAXDIST) && etemp > SGATE*association.Jitter && (float64(f[0].t)-
		association.t) < float64(2*ntp.Log2ToDouble(association.Hpoll)) {
		info("Popcorn spike suppresor failed, either offset change WAY above jitter or disp too high")
		return
	}

	/*
	 * Prime directive: use a sample only once and never a sample
	 * older than the latest one, but anything goes before first
	 * synchronized.
	 */
	if float64(f[0].t) <= association.t && system.leap != NOSYNC {
		return
	}
	association.t = float64(f[0].t)

	if association.burst == 0 || system.leap == NOSYNC {
		system.clockSelect()
	}
}

func (system *NTPalSystem) clockSelect() {
	/*
	 * We first cull the falsetickers from the server population,
	 * leaving only the truechimers.  The correctness interval for
	 * association p is the interval from offset - root_dist() to
	 * offset + root_dist().  The object of the game is to find a
	 * majority clique; that is, an intersection of correctness
	 * intervals numbering more than half the server population.
	 *
	 * First, construct the chime list of tuples (p, type, edge) as
	 * shown below, then sort the list by edge from lowest to
	 * highest.
	 */
	osys := system.association
	system.association = nil

	n := 0
	system.m = []Chime{}
	for _, association := range system.associations {
		if !system.fit(association) {
			info("Association unfit:", association.Srcaddr.IP)
			continue
		}

		system.m = append(system.m, Chime{
			association: association,
			levelType:   -1,
			edge:        association.Offset - system.rootDist(association),
		})
		system.m = append(system.m, Chime{
			association: association,
			levelType:   0,
			edge:        association.Offset,
		})
		system.m = append(system.m, Chime{
			association: association,
			levelType:   1,
			edge:        association.Offset + system.rootDist(association),
		})

		n += 3
	}

	sort.Sort(ByEdge{system.m})

	/*
	 * Find the largest contiguous intersection of correctness
	 * intervals.  Allow is the number of allowed falsetickers;
	 * found is the number of midpoints.  Note that the edge values
	 * are limited to the range +-(2 ^ 30) < +-2e9 by the timestamp
	 * calculations.
	 */
	m := len(system.associations)
	low := 2e9
	high := -2e9
	for allow := 0; 2*allow < m; allow++ {
		/*
		 * Scan the chime list from lowest to highest to find
		 * the lower endpoint.
		 */
		found := 0
		chime := 0
		for i := 0; i < n; i++ {
			chime -= system.m[i].levelType
			if chime >= m-allow {
				low = system.m[i].edge
				break
			}
			if system.m[i].levelType == 0 {
				found++
			}
		}

		/*
		 * Scan the chime list from highest to lowest to find
		 * the upper endpoint.
		 */
		chime = 0
		for i := n - 1; i >= 0; i-- {
			chime += system.m[i].levelType
			if chime >= m-allow {
				high = system.m[i].edge
				break
			}
			if system.m[i].levelType == 0 {
				found++
			}
		}

		/*
		 * If the number of midpoints is greater than the number
		 * of allowed falsetickers, the intersection contains at
		 * least one truechimer with no midpoint.  If so,
		 * increment the number of allowed falsetickers and go
		 * around again.  If not and the intersection is
		 * non-empty, declare success.
		 */
		if found > allow {
			continue
		}

		if high > low {
			break
		}
	}

	if high < low {
		return
	}

	/*
	 * Clustering algorithm.  Construct a list of survivors (p,
	 * metric) from the chime list, where metric is dominated first
	 * by stratum and then by root distance.  All other things being
	 * equal, this is the order of preference.
	 */
	system.n = 0
	system.v = []Survivor{}
	for i := 0; i < n; i++ {
		if system.m[i].edge < low || system.m[i].edge > high {
			continue
		}

		association := system.m[i].association
		system.v = append(system.v, Survivor{
			association: association,
			metric:      float64(MAXDIST*association.Stratum) + system.rootDist(association),
		})
		system.n++
	}

	/*
	 * There must be at least NSANE survivors to satisfy the
	 * correctness assertions.  Ordinarily, the Byzantine criteria
	 * require four survivors, but for the demonstration here, one
	 * is acceptable.
	 */
	if system.n < NSANE {
		return
	}

	sort.Sort(ByMetric{system.v})

	/*
	 * For each association p in turn, calculate the selection
	 * jitter p->sjitter as the square root of the sum of squares
	 * (p->offset - q->offset) over all q associations.  The idea is
	 * to repeatedly discard the survivor with maximum selection
	 * jitter until a termination condition is met.
	 */
	for {
		var sjitterMaxIdx int
		var max, min, dtemp float64

		max = -2e9
		min = 2e9
		for i := 0; i < system.n; i++ {
			p := system.v[i].association
			if p.Jitter < min {
				min = p.Jitter
			}
			dtemp = 0
			if system.n > 1 {
				for j := 0; j < system.n; j++ {
					q := system.v[j].association
					dtemp += math.Pow(p.Offset-q.Offset, 2)
				}
				dtemp = math.Sqrt(dtemp / float64(system.n-1))
			}

			if dtemp*system.rootDist(p) > max {
				max = dtemp
				sjitterMaxIdx = i
			}
		}

		/*
		 * If the maximum selection jitter is less than the
		 * minimum peer jitter, then tossing out more survivors
		 * will not lower the minimum peer jitter, so we might
		 * as well stop.  To make sure a few survivors are left
		 * for the clustering algorithm to chew on, we also stop
		 * if the number of survivors is less than or equal to
		 * NMIN (3).
		 */
		if max < min || n <= NMIN {
			break
		}

		/*
		 * Delete survivor with max sjitter from the list and go around
		 * again.
		 */
		removeIndex(&system.v, sjitterMaxIdx)
		system.n--
	}

	/*
	 * Pick the best clock.  If the old system peer is on the list
	 * and at the same stratum as the first survivor on the list,
	 * then don't do a clock Hop.  Otherwise, select the first
	 * survivor on the list as the new system peer.
	 */
	if osys != nil && osys.Stratum == system.v[0].association.Stratum && containsAssociation(system.v, osys) {
		system.association = osys
	} else {
		system.association = system.v[0].association
		info("NEW SYSTEM PEER picked:", system.association.Srcaddr.IP)
	}

	system.clockUpdate(system.association)
}

func (system *NTPalSystem) clockUpdate(association *Association) {
	info("Clock Update**")
	/*
	 * If this is an old update, for instance, as the result of a
	 * system peer change, avoid it.  We never use an old sample or
	 * the same sample twice.
	 */
	if float64(system.t) >= association.t {
		return
	}

	/*
	 * Combine the survivor offsets and update the system clock; the
	 * local_clock() routine will tell us the good or bad news.
	 */
	system.clockCombine()
	switch system.localClock(association, system.offset) {
	/*
	 * The offset is too large and probably bogus.  Complain to the
	 * system log and order the operator to set the clock Manually
	 * within PANIC range.  The reference implementation includes a
	 * command line option to disable this check and to change the
	 * panic threshold from the default 1000 s as required.
	 */
	//   TODO: Above^
	case PANIC:
		debug("Offset:", system.offset)
		log.Fatal("Offset too large!")

	/*
	 * The offset is more than the step threshold (0.125 s by
	 * default).  After a step, all associations now have
	 * inconsistent time values, so they are reset and started
	 * fresh.  The step threshold can be changed in the reference
	 * implementation in order to lessen the chance the clock Might
	 * be stepped backwards.  However, there may be serious
	 * consequences, as noted in the white papers at the NTP project
	 * site.
	 */
	case LSTEP:
		info("Discipline STEPPED")
		system.t = uint64(association.t)
		for _, association := range system.associations {
			system.clear(association, STEP)
		}
		system.stratum = MAXSTRAT
		system.poll = association.minpoll
		system.rootdelay = 0
		system.rootdisp = 0
		system.jitter = ntp.Log2ToDouble(system.precision)

	/*
	 * The offset was less than the step threshold, which is the
	 * normal case.  Update the system variables from the peer
	 * variables.  The lower clamp on the dispersion increase is to
	 * avoid timing loops and clockhOpping when highly precise
	 * sources are in play.  The clamp can be changed from the
	 * default .01 s in the reference implementation.
	 */
	case SLEW:
		info("Discipline SLEWED")
		// Offset and jitter already set by clockCombine()
		system.leap = association.Leap
		system.t = uint64(association.t)
		system.stratum = association.Stratum + 1
		if association.Stratum == 0 || association.Stratum == 16 {
			system.refid = association.Refid
		} else {
			system.refid = ipToRefID(association.Srcaddr.IP)
		}
		system.reftime = association.Reftime
		system.rootdelay = association.Rootdelay + association.Delay
		dtemp := math.Max(association.Rootdisp+association.Disp+system.jitter+PHI*(float64(system.Clock.T)-association.Update)+
			math.Abs(association.Offset), MINDISP)
		system.rootdisp = dtemp
		fmt.Println("Root disp calc:", association.Disp, association.Rootdisp)
	/*
	 * Some samples are discarded while, for instance, a direct
	 * frequency measurement is being made.
	 */
	case IGNORE:
		info("Discipline IGNORED")
	}
}

func (system *NTPalSystem) clockCombine() {
	var association *Association
	var x, y, z, w float64

	/*
	 * Combine the offsets of the clustering algorithm survivors
	 * using a weighted average with weight determined by the root
	 * distance.  Compute the selection jitter as the weighted RMS
	 * difference between the first survivor and the remaining
	 * survivors.  In some cases, the inherent clock Jitter can be
	 * reduced by not using this algorithm, especially when frequent
	 * clockhOpping is involved.  The reference implementation can
	 * be configured to avoid this algorithm by designating a
	 * preferred peer.
	 */
	w = 0
	z = w
	y = z
	for i := 0; i < len(system.v) && system.v[i].association != nil; i++ {
		association = system.v[i].association
		x = system.rootDist(association)
		y += 1 / x
		z += association.Offset / x
		w += math.Pow(association.Offset-system.v[0].association.Offset, 2) / x
	}
	system.offset = z / y
	system.jitter = math.Sqrt(w / y)
}

func (system *NTPalSystem) localClock(association *Association, offset float64) LocalClockReturnCode {
	system.Clock.lock.Lock()
	defer system.Clock.lock.Unlock()

	var freq, mu float64
	var rval LocalClockReturnCode
	var etemp, dtemp float64

	/*
	 * If the offset is too large, give up and go home.
	 */
	if math.Abs(offset) > PANICT {
		return PANIC
	}

	/*
	 * Clock State machine transition function.  This is where the
	 * action is and defines how the system reacts to large time
	 * and frequency errors.  There are two main regimes: when the
	 * offset exceeds the step threshold and when it does not.
	 */
	rval = SLEW
	mu = association.t - float64(system.t)
	freq = 0
	info("Disciplining with offset:", offset)
	if math.Abs(offset) > STEPT {
		// fmt.Println("Offset > STEPT (0.128)", "|STATE:", system.Clock.State, "|OFFSET:", offset)
		switch system.Clock.State {
		/*
		 * In S_SYNC state, we ignore the first outlier and
		 * switch to S_SPIK state.
		 */
		case SYNC:
			system.Clock.State = SPIK
			return rval

		/*
		 * In S_FREQ state, we ignore outliers and inliers.  At
		 * the first outlier after the stepout threshold,
		 * compute the apparent frequency correction and step
		 * the time.
		 */
		case FREQ:
			if mu < WATCH {
				return IGNORE
			}

			freq = (offset - system.Clock.Offset) / mu
			fallthrough

		/*
		 * In S_SPIK state, we ignore succeeding outliers until
		 * either an inlier is found or the stepout threshold is
		 * exceeded.
		 */
		case SPIK:
			if mu < WATCH {
				return IGNORE
			}

			/* fall through to default */
			fallthrough

		/*
		 * We get here by default in S_NSET and S_FSET states
		 * and from above in S_FREQ state.  Step the time and
		 * clamp down the poll interval.
		 *
		 * In S_NSET state, an initial frequency correction is
		 * not available, usually because the frequency file has
		 * not yet been written.  Since the time is outside the
		 * capture range, the clock Is stepped.  The frequency
		 * will be set directly following the stepout interval.
		 *
		 * In S_FSET state, the initial frequency has been set
		 * from the frequency file.  Since the time is outside
		 * the capture range, the clock Is stepped immediately,
		 * rather than after the stepout interval.  Guys get
		 * nervous if it takes 17 minutes to set the clock For

		 * the first time.
		 *
		 * In S_SPIK state, the stepout threshold has expired
		 * and the phase is still above the step threshold.
		 * Note that a single spike greater than the step
		 * threshold is always suppressed, even at the longer
		 * poll intervals.
		 */
		default:
			/*
			 * This is the kernel set time function, usually
			 * implemented by the Unix settimeofday() system
			 * call.
			 */
			stepTime(offset)
			system.Clock.Count = 0
			system.poll = association.minpoll
			rval = LSTEP
			// Initialize hold timer for training and startup intervals
			if system.Clock.State == NSET || system.Clock.State == FSET {
				system.hold = WATCH
			}

			if system.Clock.State == NSET {
				system.rstclock(FREQ, association.t, 0)
				return rval
			}
		}
		system.rstclock(SYNC, association.t, 0)
	} else {
		// fmt.Println("OFFSET < STEPT (0.128)", "|STATE:", system.Clock.State, "|OFFSET:", offset)
		/*
		* Compute the clock Jitter as the RMS of exponentially
		* weighted offset differences.  This is used by the
		* poll-adjust code.
		 */
		etemp = math.Pow(system.Clock.Jitter, 2)
		dtemp = math.Pow(math.Max(math.Abs(offset-system.Clock.Last),
			ntp.Log2ToDouble(system.precision)), 2)
		system.Clock.Jitter = math.Sqrt(etemp + (dtemp-etemp)/AVG)
		switch system.Clock.State {

		/*
		 * In S_NSET state, this is the first update received
		 * and the frequency has not been initialized.  The
		 * first thing to do is directly measure the oscillator
		 * frequency.
		 */
		case NSET:
			// Perform a step, despite offset < STEPT. The reason for this is that adjustTime
			// would mess up the frequency measurement in the next clock State.
			stepTime(offset)
			system.Clock.Count = 0
			system.poll = association.minpoll
			system.hold = WATCH
			system.rstclock(FREQ, association.t, 0)
			return LSTEP

		/*
		 * In S_FREQ state, ignore updates until the stepout
		 * threshold.  After that, correct the phase and
		 * frequency and switch to S_SYNC state.
		 */
		case FREQ:
			if mu < WATCH {
				// An addition to help better find the initial frequency, since sometimes the step is bad
				// if system.Clock.Offset == 0 {
				// 	system.Clock.Offset = offset
				// }
				return IGNORE
			}

			system.hold = WATCH
			freq = (offset - system.Clock.Offset) / mu

			fallthrough

		/*
		 * We get here by default in S_SYNC and S_SPIK states.
		 * Here we compute the frequency update due to PLL and
		 * FLL contributions.
		 */
		default:

			/*
			 * The FLL and PLL frequency gain constants
			 * depending on the poll interval and Allan
			 * intercept.  The FLL is not used below one
			 * half the Allan intercept.  Above that the
			 * loop gain increases in steps to 1 / AVG.
			 */
			//  TODO: re-add this?
			if system.hold == 0 {
				if ntp.Log2ToDouble(system.poll) > ALLAN {
					freq += (offset - system.Clock.Offset) / (FLL * math.Max(mu, float64(system.poll)))

					info("FREQ update (FLL):", freq)
				}
				/*
				 * For the PLL the integration interval
				 * (numerator) is the minimum of the update
				 * interval and poll interval.  This allows
				 * oversampling, but not undersampling.
				 */

				//  PLL
				etemp = math.Min(mu, ALLAN)
				dtemp = 4 * PLL * ntp.Log2ToDouble(system.poll)
				freq += offset * etemp / (dtemp * dtemp)
				info("FREQ update (PLL):", offset*etemp/(dtemp*dtemp))
			}

			if math.Abs(offset) < STARTUP_OFFSET_MAX {
				system.hold = 0
			}
		}
		system.rstclock(SYNC, association.t, offset)
	}

	/*
	 * Calculate the new frequency and frequency stability (wander).
	 * Compute the clock Wander as the RMS of exponentially weighted
	 * frequency differences.  This is not used directly, but can,
	 * along with the jitter, be a highly useful monitoring and
	 * debugging tool.
	 */
	freq += system.Clock.Freq
	system.Clock.Freq = math.Max(math.Min(MAXFREQ, freq), -MAXFREQ)
	info("Set FREQ to:", system.Clock.Freq)
	etemp = math.Pow(system.Clock.Wander, 2)
	dtemp = math.Pow(freq, 2)
	system.Clock.Wander = math.Sqrt(etemp + (dtemp-etemp)/AVG)

	/*
	 * Here we adjust the poll interval by comparing the current
	 * offset with the clock Jitter.  If the offset is less than the
	 * clock Jitter times a constant, then the averaging interval is
	 * increased; otherwise, it is decreased.  A bit of hysteresis
	 * helps calm the dance.  Works best using burst mode.
	 */
	// fmt.Println("CLOCK OFFSET:", system.Clock.Offset, "PGATE*system.Clock.Jitter:", PGATE*system.Clock.Jitter)
	if system.hold > 0 {
		system.Clock.Count = 0
		return rval
	}

	if math.Abs(system.Clock.Offset) < PGATE*system.Clock.Jitter {
		info("Incrementing clock Count based on offset and jitter")
		system.Clock.Count += int32(system.poll)
		if system.Clock.Count > LIMIT {
			system.Clock.Count = LIMIT
			if system.poll < association.maxpoll {
				system.Clock.Count = 0
				system.poll++
			}
		}
	} else {
		system.Clock.Count -= int32(system.poll << 1)
		if system.Clock.Count < -LIMIT {
			system.Clock.Count = -LIMIT
			if system.poll > association.minpoll {
				system.Clock.Count = 0
				system.poll--
			}
		}
	}
	return rval
}

func (system *NTPalSystem) rstclock(state int, t, offset float64) {
	/*
	 * Enter new state and set state variables.  Note, we use the
	 * time of the last clock Filter sample, which must be earlier
	 * than the current time.
	 */
	system.Clock.State = state
	system.Clock.Last = system.Clock.Offset
	system.Clock.Offset = offset
	system.t = uint64(t)
}

/*
 * fit() - test if association p is acceptable for synchronization
 */
func (system *NTPalSystem) fit(association *Association) bool {
	/*
	 * A stratum error occurs if (1) the server has never been
	 * synchronized, (2) the server stratum is invalid.
	 */
	if association.Leap == NOSYNC || association.Stratum >= MAXSTRAT {
		return false
	}

	/*
	 * A distance error occurs if the root distance exceeds the
	 * distance threshold plus an increment equal to one poll
	 * interval.
	 */
	if system.rootDist(association) > float64(MAXDIST)+PHI*ntp.Log2ToDouble(system.poll) {
		info("Don't fit?:", association.Srcaddr.IP, system.rootDist(association), float64(MAXDIST)+PHI*ntp.Log2ToDouble(system.poll), association.Disp, association.Rootdisp)
		return false
	}

	/*
	 * A loop error occurs if the remote peer is synchronized to the
	 * local peer or the remote peer is synchronized to the current
	 * system peer.  Note this is the behavior for IPv4; for IPv6
	 * the MD5 hash is used instead.
	 */
	if association.Refid == ipToRefID(association.Dstaddr.IP) || association.Refid == system.refid {
		return false
	}

	/*
	 * An unreachable error occurs if the server is unreachable.
	 */
	if association.Reach == 0 {
		return false
	}

	return true
}

func (system *NTPalSystem) rootDist(association *Association) float64 {
	/*
	 * The root synchronization distance is the maximum error due to
	 * all causes of the local clock Relative to the primary server.
	 * It is defined as half the total delay plus total dispersion
	 * plus peer jitter.
	 */
	return (association.Rootdelay+association.Delay)/2 +
		association.Rootdisp + association.Disp + PHI*float64(float64(system.Clock.T)-association.Update) + association.Jitter
}

func (system *NTPalSystem) GetAssociations() []*ntp.Association {
	ntpAssociations := []*ntp.Association{}
	for _, association := range system.associations {
		ntpAssociations = append(ntpAssociations, &association.Association)
	}
	return ntpAssociations
}

func containsAssociation(survivors []Survivor, association *Association) bool {
	for _, survivor := range survivors {
		if survivor.association == association {
			return true
		}
	}
	return false
}

func refIDToIP(refID ntp.ShortEncoded) net.IP {
	ipBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ipBytes, refID)
	return net.IP(ipBytes)
}

func ipToRefID(ip net.IP) ntp.ShortEncoded {
	return binary.BigEndian.Uint32(ip.To4())
}
