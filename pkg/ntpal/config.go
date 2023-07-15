package ntpal

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/AndrewLester/ntpal/internal/ntp"
)

type ntpConfig struct {
	driftfile string
	servers   []serverAssociationConfig
}

type serverAssociationConfig struct {
	address  *net.UDPAddr
	hostname string
	burst    bool
	iburst   bool
	prefer   bool
	version  int
	minpoll  int
	maxpoll  int
	hmode    ntp.Mode
}

const defaultMinpoll = 6
const defaultMaxpoll = 10

func parseConfig(path string) ntpConfig {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal("File at", path, "could not be read for configuration:", err)
	}
	defer file.Close()

	config := ntpConfig{}

	serverAssociations := []serverAssociationConfig{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		arguments := strings.Split(line, " ")

		switch arguments[0] {
		case "server":
			if len(arguments) < 2 {
				configParseError("Missing required argument \"address\"")
			}

			address, err := net.ResolveUDPAddr("udp", arguments[1]+":123")
			if err != nil {
				configParseError("Invalid address: ", arguments[1])
			}
			fmt.Println(arguments[1], "resolved to:", address.IP)

			burst := optionalArgument("burst", &arguments)
			iburst := optionalArgument("iburst", &arguments)
			prefer := optionalArgument("prefer", &arguments)
			version := integerArgument("version", 4, &arguments)
			minpoll := integerArgument("minpoll", defaultMinpoll, &arguments)
			maxpoll := integerArgument("maxpoll", defaultMaxpoll, &arguments)

			if len(arguments) > 2 {
				configParseError("Invalid arguments supplied to command. One was: \"", arguments[2], "\"")
			}

			if version != int(VERSION) {
				configParseError("Only NTP version", VERSION, "is supported")
			}

			if minpoll < int(MINPOLL) {
				configParseError("minpoll must be greater than or equal to", MINPOLL)
			}

			if maxpoll > int(MAXPOLL) {
				configParseError("maxpoll must be less than or equal to", MAXPOLL)
			}

			if minpoll > maxpoll {
				configParseError("minpoll must be less than maxpoll")
			}

			serverAssociation := serverAssociationConfig{
				address:  address,
				hostname: arguments[1],
				burst:    burst,
				iburst:   iburst,
				prefer:   prefer,
				version:  version,
				minpoll:  minpoll,
				maxpoll:  maxpoll,
				hmode:    ntp.CLIENT,
			}
			serverAssociations = append(serverAssociations, serverAssociation)
		case "driftfile":
			if len(arguments) < 2 {
				configParseError("Missing required argument \"path\"")
			}

			config.driftfile = arguments[1]
		case "#", " ", "":
			// Comment/empty
		default:
			configParseError("Invalid command: ", arguments[0])
		}

	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	config.servers = serverAssociations

	return config
}

func optionalArgument(name string, arguments *[]string) bool {
	for i, argument := range *arguments {
		if name == argument {
			removeIndex(arguments, i)
			return true
		}
	}
	return false
}

func integerArgument(name string, initial int, arguments *[]string) int {
	valueStr := stringArgument(name, strconv.Itoa(initial), arguments)
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		configParseError(name, " argument requires an integer value.")
	}

	return value
}

func stringArgument(name string, initial string, arguments *[]string) string {
	for i, argument := range *arguments {
		if name == argument {
			if i == len(*arguments)-1 {
				configParseError("No value supplied for argument: ", argument)
			}

			value := (*arguments)[i+1]
			removeIndex(arguments, i)
			removeIndex(arguments, i+1)
			return value
		}
	}
	return initial
}

func removeIndex[T any](s *[]T, index int) {
	ret := make([]T, 0)
	ret = append(ret, (*s)[:index]...)
	ret = append(ret, (*s)[index+1:]...)
	*s = ret
}

func configParseError(args ...any) {
	args = append([]any{"Config parse error: "}, args...)
	log.Fatal(args...)
}
