output "page" {
  value       = "https://ubiq.${local.domain}"
  description = "The page; log in here first."
}

output "mcp" {
  value       = "https://mcp.${local.domain}/mcp"
  description = "The MCP endpoint to connect Claude Code to."
}

output "callback_url" {
  value       = "https://authenticate.${local.domain}/oauth2/callback"
  description = "The redirect URI the Identity Provider's OAuth client must allow."
}

output "public_ip" {
  value = aws_eip.ubiq.public_ip
}

output "instance_id" {
  value       = aws_instance.ubiq.id
  description = "For aws ssm start-session --target."
}
