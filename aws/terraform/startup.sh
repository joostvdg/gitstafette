#!/bin/bash

echo "Bootstrapping the instance..."
cd /home/ubuntu/gitstafette

echo "Collecting resources..."

# Retrieve the secrets from AWS Secrets Manager
echo "Reading secrets from AWS Secrets Manager..."

DNS_ACCESS_KEY=$(aws --region eu-central-1 secretsmanager get-secret-value --secret-id "gitstafette/dns/gitstafette" --query SecretString --output text | jq .KEY_ID)
DNS_ACCESS_SECRET=$(aws --region eu-central-1 secretsmanager get-secret-value --secret-id "gitstafette/dns/gitstafette" --query SecretString --output text | jq .KEY)
SENTRY_DSN=$(aws --region eu-central-1 secretsmanager get-secret-value --secret-id "gitstafette/sentry" --query SecretString --output text | jq .DSN)
WEBHOOK_OAUTH_TOKEN=$(aws --region eu-central-1 secretsmanager get-secret-value --secret-id "gitstafette/oauth" --query SecretString --output text | jq .TOKEN)

# Write the secrets to a default.env file
echo "Cleaning up .env file..."
rm -f .override.env

echo "Writing secrets to .env file..."
echo "AWS_ACCESS_KEY_ID=$DNS_ACCESS_KEY" > ./override.env
echo "AWS_SECRET_ACCESS_KEY=$DNS_ACCESS_SECRET" >> ./override.env
echo "SENTRY_DSN=$SENTRY_DSN" >> ./override.env
echo "WEBHOOK_OAUTH_TOKEN=$WEBHOOK_OAUTH_TOKEN" >> ./override.env

aws s3 cp s3://gitstafette-resources/ca.pem ./certs/ca.pem
aws s3 cp s3://gitstafette-resources/events-aws-key.pem ./certs/events-aws-key.pem
aws s3 cp s3://gitstafette-resources/events-aws.pem ./certs/events-aws.pem

echo "Starting Docker Compose..."
docker compose up -d

sleep 5
echo "Docker Compose started"
docker compose ps