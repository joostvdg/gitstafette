#!/bin/bash
echo "> Reading source location"
ls -lath /etc/certbot/certificates/live/events.gitstafette.joostvdg.net

echo "> Copy to target location"
cp /etc/certbot/certificates/live/events.gitstafette.joostvdg.net/*.pem /etc/envoy/certificates/

echo "> Reading target location"
ls -lath /etc/envoy/certificates

echo "> Set Cert permissions"
chmod 0444 /etc/envoy/certificates/fullchain.pem
chmod 0444 /etc/envoy/certificates/cert.pem
chmod 0444 /etc/envoy/certificates/privkey.pem

echo "> Sleeping for 30 seconds"
sleep 30