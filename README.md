# NTPal (Network Time Pal) &mdash; Your Network Time Best Friend

NTPal is an in-development, incomplete, and rough around the edges implementation of an NTP client/server process. It also supports NTP symmetric mode, like any good friend would.

## Code Credits

The code for NTPal is _very_ similar to pseudocode provided in the [NTPv4 RFC](https://datatracker.ietf.org/doc/html/rfc5905), which is nicely documented and made for a good rewriting experience. Certain changes were made based on more recent additions to projects such as [ntpd](https://github.com/ntp-project/ntp), [chrony](https://github.com/mlichvar/chrony), and others. Additionally, some care was taken to map the program's requirements into better-fit Golang structures, but there's room for improvement (especially regarding race conditions).

## Usage

### Installation

The NTPal binary (`ntpal`) can be downloaded for Windows, MacOS, or Linux via the [most recent GitHub release](https://github.com/AndrewLester/NTPal/releases).

### [time.andrewlester.net](https://time.andrewlester.net)

[time.andrewlester.net](https://time.andrewlester.net) is the canonical server running NTPal for anyone to synchronize with. It does not support symmetric synchronization.

The server is hosted on [fly.io](https://fly.io/) with a configuration outlined in the [fly.toml](https://github.com/AndrewLester/ntpal/blob/main/fly.toml). Deployment is as simple as running `fly deploy`.

### Configuration

NTPal uses a configuration format similar to the [standard `ntpd` config](https://docs.ntpsec.org/latest/ntp_conf.html), but with far fewer options. The available commands, with arguments defined in the `ntpd` config docs, are as follows:

-   `server <address> [key _key_] [burst] [iburst] [version _version_] [prefer] [minpoll _minpoll_] [maxpoll _maxpoll_]`
-   `driftfile <path>`

Some environment variables are also available to configure the application's runtime and logging:

-   `SYMMETRIC`: Set to "1" to allow symmetric active servers to connect.
-   `INFO`: Set to "1" to print periodic system information logs.
-   `DEBUG`: Set to "1" to print timeseries clock statistics meant for graphing.

### NTPal &mdash; Query

NTPal supports a simpler "query" flag to simply obtain your device's time offset from an NTP server. Accessible via `--query` or `-q`, 5 messages are sent in an attempt to obtain the best measurement possible. This command functions almost identically to the `sntp` command shipped with OSX, though NTPal has far less functionality.

## Development

### Run

    sudo -E go run github.com/AndrewLester/ntpal/cmd/ntpal --config ntp.conf --drift ntp.drift

### Debug Log Transform

CTRL+F

```
\*\*\*\*\*ADJUSTING:\nTIME: (\d+?) SYS OFFSET: (.*?) CLOCK OFFSET: (.*?)\nFREQ:  (.*?) OFFSET \(dtemp\): (.*?)\n(Adjtime:  .*? .*?\n)?

$1,$2,$3,$4,$5\n
```
