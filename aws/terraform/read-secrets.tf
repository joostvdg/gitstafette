#
# resource "aws_iam_policy" "secrets_manager_access" {
#   name        = "secrets_manager_access"
#   description = "Policy to allow EC2 instances to read secrets from Secrets Manager"
#
#   policy = jsonencode({
#     Version = "2012-10-17"
#     Statement = [
#       {
#         Action = [
#           "secretsmanager:GetSecretValue"
#         ]
#         Effect   = "Allow"
#         Resource = "arn:aws:secretsmanager:eu-central-1:853805194132:secret:gitstafette-demo-x1iBnU"
#       }
#     ]
#   })
# }
#
#
# resource "aws_iam_role" "ec2_role" {
#   name = "ec2_secrets_manager_role"
#
#   assume_role_policy = jsonencode({
#     Version = "2012-10-17"
#     Statement = [
#       {
#         Action    = "sts:AssumeRole"
#         Effect    = "Allow"
#         Principal = {
#           Service = "ec2.amazonaws.com"
#         }
#       }
#     ]
#   })
# }
#
#
# resource "aws_iam_role_policy_attachment" "ec2_secrets_manager_attachment" {
#   role       = aws_iam_role.ec2_role.name
#   policy_arn = aws_iam_policy.secrets_manager_access.arn
# }
#
# resource "aws_iam_instance_profile" "ec2_instance_profile" {
#   name = "ec2_instance_profile"
#   role = var.ec2_role_name
# }
