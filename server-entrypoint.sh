#!/usr/bin/env bash
set -e

nginx -c "$PWD/nginx.conf" &
export PORT=8081
env PORT=8081 /usr/bin/gitstafette --repositories 537845873 --port 8081 --grpcPort 50051 &

wait -n