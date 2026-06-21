#!/bin/bash

set -e

# /app/app &
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &

wait -n # exit when ANY child exits
exit $?
