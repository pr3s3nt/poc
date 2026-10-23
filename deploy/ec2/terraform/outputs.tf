output "instance_id" { value = aws_instance.host.id }
output "public_ip" { value = aws_instance.host.public_ip }
output "volume_id" { value = aws_instance.host.root_block_device[0].volume_id }
output "role_arn" {
  value = local.create_role ? aws_iam_role.executor[0].arn : data.aws_iam_instance_profile.existing[0].role_arn
}
output "console_url" {
  value = var.enable_alb ? "http://${aws_lb.console[0].dns_name}/ui/" : "http://localhost:3001/ui/"
}
output "alb_arn" { value = var.enable_alb ? aws_lb.console[0].arn : "" }
output "target_group_arn" { value = var.enable_alb ? aws_lb_target_group.console[0].arn : "" }
output "host_security_group_id" { value = aws_security_group.host.id }
output "alb_security_group_id" { value = var.enable_alb ? aws_security_group.alb[0].id : "" }
output "release_commit" { value = var.release_commit }
