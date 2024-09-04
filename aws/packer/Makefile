format:
	packer fmt .

validate:
	packer validate .

build:
	packer build aws-ubuntu.pkr.hcl
	cat manifest.json | jq -r '.builds[-1].artifact_id' |  cut -d':' -f2