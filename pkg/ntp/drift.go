package ntp

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

type ServerPollInterval struct {
	address string
	poll    int8
}

func readDriftInfo(system *NTPSystem) (float64, map[string]ServerPollInterval) {
	file, err := os.Open(system.drift)
	if err != nil {
		return 0, nil
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	text, _ := reader.ReadString('\n')
	text = text[:len(text)-1]
	frequency, err := strconv.ParseFloat(text, 64)
	if err != nil {
		log.Fatal("NTP drift file invalid. Delete: ", system.drift)
	}

	serverPollIntervals := map[string]ServerPollInterval{}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		tokens := strings.Split(line, " ")
		poll, _ := strconv.Atoi(tokens[1])
		serverPollIntervals[tokens[0]] = ServerPollInterval{
			address: tokens[0],
			poll:    int8(poll),
		}
	}

	return frequency, serverPollIntervals
}

func writeDriftInfo(system *NTPSystem) {
	file, err := os.OpenFile(system.drift, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("Could not open or create drift file: %v", err)
	}
	defer file.Close()

	info("Writing clock freq:", system.clock.freq, "associations:", len(system.associations))
	_, err = file.WriteString(strconv.FormatFloat(system.clock.freq, 'E', -1, 64) + "\n")
	if err != nil {
		log.Fatalf("Could not write to drift file: %v", err)
	}

	for _, association := range system.associations {
		file.WriteString(association.srcaddr.IP.String() + " " + strconv.Itoa(int(association.hpoll)) + "\n")
	}
}
