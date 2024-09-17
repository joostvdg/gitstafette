packer {
  required_plugins {
    amazon = {
      version = ">= 1.1.1"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

source "amazon-ebs" "ubuntu" {
  ami_name      = "${var.ami_prefix}-${local.date}"
  instance_type = "t4g.micro"
  region        = "eu-central-1"
  source_ami_filter {
    filters = {
      name                = "ubuntu/images/*ubuntu-*-24.04-arm64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"]
  }
  ssh_username = "ubuntu"
}

build {
  name = "gitstafette"
  sources = [
    "source.amazon-ebs.ubuntu"
  ]


  provisioner "shell" {
    inline = [
      "sudo apt-get update",
      "sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common gnupg lsb-release",
      "sudo mkdir -m 0755 -p /etc/apt/keyrings",
      "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg",
      "echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable\" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null",
      "sudo apt-get update",
      "sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin",
      "sudo systemctl status docker",
      "sudo usermod -aG docker ubuntu",
      "docker compose version",
      "sudo snap install btop",
      "sudo snap install aws-cli --classic",
      "aws --version",
      "sudo apt upgrade -y",
    ]
  }

  provisioner "file" {
    source = "../docker-compose"
    destination = "/home/ubuntu/gitstafette"
  }

  provisioner "shell" {
    inline = [
      "cd /home/ubuntu/gitstafette",
      "chmod +x /home/ubuntu/gitstafette/scripts/*.sh",
      "sudo su - ubuntu -c 'docker compose version'",
      "sudo su - ubuntu -c 'docker compose --project-directory=/home/ubuntu/gitstafette --progress=plain pull '",
    ]
  }


  post-processor "manifest" {
    output     = "manifest.json"
    strip_path = true
  }

}


locals {
  date = formatdate("YYYY-MM-DD-hh-mm", timestamp())
}

variable "ami_prefix" {
  type    = string
  default = "gitstafette-server"
}

