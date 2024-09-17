variable "ami_id" {
  description = "ID of the AMI"
  default     = "ami-09652ef7f1166e7d3"
  // new AMI from 2024-09-04 ami-09652ef7f1166e7d3
}

variable "ssh_key_name" {
  description = "AWS SSH Key Name"
  default     = "gitstafette"
}

variable "aws_region" {
  type        = string
  description = "AWS region"
  default     = "eu-central-1"
}

# AWS AZ
variable "aws_az" {
  type        = string
  description = "AWS AZ"
  default     = "eu-central-1a"
}

variable "ec2_role_name" {
  type        = string
  description = "Name of the EC2 role"
  default     = "ec2-read-gitstafette-secrets"
}


# VPC Variables
variable "vpc_id" {
  type        = string
  description = "ID of your default VPC"
  default     = "vpc-a13a59ca"
}

variable "subnet_id" {
  type        = string
  description = "ID of the default subnet of your default VPC"
  default     = "subnet-84b1d9c9"
}

variable "linux_instance_type" {
  type        = string
  description = "EC2 instance type for Linux Server"
  default     = "t4g.micro"
}

variable "linux_associate_public_ip_address" {
  type        = bool
  description = "Associate a public IP address to the EC2 instance"
  default     = true
}

variable "linux_root_volume_size" {
  type        = number
  description = "Size of root volume of Linux Server"
  default     = 25
}


variable "linux_root_volume_type" {
  type        = string
  description = "Type of root volume of Linux Server."
  default     = "gp2"
}