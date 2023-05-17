#!/bin/bash

if [[ $# -gt 0 ]]; then
    # If we pass a command, run it
    exec "$@"
else
    # Else default to starting the server
    exec bin/ntp-report
fi
