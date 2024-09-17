#!/bin/bash
echo "> Reading source location"
echo "-----------------------------------------"
echo "-----------------------------------------"
echo " > GSF Cert Location"
ls -lath /etc/certbot/certificates/live/events.gitstafette.joostvdg.net

echo "-----------------------------------------"

echo " > CMG Cert Location"
ls -lath /etc/certbot/certificates-cmg/live/
ls -lath /etc/certbot/certificates-cmg/live/map.cmg.joostvdg.net
echo "-----------------------------------------"
echo "-----------------------------------------"

echo "> Copy GSF Certs to target location"
cp /etc/certbot/certificates/live/events.gitstafette.joostvdg.net/fullchain.pem /etc/envoy/certificates/gsf-fullchain.pem
cp /etc/certbot/certificates/live/events.gitstafette.joostvdg.net/cert.pem /etc/envoy/certificates/gsf-cert.pem
cp /etc/certbot/certificates/live/events.gitstafette.joostvdg.net/privkey.pem /etc/envoy/certificates/gsf-privkey.pem

echo "> Copy CMG Certs to target location"
cp /etc/certbot/certificates-cmg/live//map.cmg.joostvdg.net/fullchain.pem /etc/envoy/certificates/cmg-fullchain.pem
cp /etc/certbot/certificates-cmg/live/map.cmg.joostvdg.net/cert.pem /etc/envoy/certificates/cmg-cert.pem
cp /etc/certbot/certificates-cmg/live/map.cmg.joostvdg.net/privkey.pem /etc/envoy/certificates/cmg-privkey.pem

echo "> Reading target location"
ls -lath /etc/envoy/certificates

echo "> Set Cert permissions"
chmod 0444 /etc/envoy/certificates/gsf-fullchain.pem
chmod 0444 /etc/envoy/certificates/gsf-cert.pem
chmod 0444 /etc/envoy/certificates/gsf-privkey.pem
chmod 0444 /etc/envoy/certificates/cmg-fullchain.pem
chmod 0444 /etc/envoy/certificates/cmg-cert.pem
chmod 0444 /etc/envoy/certificates/cmg-privkey.pem

echo "> Sleeping for 1 hour"
sleep 3600