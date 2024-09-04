terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.65.0"
    }
  }
}

terraform {
  backend "s3" {
    bucket = "gitstafette-tf"
    key    = "gsf-backend-prod"
    region = "eu-central-1"
  }
}

resource "aws_eip" "lb" {
  instance = aws_instance.gistafette.id
  domain   = "vpc"
}

# datasource for vpc to collect object?
data "aws_vpc" "vpc" {
  id = var.vpc_id
}

data "aws_subnet" "selected" {
  id = var.subnet_id
}

data "aws_internet_gateway" "default" {
  filter {
    name   = "attachment.vpc-id"
    values = [var.vpc_id]
  }
}

# Define the security group for the Linux server
resource "aws_security_group" "aws-linux-sg" {
  name        = "linux-sg"
  description = "Allow incoming traffic to the Linux EC2 Instance"
  vpc_id      = data.aws_vpc.vpc.id

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["77.174.22.146/32"]
    description = "Allow incoming SSH connections"
  }

  ingress  {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Web server"
  }

  ingress  {
    from_port   = 50051
    to_port     = 50051
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "GPRC Server"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Create EC2 Instance
resource "aws_instance" "gistafette" {
  ami                         = var.ami_id
  instance_type               = var.linux_instance_type
  subnet_id                   = data.aws_subnet.selected.id
  vpc_security_group_ids      = [aws_security_group.aws-linux-sg.id]
  associate_public_ip_address = var.linux_associate_public_ip_address
  source_dest_check           = false
  key_name                    = var.ssh_key_name

  iam_instance_profile        = aws_iam_instance_profile.ec2_instance_profile.name

  instance_market_options {
    market_type = "spot"
    spot_options {
      instance_interruption_behavior = "stop"
      spot_instance_type             = "persistent"
    }
  }

  # root disk
  root_block_device {
    volume_size           = var.linux_root_volume_size
    volume_type           = var.linux_root_volume_type
    delete_on_termination = true
    encrypted             = true
  }

  user_data = file("${path.module}/startup.sh")

  tags = {
    Name = "GSF-BE-Prod"
    Environment = "production"
  }
}
