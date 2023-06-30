# NTPal (Network Time Pal) &mdash; Your Network Time Best Friend

NTPal is an in-development, incomplete, and rough around the edges implementation of an NTP client/server process. It also supports NTP symmetric mode, like any good friend would.

## [time.andrewlester.net](https://time.andrewlester.net)

[time.andrewlester.net](https://time.andrewlester.net) is the canonical server running NTPal for anyone to synchronize with. It does not support symmetric synchronization.

The server is hosted on [fly.io](https://fly.io/) with a configuration outlined in the [fly.toml](https://github.com/AndrewLester/ntpal/blob/main/fly.toml). Deployment is as simple as running `fly deploy`.

## NTPal &mdash; Query

NTPal supports a simpler "query" flag to simply obtain your device's time offset from an NTP server. Accessible via `--query` or `-q`, 5 messages are sent in an attempt to obtain the best measurement possible. This command functions almost identically to the `sntp` command shipped with OSX, though NTPal has far less functionality.
