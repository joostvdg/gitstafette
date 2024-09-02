#!/bin/bash
echo "Testing log_secrets.sh"

echo "Printing certificates folder"
ls -latrh /run/secrets/

echo "Printing Environment Variables"
env
