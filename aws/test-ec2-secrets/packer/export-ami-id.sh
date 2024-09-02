#!/bin/bash

export AMI_ID=$(jq -r '.builds[-1].artifact_id | split(":") | .[1]' manifest.json)
echo "AMI_ID=$AMI_ID"
