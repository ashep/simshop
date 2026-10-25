FROM --platform=${TARGETPLATFORM} caddy:2-alpine

RUN apk add --no-cache bash

COPY ./Caddyfile /etc/caddy/Caddyfile
RUN caddy fmt --overwrite /etc/caddy/Caddyfile

# Frontend theme is not bundled: downstream images COPY it to /frontend (see README).

# Backend
COPY app.out /app/app
RUN chmod 0755 /app/app

WORKDIR /
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
